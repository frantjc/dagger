package netdriver

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTCPAvailable(t *testing.T) {
	for raw, ok := range map[string]bool{
		"tcp://localhost:1234": true,
		"tcp://10.0.0.1:1234":  true,
		"tcp://[::1]:1234":     true,
		"tcp://localhost":      false,
		"tcp://:1234":          false,
		"tcp://":               false,
	} {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		if ok {
			require.NoError(t, tcpAvailable(t.Context(), u), raw)
		} else {
			require.Error(t, tcpAvailable(t.Context(), u), raw)
		}
	}
}

func TestUnixAvailable(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "engine.sock")

	u, err := url.Parse("unix://" + sock)
	require.NoError(t, err)
	require.Error(t, unixAvailable(t.Context(), u), "missing socket")

	require.NoError(t, os.WriteFile(sock, nil, 0o600))
	require.NoError(t, unixAvailable(t.Context(), u))

	empty, err := url.Parse("unix://")
	require.NoError(t, err)
	require.Error(t, unixAvailable(t.Context(), empty))
}
