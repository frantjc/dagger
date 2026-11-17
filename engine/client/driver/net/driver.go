package netdriver

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/dagger/dagger/engine/client"
	"github.com/dagger/dagger/engine/client/driver"
)

func init() {
	client.RegisterDriver(driver.NewDialDriver(nil, driver.AvailableFunc(tcpAvailable)), "tcp")
	client.RegisterDriver(driver.NewDialDriver(nil, driver.AvailableFunc(unixAvailable)), "unix")
}

func tcpAvailable(_ context.Context, u *url.URL) error {
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil || host == "" || port == "" {
		return fmt.Errorf("tcp engine address must be tcp://HOST:PORT, got %q", u.Redacted())
	}
	return nil
}

func unixAvailable(_ context.Context, u *url.URL) error {
	// mirrors how the dial driver derives the address to dial
	path := strings.TrimPrefix(u.String(), "unix://")
	if path == "" {
		return fmt.Errorf("unix engine address must be unix://PATH, got %q", u.Redacted())
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("unix socket %q is not usable: %w", path, err)
	}
	return nil
}
