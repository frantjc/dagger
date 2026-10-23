package clouddriver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/dagger/dagger/engine/client"
	"github.com/dagger/dagger/engine/client/imageload"
	"github.com/dagger/dagger/internal/cloud"
)

func init() {
	client.RegisterDriver(&daggerCloudDriver{}, "dagger-cloud")
}

var TLSHandshakeTimeout = 15 * time.Second

// daggerCloudDriver creates and manages a Cloud Engine, then connects to it
type daggerCloudDriver struct{}

type DaggerCloudConnector struct {
	EngineSpec cloud.EngineSpec
}

func (dc *DaggerCloudConnector) Connect(ctx context.Context) (net.Conn, error) {
	serverAddr := dc.EngineSpec.URL

	// Extract hostname for SNI
	host, _, err := net.SplitHostPort(serverAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to split host and port from '%s': %w", serverAddr, err)
	}

	clientCert, err := dc.EngineSpec.TLSCertificate()
	if err != nil {
		return nil, fmt.Errorf("failed to unserialize Cloud Engine cert: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*clientCert},
		ServerName:   host,             // Required for SNI and server certificate validation
		MinVersion:   tls.VersionTLS12, // TLS 1.2 is the oldest version that is still maintained in 2025
	}

	dialer := net.Dialer{Timeout: 10 * time.Second}
	rawConn, err := dialer.DialContext(ctx, "tcp", serverAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial TCP to '%s': %w", serverAddr, err)
	}

	// tls.Client wraps rawConn; closing tlsConn closes rawConn on success.
	tlsConn := tls.Client(rawConn, tlsConfig)

	handshakeCtx, cancelHandshake := context.WithTimeout(ctx, TLSHandshakeTimeout)
	defer cancelHandshake()

	if err := tlsConn.HandshakeContext(handshakeCtx); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("tls handshake failed with '%s': %w", serverAddr, err)
	}

	return tlsConn, nil
}

func (dc *DaggerCloudConnector) EngineID() string {
	return dc.EngineSpec.InstanceID
}

func (d *daggerCloudDriver) Available(context.Context, *url.URL) (bool, error) {
	return true, nil // assume always available
}

func (d *daggerCloudDriver) Provision(ctx context.Context, _ *url.URL, opts *client.DriverOpts) (client.Connector, error) {
	if opts == nil || opts.CloudAuth == nil {
		return nil, errors.New("please run `dagger login <org>` first or configure a DAGGER_CLOUD_TOKEN")
	}
	cloudClient, err := cloud.NewClient(ctx, opts.CloudAuth)
	if err != nil {
		return nil, err
	}

	engineSpec, err := cloudClient.Engine(ctx, cloud.EngineRequest{
		Module:   opts.Module,
		Function: opts.Function,
		ExecCmd:  opts.ExecCmd,
		ClientID: opts.ClientID,
	})
	if err != nil {
		if errors.Is(err, cloud.ErrNoOrg) {
			return nil, errors.New("please associate this Engine with an org by running `dagger login <org>")
		}
		return nil, err
	}

	return &DaggerCloudConnector{EngineSpec: *engineSpec}, nil
}

func (d *daggerCloudDriver) ImageLoader(context.Context) imageload.Backend {
	return nil
}
