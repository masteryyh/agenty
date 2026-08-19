//go:build !windows

package builtin

import (
	"context"
	"os/exec"
)

func newShellCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "/bin/sh", "-c", command)
}
