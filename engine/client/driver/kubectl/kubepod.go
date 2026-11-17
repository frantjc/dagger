package kubectldriver

import (
	"cmp"
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	"github.com/dagger/dagger/engine/client"
	"github.com/dagger/dagger/engine/client/driver"
	"github.com/dagger/dagger/internal/buildkit/client/connhelper/kubepod"
	"github.com/dagger/dagger/util/traceexec"
	telemetry "github.com/dagger/otel-go"
)

var kubePodDriver = driver.NewDialDriver(kubepod.Helper, driver.AvailableFunc(kubePodAvailable))

func init() {
	client.RegisterDriver(kubePodDriver,
		"kube-pod",
	)
}

// access is a Kubernetes permission that the driver needs, in the form that
// `kubectl auth can-i` takes it.
type access struct {
	verb     string
	resource string
}

// the kube-pod driver connects with `kubectl exec`
var connectAccess = []access{
	{"create", "pods/exec"},
}

func kubePodAvailable(ctx context.Context, u *url.URL) error {
	spec, err := kubepod.SpecFromURL(u)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("kubectl"); err != nil {
		return fmt.Errorf("the kube-pod driver requires kubectl: %w", err)
	}
	if err := checkAccess(ctx, spec.Context, spec.Namespace, connectAccess...); err != nil {
		return err
	}

	args := append(kubectlScope(spec.Context, spec.Namespace), "get", "pod", spec.Pod, "-o", "name")
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	if _, stderr, err := traceexec.ExecOutput(ctx, cmd, telemetry.Encapsulated()); err != nil {
		return fmt.Errorf("pod %q is not usable: %w%s", spec.Pod, err, stderrDetail(stderr))
	}
	return nil
}

// checkAccess verifies that the cluster is reachable and that the current
// user may do everything in needs, in the given context and namespace (the
// kubeconfig defaults if empty).
func checkAccess(ctx context.Context, kctx, namespace string, needs ...access) error {
	scope := describeScope(kctx, namespace)
	for _, need := range needs {
		args := append(kubectlScope(kctx, namespace), "--request-timeout=10s", "auth", "can-i", need.verb, need.resource)
		cmd := exec.CommandContext(ctx, "kubectl", args...)
		// `auth can-i` exits non-zero both for "no" and for failing to ask, so
		// the answer on stdout is what tells them apart.
		stdout, stderr, err := traceexec.ExecOutput(ctx, cmd, telemetry.Encapsulated())
		answer := strings.ToLower(strings.TrimSpace(stdout))
		switch {
		case strings.HasPrefix(answer, "yes"):
		case strings.HasPrefix(answer, "no"):
			return fmt.Errorf("not permitted to %s %s in %s", need.verb, need.resource, scope)
		case err != nil:
			return fmt.Errorf("cluster in %s is not reachable: %w%s", scope, err, stderrDetail(stderr))
		default:
			return fmt.Errorf("unexpected answer %q checking permission to %s %s in %s", answer, need.verb, need.resource, scope)
		}
	}
	return nil
}

func kubectlScope(kctx, namespace string) []string {
	var args []string
	if kctx != "" {
		args = append(args, "--context="+kctx)
	}
	if namespace != "" {
		args = append(args, "--namespace="+namespace)
	}
	return args
}

func describeScope(kctx, namespace string) string {
	kctx = cmp.Or(kctx, "the current context")
	if namespace == "" {
		return kctx
	}
	return fmt.Sprintf("%s, namespace %q", kctx, namespace)
}

func stderrDetail(stderr string) string {
	if stderr = strings.TrimSpace(stderr); stderr != "" {
		return ": " + stderr
	}
	return ""
}
