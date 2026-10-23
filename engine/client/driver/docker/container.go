package dockerdriver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"

	telemetry "github.com/dagger/otel-go"
	"go.opentelemetry.io/otel"

	"github.com/dagger/dagger/engine/client"
	"github.com/dagger/dagger/engine/client/imageload"
	"github.com/dagger/dagger/engine/config"
	"github.com/dagger/dagger/engine/distconsts"
	"github.com/dagger/dagger/engine/slog"
)

var (
	dockerImageDriver     = &imageDriver{docker{cmd: "docker"}}
	dockerContainerDriver = &containerDriver{docker{cmd: "docker"}}

	appleImageDriver     = &imageDriver{apple{}}
	appleContainerDriver = &containerDriver{apple{}}

	podmanImageDriver     = &imageDriver{docker{cmd: "podman"}}
	podmanContainerDriver = &containerDriver{docker{cmd: "podman"}}

	nerdctlContainerDriver = &containerDriver{docker{cmd: "nerdctl"}}
	nerdctlImageDriver     = &imageDriver{docker{cmd: "nerdctl"}}

	finchImageDriver     = &imageDriver{docker{cmd: "finch"}}
	finchContainerDriver = &containerDriver{docker{cmd: "finch"}}
)

func init() {
	client.RegisterDriver(dockerImageDriver,
		"docker-image", // legacy
		"image",
		"image+docker",
	)

	client.RegisterDriver(appleImageDriver,
		"image",
		"image+apple",
	)

	client.RegisterDriver(podmanImageDriver,
		"image",
		"image+podman",
	)

	client.RegisterDriver(finchImageDriver,
		"image",
		"image+finch",
	)

	client.RegisterDriver(nerdctlImageDriver,
		"image",
		"image+nerdctl",
	)

	client.RegisterDriver(dockerContainerDriver,
		"docker-container", // legacy
		"container",
		"container+docker",
	)

	client.RegisterDriver(appleContainerDriver,
		"container",
		"container+apple",
	)

	client.RegisterDriver(podmanContainerDriver,
		"container",
		"container+podman",
	)

	client.RegisterDriver(finchContainerDriver,
		"container",
		"container+finch",
	)

	client.RegisterDriver(nerdctlContainerDriver,
		"container",
		"container+nerdctl",
	)
}

// containerBackend is a generic backend for containers that can be plugged
// into image/container drivers. This allows us to have all the exact same
// logic between different container providers, but have different backend
// commands.
type containerBackend interface {
	// Name identifies the backend to the user, e.g. in error messages.
	Name() string
	Available(context.Context, *url.URL) (bool, error)

	ImagePull(ctx context.Context, image string) error
	ImageExists(ctx context.Context, image string) (bool, error)
	ImageRemove(ctx context.Context, image string) error
	ImageLoader(ctx context.Context) imageload.Backend

	ContainerRun(ctx context.Context, name string, opts runOpts) error
	ContainerExec(ctx context.Context, name string, args []string) (string, string, error)
	ContainerDial(ctx context.Context, name string, args []string) (net.Conn, error)
	ContainerRemove(ctx context.Context, name string) error
	ContainerStart(ctx context.Context, name string) error
	ContainerExists(ctx context.Context, name string) (bool, error)
	ContainerLs(ctx context.Context) ([]string, error)
}

var errContainerAlreadyExists = errors.New("container already exists")

type runOpts struct {
	image string

	volumes []string
	env     []string
	ports   []string

	privileged bool

	cpus   string
	memory string
	gpus   bool

	args []string
}

// imageDriver creates and manages a container, then connects to it
type imageDriver struct {
	backend containerBackend
}

func (d *imageDriver) Available(ctx context.Context, u *url.URL) (bool, error) {
	return d.backend.Available(ctx, u)
}

func (d *imageDriver) Provision(ctx context.Context, target *url.URL, opts *client.DriverOpts) (client.Connector, error) {
	image, err := client.ParseImageTarget(target, "container")
	if err != nil {
		return nil, err
	}

	port, _ := strconv.Atoi(target.Query().Get("port"))
	target, err = d.create(ctx, containerCreateOpts{
		ImageTarget: image,
		volumeName:  target.Query().Get("volume"),
		port:        port,
		cpus:        target.Query().Get("cpus"),
		memory:      target.Query().Get("memory"),
	}, opts)
	if err != nil {
		return nil, err
	}
	return containerConnector{
		backend: d.backend,
		host:    target.Host,
		values:  target.Query(),
	}, nil
}

func (d *imageDriver) ImageLoader(ctx context.Context) imageload.Backend {
	return d.backend.ImageLoader(ctx)
}

type containerConnector struct {
	host    string
	values  url.Values
	backend containerBackend
}

// imageDriver connects to a container directly
type containerDriver struct {
	backend containerBackend
}

func (d *containerDriver) Available(ctx context.Context, u *url.URL) (bool, error) {
	if u.Host == "" {
		return false, fmt.Errorf("container engine address must name a container, got %q", u.Redacted())
	}

	if available, err := d.backend.Available(ctx, u); err != nil || !available {
		return false, err
	}

	exists, err := d.backend.ContainerExists(ctx, u.Host)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, &client.UnavailableError{Reason: fmt.Sprintf("%s: container %q not found", d.backend.Name(), u.Host)}
	}
	return true, nil
}

func (d *containerDriver) Provision(ctx context.Context, target *url.URL, opts *client.DriverOpts) (client.Connector, error) {
	return containerConnector{
		backend: d.backend,
		host:    target.Host,
		values:  target.Query(),
	}, nil
}

func (d *containerDriver) ImageLoader(ctx context.Context) imageload.Backend {
	return d.backend.ImageLoader(ctx)
}

