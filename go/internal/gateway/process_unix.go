//go:build !windows

package gateway

import (
	"os/exec"
	"syscall"
)

func configurePythonCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killPythonCommand(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	if pid > 0 {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		return
	}
	_ = cmd.Process.Kill()
}
