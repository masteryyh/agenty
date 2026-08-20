//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package builtin

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func prepareShellProcess(process *exec.Cmd) {
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	process.Cancel = func() error {
		if process.Process == nil {
			return nil
		}
		err := syscall.Kill(-process.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	process.WaitDelay = shellWaitDelay
}
