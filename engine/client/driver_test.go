package client

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dagger/dagger/engine/client/imageload"
)

type stubDriver struct {
	available bool
	err       error
}

func (d stubDriver) Available(context.Context, *url.URL) (bool, error) { return d.available, d.err }

func (stubDriver) Provision(context.Context, *url.URL, *DriverOpts) (Connector, error) {
	return nil, nil
}

func (stubDriver) ImageLoader(context.Context) imageload.Backend { return nil }

func TestOpenDriver(t *testing.T) {
	t.Run("skips unavailable drivers and returns the first usable one", func(t *testing.T) {
		usable := stubDriver{available: true}
		RegisterDriver(stubDriver{err: &UnavailableError{Reason: "first: nope"}}, "opendriver-test-a")
		RegisterDriver(stubDriver{}, "opendriver-test-a")
		RegisterDriver(usable, "opendriver-test-a")

		got, err := OpenDriver(t.Context(), "opendriver-test-a://target")
		require.NoError(t, err)
		require.Equal(t, usable, got)
	})

	t.Run("reports why every driver was unavailable", func(t *testing.T) {
		RegisterDriver(stubDriver{err: &UnavailableError{Reason: "first: container not found"}}, "opendriver-test-b")
		RegisterDriver(stubDriver{err: &UnavailableError{Reason: "second: cluster unreachable"}}, "opendriver-test-b")

		_, err := OpenDriver(t.Context(), "opendriver-test-b://target?secret=value")
		require.ErrorContains(t, err, "first: container not found")
		require.ErrorContains(t, err, "second: cluster unreachable")
		require.NotContains(t, err.Error(), "secret")
	})

	t.Run("other errors stop the search", func(t *testing.T) {
		boom := errors.New("boom")
		RegisterDriver(stubDriver{err: boom}, "opendriver-test-c")
		RegisterDriver(stubDriver{available: true}, "opendriver-test-c")

		_, err := OpenDriver(t.Context(), "opendriver-test-c://target")
		require.ErrorIs(t, err, boom)
	})

	t.Run("no available driver and no reasons", func(t *testing.T) {
		RegisterDriver(stubDriver{}, "opendriver-test-d")

		_, err := OpenDriver(t.Context(), "opendriver-test-d://target")
		var runtimeErr *RuntimeUnavailableError
		require.ErrorAs(t, err, &runtimeErr)
	})

	t.Run("unknown scheme lists supported schemes", func(t *testing.T) {
		_, err := OpenDriver(t.Context(), "opendriver-test-unknown://target")
		require.ErrorContains(t, err, "supported schemes")
	})
}
