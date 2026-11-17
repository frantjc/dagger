package kubectldriver

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
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

func parse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

func TestKubePodAvailable(t *testing.T) {
	t.Run("pod exists and exec is permitted", func(t *testing.T) {
		fakeKubectl(t, []string{"create pods/exec"}, false)
		require.NoError(t, kubePodAvailable(t.Context(), parse(t, "kube-pod://engine?namespace=dagger")))
	})

	t.Run("kubeconfig is passed to kubectl", func(t *testing.T) {
		argsLog := fakeKubectl(t, []string{"create pods/exec"}, false)
		require.NoError(t, kubePodAvailable(t.Context(), parse(t, "kube-pod://engine?kubeconfig=/tmp/other.yaml")))

		args, err := os.ReadFile(argsLog)
		require.NoError(t, err)
		require.Contains(t, string(args), "--kubeconfig=/tmp/other.yaml auth can-i")
		require.Contains(t, string(args), "--kubeconfig=/tmp/other.yaml get pod engine")
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
