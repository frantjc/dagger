package cmddriver

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseURL(t *testing.T) {
	for _, tc := range []struct {
		raw       string
		name      string
		args      []string
		printsURL bool
		wantErr   bool
	}{
		{raw: "cmd://provision", name: "provision"},
		{raw: "cmd://./provision.sh?arg=--foo&arg=bar", name: "./provision.sh", args: []string{"--foo", "bar"}},
		{raw: "cmd:///usr/local/bin/provision?url=true", name: "/usr/local/bin/provision", printsURL: true},
		{raw: "cmd://provision?url=false", name: "provision"},
		{raw: "cmd://provision?url=maybe", wantErr: true},
		{raw: "cmd://", wantErr: true},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			u, err := url.Parse(tc.raw)
			require.NoError(t, err)

			name, args, printsURL, err := parseURL(u)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.name, name)
			require.Equal(t, tc.args, args)
			require.Equal(t, tc.printsURL, printsURL)
		})
	}
}

func TestParseEngineURL(t *testing.T) {
	got, err := parseEngineURL("container://engine\n")
	require.NoError(t, err)
	require.Equal(t, "container://engine", got)

	got, err = parseEngineURL("  tcp://localhost:1234  \n\n")
	require.NoError(t, err)
	require.Equal(t, "tcp://localhost:1234", got)

	for _, stdout := range []string{"", "\n", "starting...\ncontainer://engine\n", "container://engine extra"} {
		_, err := parseEngineURL(stdout)
		require.Error(t, err, "%q", stdout)
	}
}
