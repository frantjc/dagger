package client

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/adrg/xdg"
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/dagger/dagger/engine"
)

var (
	EngineConfigPath       = filepath.Join(xdg.ConfigHome, "dagger", "engine.json")
	EngineCertificatesPath = filepath.Join(xdg.ConfigHome, "dagger", "ca-certificates")
)

const (
	HashLen = 16
)

// DefaultImageRef fills in what imageRef leaves out: an empty ref becomes the
// Dagger engine image, and a ref with neither a tag nor a digest gets the
// current engine tag. Naive dev builds have no tag, so those refs are left
// alone (which resolves to latest).
func DefaultImageRef(imageRef string) string {
	if imageRef == "" {
		imageRef = engine.EngineImageRepo
	}
	if engine.Tag == "" || hasTagOrDigest(imageRef) {
		return imageRef
	}
	return imageRef + ":" + engine.Tag
}

// hasTagOrDigest reports whether the last path element of imageRef carries a
// tag or digest, so that a registry port ("localhost:5000/engine") is not
// mistaken for one.
func hasTagOrDigest(imageRef string) bool {
	last := imageRef[strings.LastIndex(imageRef, "/")+1:]
	return strings.ContainsAny(last, ":@")
}

func ResolveImageID(imageRef string) (string, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("parsing image reference: %w", err)
	}
	if digest, ok := ref.(name.Digest); ok {
		// We already have the digest as part of the image ref
		_, id, ok := strings.Cut(digest.DigestStr(), "sha256:")
		if !ok {
			return "", fmt.Errorf("invalid image reference %q", imageRef)
		}
		return id[:7], nil
	}
	if tag, ok := ref.(name.Tag); ok {
		// Otherwise, fallback to the image tag
		return tag.TagStr(), nil
	}

	// default to latest
	return engine.Tag, nil
}

// EngineInstancePrefix prefixes the names of the containers and pods that run
// the engine, so that leftovers from other versions can be found.
const EngineInstancePrefix = "dagger-engine-"

// ImageTarget is what an image-based driver (one that runs the engine as a
// container or pod) needs to know from its URL.
type ImageTarget struct {
	// Ref is the image to run.
	Ref string
	// Name is the name of the container or pod to run it in.
	Name string
	// Cleanup is whether to remove engines left over from other versions.
	Cleanup bool
	// Env is the KEY=VALUE environment to run the engine with.
	Env []string
}

// ParseImageTarget reads an ImageTarget from an image-style URL
// (scheme://IMAGE?...). nameParam is the query parameter that overrides the
// derived name ("container" or "pod"). The env query parameter may be given
// repeatedly and may itself be comma-separated.
func ParseImageTarget(u *url.URL, nameParam string) (*ImageTarget, error) {
	q := u.Query()

	t := &ImageTarget{
		Ref:     DefaultImageRef(u.Host + u.Path),
		Name:    q.Get(nameParam),
		Cleanup: true,
	}

	if t.Name == "" {
		id, err := ResolveImageID(t.Ref)
		if err != nil {
			return nil, err
		}
		t.Name = EngineInstancePrefix + id
	}

	if val, ok := os.LookupEnv("DAGGER_LEAVE_OLD_ENGINE"); ok {
		leave, _ := strconv.ParseBool(val)
		t.Cleanup = !leave
	} else if val := q.Get("cleanup"); val != "" {
		t.Cleanup, _ = strconv.ParseBool(val)
	}

	for _, value := range q["env"] {
		for env := range strings.SplitSeq(value, ",") {
			if env != "" {
				t.Env = append(t.Env, env)
			}
		}
	}

	return t, nil
}
