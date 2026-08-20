//go:build !windows && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package builtin

import "os/exec"

func prepareShellProcess(process *exec.Cmd) {
	process.WaitDelay = shellWaitDelay
}
