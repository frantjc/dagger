package driver

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/dagger/dagger/engine/client"
	"github.com/dagger/dagger/engine/client/imageload"
	"github.com/dagger/dagger/internal/buildkit/client/connhelper"
)

type DialDriverFunc func(*url.URL) (*connhelper.ConnectionHelper, error)

type DialDriverOpts struct {
	ImageLoader imageload.Backend
	Available   AvailableFunc
}

type DialDriverOpt interface {
	Apply(*DialDriverOpts)
}

// AvailableFunc reports why a URL cannot be dialed, or nil if it can. Dial
// drivers are the only driver for their scheme, so the error is returned to
// the user as is.
type AvailableFunc func(context.Context, *url.URL) error

func (f AvailableFunc) Apply(opts *DialDriverOpts) {
	opts.Available = f
}

func (o *DialDriverOpts) Apply(opts *DialDriverOpts) {
	if o == nil || opts == nil {
		return
	}

	if o.ImageLoader != nil {
		opts.ImageLoader = o.ImageLoader
	}
	if o.Available != nil {
		opts.Available = o.Available
	}
}

func NewDialDriver(f DialDriverFunc, opts ...DialDriverOpt) client.Driver {
	o := &DialDriverOpts{}

	for _, opt := range opts {
		opt.Apply(o)
	}

	return &dialDriver{f, o.ImageLoader, o.Available}
}

// dialDriver uses the buildkit connhelpers to directly connect
type dialDriver struct {
	fn        func(*url.URL) (*connhelper.ConnectionHelper, error)
	loader    imageload.Backend
	available AvailableFunc
}

func (d *dialDriver) Available(ctx context.Context, u *url.URL) (bool, error) {
	if d.available != nil {
		if err := d.available(ctx, u); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (d *dialDriver) Provision(ctx context.Context, target *url.URL, _ *client.DriverOpts) (client.Connector, error) {
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

func defaultDialer(ctx context.Context, address string) (net.Conn, error) {
	addrParts := strings.SplitN(address, "://", 2)
	if len(addrParts) != 2 {
		return nil, fmt.Errorf("invalid address %s", address)
	}
	var d net.Dialer
	return d.DialContext(ctx, addrParts[0], addrParts[1])
}
