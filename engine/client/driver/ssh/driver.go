package sshdriver

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"

	"github.com/dagger/dagger/engine/client"
	"github.com/dagger/dagger/engine/client/driver"
	"github.com/dagger/dagger/internal/buildkit/client/connhelper/ssh"
)

func init() {
	client.RegisterDriver(driver.NewDialDriver(ssh.Helper, driver.AvailableFunc(available)), "ssh")
}

func available(_ context.Context, u *url.URL) error {
	if u.Hostname() == "" {
		return fmt.Errorf("ssh engine address must be ssh://[USER@]HOST[:PORT], got %q", u.Redacted())
	}
	if _, err := exec.LookPath("ssh"); err != nil {
		return fmt.Errorf("the ssh driver requires an ssh client: %w", err)
	}
	return nil
}
