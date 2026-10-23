package cmddriver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"unicode"

	"github.com/docker/cli/cli/connhelper/commandconn"

	"github.com/dagger/dagger/engine/client"
	"github.com/dagger/dagger/engine/client/imageload"
	"github.com/dagger/dagger/util/traceexec"
)

func init() {
	client.RegisterDriver(cmdDriver{}, "cmd")
}

// cmdDriver runs a user-supplied command and relies on it to provision a
// Dagger engine and to proxy the engine's buildkit API over the command's
// stdio.
//
// The URL has the form cmd://COMMAND?arg=ARG&arg=ARG, where COMMAND is the
// URL's host and path (so cmd://./provision.sh runs ./provision.sh and
// cmd:///usr/local/bin/provision runs an absolute path) and each "arg" query
// parameter is passed to the command, in order.
//
// With url=true, the command is instead run to completion and is expected to
// print only the URL of an engine that an already-registered driver can
// connect to (for example container://my-engine) on stdout; anything else on
// stdout is an error, so diagnostics belong on stderr. That URL is then
// provisioned by its own driver. The resolved driver's image loader is not
// available in this mode.
type cmdDriver struct{}

func (cmdDriver) Available(_ context.Context, u *url.URL) (bool, error) {
	name, _, _, err := parseURL(u)
	if err != nil {
		return false, err
	}
	if _, err := exec.LookPath(name); err != nil {
		return false, fmt.Errorf("cmd driver: %w", err)
	}
	return true, nil
}

func (cmdDriver) Provision(ctx context.Context, u *url.URL, opts *client.DriverOpts) (client.Connector, error) {
	name, args, printsURL, err := parseURL(u)
	if err != nil {
		return nil, err
	}

	if !printsURL {
		return connector{name: name, args: args}, nil
	}

	stdout, _, err := traceexec.ExecOutput(ctx, exec.CommandContext(ctx, name, args...))
	if err != nil {
		return nil, fmt.Errorf("cmd driver: run %q: %w", name, err)
	}

	resolved, err := parseEngineURL(stdout)
	if err != nil {
		return nil, fmt.Errorf("cmd driver: %q: %w", name, err)
	}
	resolvedURL, err := url.Parse(resolved)
	if err != nil {
		return nil, fmt.Errorf("cmd driver: parse engine URL printed by %q: %w", name, err)
	}

	resolvedDriver, err := client.OpenDriver(ctx, resolved)
	if err != nil {
		return nil, fmt.Errorf("cmd driver: open driver for %q printed by %q: %w", resolved, name, err)
	}
	return resolvedDriver.Provision(ctx, resolvedURL, opts)
}

func (cmdDriver) ImageLoader(context.Context) imageload.Backend {
	return nil
}

type connector struct {
	name string
	args []string
}

func (c connector) Connect(ctx context.Context) (net.Conn, error) {
	// using uncancelled context because context remains active for the
	// duration of the process, after dial has completed
	return commandconn.New(context.WithoutCancel(ctx), c.name, c.args...)
}

func (connector) EngineID() string {
	// not supported yet
	return ""
}

func parseURL(u *url.URL) (name string, args []string, printsURL bool, err error) {
	name = u.Host + u.Path
	if name == "" {
		return "", nil, false, errors.New("cmd driver requires a command, e.g. cmd://./provision.sh")
	}

	q := u.Query()
	if v := q.Get("url"); v != "" {
		printsURL, err = strconv.ParseBool(v)
		if err != nil {
			return "", nil, false, fmt.Errorf("cmd driver: invalid url parameter %q: %w", v, err)
		}
	}
	return name, q["arg"], printsURL, nil
}

// parseEngineURL expects stdout to hold only the engine URL, optionally
// surrounded by whitespace.
func parseEngineURL(stdout string) (string, error) {
	s := strings.TrimSpace(stdout)
	if s == "" {
		return "", errors.New("did not print an engine URL")
	}
	if strings.ContainsFunc(s, unicode.IsSpace) {
		return "", fmt.Errorf("expected only an engine URL on stdout, got %q", s)
	}
	return s, nil
}
