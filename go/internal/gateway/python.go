package gateway

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"
)

type PythonRuntime struct {
	cmd *exec.Cmd
}

func StartPython(ctx context.Context, cfg Config) (*PythonRuntime, error) {
	args := []string{
		"-m", "uvicorn", cfg.PythonApp,
		"--host", cfg.PythonHost,
		"--port", fmt.Sprintf("%d", cfg.PythonPort),
		"--workers", "1",
		"--proxy-headers",
	}
	cmd := exec.CommandContext(ctx, cfg.PythonBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")
	if cfg.PythonEnvFile != "" {
		cmd.Env = append(cmd.Env, "REBECCA_ENV_FILE="+cfg.PythonEnvFile)
	}
	configurePythonCommand(cmd)

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	runtime := &PythonRuntime{cmd: cmd}
	if err := waitForTCP(ctx, cfg.PythonAddr(), cfg.PythonStartTimeout); err != nil {
		runtime.Stop()
		return nil, err
	}
	return runtime, nil
}

func (r *PythonRuntime) Stop() {
	if r == nil || r.cmd == nil || r.cmd.Process == nil {
		return
	}
	killPythonCommand(r.cmd)
	_, _ = r.cmd.Process.Wait()
}

func waitForTCP(ctx context.Context, addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("python runtime did not become ready on %s within %s", addr, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}
