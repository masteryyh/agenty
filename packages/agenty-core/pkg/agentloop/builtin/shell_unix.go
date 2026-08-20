//go:build !windows

package builtin

import (
	"context"
	"os/exec"
	"runtime"
)

func newShellCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, shellExecutable(), "-c", command)
}

func shellExecutable() string {
	preferred := "sh"
	switch runtime.GOOS {
	case "darwin":
		preferred = "zsh"
	case "linux":
		preferred = "bash"
	}
	if path := findShellExecutable(preferred); path != "" {
		return path
	}
	if preferred != "sh" {
		if path := findShellExecutable("sh"); path != "" {
			return path
		}
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		return "/bin/sh"
	}
	return "sh"
}

func findShellExecutable(name string) string {
	candidates := []string{name}
	if name != "" && name[0] != '/' {
		candidates = append(candidates, "/bin/"+name, "/usr/bin/"+name)
	}
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	return ""
}
