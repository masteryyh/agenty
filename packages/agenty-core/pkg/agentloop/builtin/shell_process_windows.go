//go:build windows

package builtin

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
)

func prepareShellProcess(process *exec.Cmd) {
	process.Cancel = func() error {
		if process.Process == nil {
			return nil
		}
		pid := strconv.Itoa(process.Process.Pid)
		if err := exec.Command("taskkill.exe", "/PID", pid, "/T", "/F").Run(); err == nil {
			return nil
		}
		if err := process.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		return os.ErrProcessDone
	}
	process.WaitDelay = shellWaitDelay
}
