package kubectldriver

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dagger/dagger/engine/client"
)

var allProvisionAccess = []string{"list pods", "create pods", "create pods/exec"}

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
