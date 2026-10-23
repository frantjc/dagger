package kubectldriver

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dagger/dagger/engine/client"
)

// fakeKubectl puts a kubectl on PATH that answers `auth can-i VERB RESOURCE`
// from allowed ("VERB RESOURCE" entries), fails as an unreachable cluster if
// unreachable is set, and records its arguments in the returned file.
func fakeKubectl(t *testing.T, allowed []string, unreachable bool) (argsLog string) {
	t.Helper()

	dir := t.TempDir()
	argsLog = filepath.Join(dir, "args.log")

	script := "#!/bin/sh\necho \"$@\" >> " + argsLog + "\n"
	if unreachable {
		script += "echo 'Unable to connect to the server: dial tcp 127.0.0.1:1: connect: connection refused' >&2\nexit 1\n"
	} else {
		script += `case "$*" in
`
		for _, a := range allowed {
			script += "  *\"auth can-i " + a + "\"*) echo yes; exit 0;;\n"
		}
		script += `  *"auth can-i"*) echo no; exit 1;;
  *"get pod"*) case "$*" in *missing*) echo 'Error from server (NotFound)' >&2; exit 1;; *) echo pod/x; exit 0;; esac;;
esac
exit 0
`
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kubectl"), []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsLog
}

var allProvisionAccess = []string{"list pods", "create pods", "create pods/exec"}

func parse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

func TestKubectlAvailable(t *testing.T) {
	t.Run("reachable and permitted", func(t *testing.T) {
		argsLog := fakeKubectl(t, allProvisionAccess, false)

		ok, err := kubectlImageDriver.Available(t.Context(), parse(t, "image+kubectl://engine?context=prod&namespace=dagger"))
		require.NoError(t, err)
		require.True(t, ok)

		args, err := os.ReadFile(argsLog)
		require.NoError(t, err)
		require.Contains(t, string(args), "--context=prod --namespace=dagger")
	})

	t.Run("missing RBAC says what is missing", func(t *testing.T) {
		fakeKubectl(t, []string{"list pods"}, false)

		ok, err := kubectlImageDriver.Available(t.Context(), parse(t, "image+kubectl://engine?namespace=locked"))
		require.False(t, ok)
		var unavailable *client.UnavailableError
		require.ErrorAs(t, err, &unavailable)
		require.Contains(t, unavailable.Reason, "not permitted to create pods")
		require.Contains(t, unavailable.Reason, `namespace "locked"`)
	})

	t.Run("unreachable cluster", func(t *testing.T) {
		fakeKubectl(t, nil, true)

		ok, err := kubectlImageDriver.Available(t.Context(), parse(t, "image+kubectl://engine?context=dead"))
		require.False(t, ok)
		var unavailable *client.UnavailableError
		require.ErrorAs(t, err, &unavailable)
		require.Contains(t, unavailable.Reason, "not reachable")
		require.Contains(t, unavailable.Reason, "dead")
		require.Contains(t, unavailable.Reason, "connection refused")
	})

	t.Run("kubectl not installed", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())

		ok, err := kubectlImageDriver.Available(t.Context(), parse(t, "image://engine"))
		require.NoError(t, err)
		require.False(t, ok)
	})
}

func TestKubePodAvailable(t *testing.T) {
	t.Run("pod exists and exec is permitted", func(t *testing.T) {
		fakeKubectl(t, []string{"create pods/exec"}, false)
		require.NoError(t, kubePodAvailable(t.Context(), parse(t, "kube-pod://engine?namespace=dagger")))
	})

	t.Run("pod missing", func(t *testing.T) {
		fakeKubectl(t, []string{"create pods/exec"}, false)
		require.ErrorContains(t, kubePodAvailable(t.Context(), parse(t, "kube-pod://missing")), `pod "missing" is not usable`)
	})

	t.Run("exec not permitted", func(t *testing.T) {
		fakeKubectl(t, nil, false)
		require.ErrorContains(t, kubePodAvailable(t.Context(), parse(t, "kube-pod://engine")), "not permitted to create pods/exec")
	})

	t.Run("invalid URL", func(t *testing.T) {
		fakeKubectl(t, nil, false)
		require.Error(t, kubePodAvailable(t.Context(), parse(t, "kube-pod://")))
	})
}
