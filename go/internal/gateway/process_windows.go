//go:build windows

package gateway

import "os/exec"

func configurePythonCommand(_ *exec.Cmd) {}

func killPythonCommand(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
