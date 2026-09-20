//go:build windows

package builtin

import (
	"context"
	"os/exec"
)

func newShellCommand(ctx context.Context, command string) *exec.Cmd {
	for _, executable := range []string{"pwsh.exe", "powershell.exe"} {
		if path, err := exec.LookPath(executable); err == nil {
			return exec.CommandContext(ctx, path, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command)
		}
	}

	return exec.CommandContext(ctx, "cmd.exe", "/D", "/S", "/C", command)
}
