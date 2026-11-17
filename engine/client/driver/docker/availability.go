package dockerdriver

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/dagger/dagger/engine/client"
	"github.com/dagger/dagger/util/traceexec"
	telemetry "github.com/dagger/otel-go"
)

// containerRuntimeAvailable reports whether command is installed and its
// daemon answers args. A missing binary is not an error, so that the next
// driver registered for the scheme gets a turn; a binary that is present but
// unhealthy is, and carries guidance for the user.
func containerRuntimeAvailable(ctx context.Context, command string, args ...string) (bool, error) {
	if _, err := exec.LookPath(command); err != nil {
		return false, nil //nolint:nilerr
	}

	cmd := exec.CommandContext(ctx, command, args...)
	if err := traceexec.Exec(ctx, cmd, telemetry.Encapsulated()); err != nil {
		if ctx.Err() != nil {
			return false, err
		}
		message := fmt.Sprintf("Dagger could not start the local engine using %s.\n\n"+
			"Make sure the runtime is running and your user can access it.\n"+
			"Run `%s` to diagnose the problem.\n"+
			"To select another engine, see `dagger help engine`.", command, strings.Join(cmd.Args, " "))
		return false, &client.RuntimeUnavailableError{Err: err, Message: message}
	}
	return true, nil
}
