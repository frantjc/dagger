package drivers

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/dagger/dagger/engine/client/imageload"
	connh "github.com/dagger/dagger/internal/buildkit/client/connhelper"
	connhKube "github.com/dagger/dagger/internal/buildkit/client/connhelper/kubepod"
	connhSSH "github.com/dagger/dagger/internal/buildkit/client/connhelper/ssh"
)

func init() {
	register("tcp", &dialDriver{available: tcpAvailable})
	register("unix", &dialDriver{available: unixAvailable})
	register("ssh", &dialDriver{fn: connhSSH.Helper, available: sshAvailable})
	register("kube-pod", kubePodDriver)
	register("tls", &dialDriver{fn: connhTLS, available: tlsAvailable})
}

// kubePodDriver connects to an engine in a running pod with `kubectl exec`.
var kubePodDriver = &dialDriver{fn: connhKube.Helper, available: kubePodAvailable}

// dialDriver uses the buildkit connhelpers to directly connect
type dialDriver struct {
	fn     func(*url.URL) (*connh.ConnectionHelper, error)
	loader imageload.Backend

	// available reports why the URL cannot be dialed, or nil if it can. Dial
	// drivers are the only driver for their scheme, so the error is returned
	// to the user as is.
	available func(context.Context, *url.URL) error
}

func (d *dialDriver) Available(ctx context.Context, u *url.URL) (bool, error) {
	if d.available != nil {
		if err := d.available(ctx, u); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (d *dialDriver) Provision(ctx context.Context, target *url.URL, _ *DriverOpts) (Connector, error) {
	return dialConnector{dialDriver: d, target: target}, nil
}

func (d *dialDriver) ImageLoader(ctx context.Context) imageload.Backend {
	return d.loader
}

type dialConnector struct {
	*dialDriver
	target *url.URL
}

func (d dialConnector) Connect(ctx context.Context) (_ net.Conn, rerr error) {
	if d.fn == nil {
		return defaultDialer(ctx, d.target.String())
	}

	helper, err := d.fn(d.target)
	if err != nil {
		return nil, err
	}
	return helper.ContextDialer(ctx, d.target.String())
}

func (d dialConnector) EngineID() string {
	// not supported yet
	return ""
}

func (d dialConnector) ClaimID() string {
	return ""
}

func defaultDialer(ctx context.Context, address string) (net.Conn, error) {
	addrParts := strings.SplitN(address, "://", 2)
	if len(addrParts) != 2 {
		return nil, fmt.Errorf("invalid address %s", address)
	}
	var d net.Dialer
	return d.DialContext(ctx, addrParts[0], addrParts[1])
}

func tcpAvailable(_ context.Context, u *url.URL) error {
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil || host == "" || port == "" {
		return fmt.Errorf("tcp engine address must be tcp://HOST:PORT, got %q", u.Redacted())
	}
	return nil
}

func unixAvailable(_ context.Context, u *url.URL) error {
	// mirrors how defaultDialer derives the address to dial
	path := strings.TrimPrefix(u.String(), "unix://")
	if path == "" {
		return fmt.Errorf("unix engine address must be unix://PATH, got %q", u.Redacted())
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("unix socket %q is not usable: %w", path, err)
	}
	return nil
}

func sshAvailable(_ context.Context, u *url.URL) error {
	if u.Hostname() == "" {
		return fmt.Errorf("ssh engine address must be ssh://[USER@]HOST[:PORT], got %q", u.Redacted())
	}
	if _, err := exec.LookPath("ssh"); err != nil {
		return fmt.Errorf("the ssh driver requires an ssh client: %w", err)
	}
	return nil
}