func (d containerConnector) Connect(ctx context.Context) (net.Conn, error) {
	args := []string{}
	if context := d.values.Get("context"); context != "" {
		args = append(args, "--context="+context)
	}
	args = append(args, "buildctl", "dial-stdio")

	// using uncancelled context because context remains active for the
	// duration of the process, after dial has completed
	return d.backend.ContainerDial(context.WithoutCancel(ctx), d.host, args)
}

func (d containerConnector) EngineID() string {
	// not supported yet
	return ""
}

const (
	defaultDebugListenerAddress = "127.0.0.1:6060"
)

const InstrumentationLibrary = "dagger.io/client.drivers"

type containerCreateOpts struct {
	*client.ImageTarget

	volumeName string

	port int

	cpus   string
	memory string
}

// Pull the image and run it with a unique name tied to the pinned
// sha of the image. Remove any other containers leftover from
// previous executions of the engine at different versions (which
// are identified by looking for containers with the prefix
// "dagger-engine-").
func (d *imageDriver) create(ctx context.Context, opts containerCreateOpts, dopts *client.DriverOpts) (target *url.URL, rerr error) {
	ctx, span := otel.Tracer("").Start(ctx, "create container")
	defer telemetry.EndWithCause(span, &rerr)
	slog := slog.SpanLogger(ctx, InstrumentationLibrary)

	containerName := opts.Name

	leftoverEngines, err := d.collectLeftoverEngines(ctx, containerName)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		slog.Warn("failed to list containers", "error", err)
		leftoverEngines = []string{}
	}

	for i, leftoverEngine := range leftoverEngines {
		// if we already have a container with that name, attempt to start it
		if leftoverEngine == containerName {
			if err := d.backend.ContainerStart(ctx, leftoverEngine); err != nil {
				return nil, fmt.Errorf("failed to start container: %w", err)
			}
			d.garbageCollectEngines(ctx, opts.Cleanup, nil, slices.Delete(leftoverEngines, i, i+1))
			return &url.URL{Host: containerName}, nil
		}
	}

	// ensure the image is pulled
	exists, err := d.backend.ImageExists(ctx, opts.Ref)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect image: %w", err)
	}
	if !exists {
		if err := d.backend.ImagePull(ctx, opts.Ref); err != nil {
			return nil, fmt.Errorf("failed to pull image: %w", err)
		}
	}

	volume := distconsts.EngineDefaultStateDir
	if opts.volumeName != "" {
		volume = opts.volumeName + ":" + volume
	}

	runOptions := runOpts{
		image:      opts.Ref,
		volumes:    []string{volume},
		privileged: true,
		args:       []string{"--debug", "--debugaddr", defaultDebugListenerAddress},
		env:        opts.Env,
		cpus:       opts.cpus,
		memory:     opts.memory,
	}

	// mount the config path
	if _, err := os.Stat(client.EngineConfigPath); err == nil {
		runOptions.volumes = append(runOptions.volumes, client.EngineConfigPath+":"+config.DefaultConfigPath())
	} else if !errors.Is(err, os.ErrNotExist) {
		slog.Warn("could not stat config", "path", client.EngineConfigPath, "error", err)
	}
	// mount the certificates path
	if _, err := os.Stat(client.EngineCertificatesPath); err == nil {
		runOptions.volumes = append(runOptions.volumes, client.EngineCertificatesPath+":"+distconsts.EngineCustomCACertsDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		slog.Warn("could not stat certificates", "path", client.EngineCertificatesPath, "error", err)
	}

	if dopts.DaggerCloudToken != "" {
		runOptions.env = append(runOptions.env, fmt.Sprintf("%s=%s", client.EnvDaggerCloudToken, dopts.DaggerCloudToken))
	}
	if dopts.GPUSupport != "" {
		runOptions.gpus = true
		runOptions.env = append(runOptions.env, fmt.Sprintf("%s=%s", client.EnvGPUSupport, dopts.GPUSupport))
	}
	if opts.port != 0 {
		runOptions.ports = append(runOptions.ports, fmt.Sprintf("%d:%d", opts.port, opts.port))
		runOptions.args = append(runOptions.args, "--addr", fmt.Sprintf("tcp://0.0.0.0:%d", opts.port))
	}

	if err := d.backend.ContainerRun(ctx, containerName, runOptions); err != nil {
		// maybe someone else started the container simultaneously?
		if !errors.Is(err, errContainerAlreadyExists) {
			if exists, _ := d.backend.ContainerExists(ctx, containerName); !exists {
				return nil, fmt.Errorf("failed to run container: %w", err)
			}
		}
	}

	// garbage collect any other containers with the same name pattern, which
	// we assume to be leftover from previous runs of the engine using an older
	// version
	d.garbageCollectEngines(ctx, opts.Cleanup, nil, leftoverEngines)

	return &url.URL{Host: containerName}, nil
}

func (d *imageDriver) garbageCollectEngines(ctx context.Context, cleanup bool, preserveNames, engines []string) {
	if !cleanup {
		return
	}
	for _, engineName := range engines {
		if engineName == "" || slices.Contains(preserveNames, engineName) {
			continue
		}
		if err := d.backend.ContainerRemove(ctx, engineName); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
		}
	}
}

func (d *imageDriver) collectLeftoverEngines(ctx context.Context, additionalNames ...string) ([]string, error) {
	engines, err := d.backend.ContainerLs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers %s: %w", engines, err)
	}

	var filteredEngines []string
	for _, name := range engines {
		if strings.HasPrefix(name, client.EngineInstancePrefix) || slices.Contains(additionalNames, name) {
			filteredEngines = append(filteredEngines, name)
		}
	}
	return filteredEngines, nil
}
