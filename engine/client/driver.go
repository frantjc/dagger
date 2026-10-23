package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/dagger/dagger/engine/client/imageload"
	"github.com/dagger/dagger/internal/cloud/auth"
)

type Driver interface {
	// Available reports whether the driver can serve the given URL, judging by
	// the URL itself (its target and query parameters) and not only by whether
	// the driver's backend is installed. Return false with a nil error to
	// defer to the next driver registered for the scheme; wrap an
	// *UnavailableError to defer while recording why this driver cannot serve
	// the URL; return any other error to stop the search.
	Available(context.Context, *url.URL) (bool, error)

	// Provision creates any underlying resources for a driver, and returns a
	// Connector that can connect to it.
	Provision(context.Context, *url.URL, *DriverOpts) (Connector, error)

	// ImageLoader returns an optional associated image loader - not all
	// drivers will have this!
	ImageLoader(context.Context) imageload.Backend
}

type Connector interface {
	// Connect creates a connection to a dagger instance.
	//
	// Connect can be called multiple times during attempts to establish a
	// connection - but a connector can choose to block this call until
	// previously returned connections have been closed.
	Connect(context.Context) (net.Conn, error)

	// If available, a unique identifier for the engine, otherwise empty string.
	EngineID() string
}

type DriverOpts struct {
	DaggerCloudToken string
	GPUSupport       string
	Module           string
	Function         string
	ExecCmd          []string
	ClientID         string
	CloudAuth        *auth.Cloud
}

const (
	EnvDaggerCloudToken = "DAGGER_CLOUD_TOKEN"
	EnvGPUSupport       = "_EXPERIMENTAL_DAGGER_GPU_SUPPORT"
)

var (
	driverMux = map[string][]Driver{}
	driverMu  sync.RWMutex
)

func RegisterDriver(driver Driver, scheme string, schemes ...string) {
	driverMu.Lock()
	defer driverMu.Unlock()
	for _, s := range append([]string{scheme}, schemes...) {
		driverMux[s] = append(driverMux[s], driver)
	}
}

func OpenDriver(ctx context.Context, addr string) (Driver, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}

	driverMu.RLock()
	drivers, ok := driverMux[u.Scheme]
	driverMu.RUnlock()
	if !ok {
		// Teach the usable values here: an engine selector is often the first
		// thing a user gets wrong, and the message is the only help they see.
		return nil, fmt.Errorf("no driver for scheme %q found; supported schemes: %s",
			u.Scheme, strings.Join(RegisteredSchemes(), ", "))
	}
	var unavailable []string
	for _, driver := range drivers {
		available, err := driver.Available(ctx, u)
		if err != nil {
			if reason, ok := errors.AsType[*UnavailableError](err); ok {
				unavailable = append(unavailable, reason.Reason)
				continue
			}
			return nil, err
		} else if available {
			return driver, nil
		}
	}
	if len(unavailable) > 0 {
		return nil, fmt.Errorf("no driver is available for %s://%s:\n  - %s\n\nTo choose a different engine, run `dagger help engine`",
			u.Scheme, u.Host, strings.Join(unavailable, "\n  - "))
	}
	return nil, noRuntimeAvailable(u.Scheme)
}
