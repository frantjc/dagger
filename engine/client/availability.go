package client

import "fmt"

// RuntimeUnavailableError keeps diagnostic details in the trace while the CLI
// displays only the guidance, using its existing quiet-error convention.
type RuntimeUnavailableError struct {
	Err     error
	Message string
}

func (e *RuntimeUnavailableError) Error() string {
	return e.Message + "\n\nDetails: " + e.Err.Error()
}

func (e *RuntimeUnavailableError) Unwrap() error { return e.Err }

func (e *RuntimeUnavailableError) Extensions() map[string]any {
	return map[string]any{"_quiet": true, "_message": e.Message}
}

// UnavailableError says why a driver cannot serve a URL. Unlike other errors
// from Driver.Available, it does not stop OpenDriver from trying the next
// driver registered for the scheme; the reasons of all drivers that were tried
// are reported if none of them can serve the URL.
type UnavailableError struct {
	Reason string
}

func (e *UnavailableError) Error() string { return e.Reason }

func noRuntimeAvailable(scheme string) error {
	return &RuntimeUnavailableError{
		Err: fmt.Errorf("no container runtime is available for engine scheme %q", scheme),
		Message: "Dagger needs a container runtime to start the local engine.\n\n" +
			"Install a compatible container runtime and make sure its command is in PATH.\n" +
			"To choose a local or remote engine, run `dagger help engine`.",
	}
}
