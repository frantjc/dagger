package client

import (
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dagger/dagger/engine"
)

func TestResolveImageIDDefaultsToEngineImage(t *testing.T) {
	orig := engine.Tag
	t.Cleanup(func() { engine.Tag = orig })

	engine.Tag = "v1.2.3"
	require.Equal(t, "registry.dagger.io/engine:v1.2.3", DefaultImageRef(""))
	id, err := ResolveImageID(DefaultImageRef(""))
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", id)

	require.Equal(t, "registry.dagger.io/engine:v1.2.3", DefaultImageRef("registry.dagger.io/engine"))
	require.Equal(t, "localhost:5000/engine:v1.2.3", DefaultImageRef("localhost:5000/engine"))
	require.Equal(t, "registry.dagger.io/engine:v0.21.7", DefaultImageRef("registry.dagger.io/engine:v0.21.7"))
	require.Equal(t, "example.com/engine@sha256:abc", DefaultImageRef("example.com/engine@sha256:abc"))

	engine.Tag = ""
	require.Equal(t, "registry.dagger.io/engine", DefaultImageRef(""))
	require.Equal(t, "registry.dagger.io/engine", DefaultImageRef("registry.dagger.io/engine"))
	require.Equal(t, "example.com/x:1", DefaultImageRef("example.com/x:1"))

	id, err = ResolveImageID(DefaultImageRef(""))
	require.NoError(t, err)
	require.Equal(t, "latest", id)

	id, err = ResolveImageID("registry.dagger.io/engine:v0.21.0")
	require.NoError(t, err)
	require.Equal(t, "v0.21.0", id)
}

func TestParseImageTarget(t *testing.T) {
	orig := engine.Tag
	t.Cleanup(func() { engine.Tag = orig })
	engine.Tag = "v1.2.3"

	parse := func(raw, nameParam string) *ImageTarget {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		got, err := ParseImageTarget(u, nameParam)
		require.NoError(t, err)
		return got
	}

	t.Run("defaults", func(t *testing.T) {
		t.Setenv("DAGGER_LEAVE_OLD_ENGINE", "")
		os.Unsetenv("DAGGER_LEAVE_OLD_ENGINE")
		got := parse("image://", "container")
		require.Equal(t, &ImageTarget{
			Ref:     "registry.dagger.io/engine:v1.2.3",
			Name:    "dagger-engine-v1.2.3",
			Cleanup: true,
		}, got)
	})

	t.Run("untagged defaults to the current version, explicit latest is kept", func(t *testing.T) {
		os.Unsetenv("DAGGER_LEAVE_OLD_ENGINE")
		require.Equal(t, "registry.dagger.io/engine:v1.2.3", parse("image://registry.dagger.io/engine", "container").Ref)
		require.Equal(t, "registry.dagger.io/engine:latest", parse("image://registry.dagger.io/engine:latest", "container").Ref)
		require.Equal(t, "dagger-engine-latest", parse("image://registry.dagger.io/engine:latest", "container").Name)
	})

	t.Run("name parameter, cleanup and env", func(t *testing.T) {
		os.Unsetenv("DAGGER_LEAVE_OLD_ENGINE")
		got := parse("image+kubectl://example.com/engine:v9?pod=mine&cleanup=false&env=A=1,B=2&env=C=3", "pod")
		require.Equal(t, &ImageTarget{
			Ref:     "example.com/engine:v9",
			Name:    "mine",
			Cleanup: false,
			Env:     []string{"A=1", "B=2", "C=3"},
		}, got)

		// the name parameter is specific to the driver
		require.Equal(t, "dagger-engine-v9", parse("image+kubectl://example.com/engine:v9?pod=mine", "container").Name)
	})

	t.Run("DAGGER_LEAVE_OLD_ENGINE wins over cleanup", func(t *testing.T) {
		t.Setenv("DAGGER_LEAVE_OLD_ENGINE", "true")
		require.False(t, parse("image://?cleanup=true", "container").Cleanup)
	})
}
