package daggercmd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dagger/dagger/engine/client"
)

// The catalog drives the flag usage, the help topic, and shell completion, so
// it must name exactly the schemes that the driver registry can dial.
func TestSchemeCatalogCoversDriverRegistry(t *testing.T) {
	var catalog []string
	seen := map[string]bool{}
	for _, scheme := range client.SchemeCatalog() {
		require.False(t, seen[scheme.Scheme], "duplicate catalog entry %q", scheme.Scheme)
		seen[scheme.Scheme] = true
		catalog = append(catalog, scheme.Scheme)
	}
	require.ElementsMatch(t, client.RegisteredSchemes(), catalog)
}
