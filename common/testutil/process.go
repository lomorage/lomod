package testutil

import (
	"context"
	"io"
	"os"
	"os/exec"
	"syscall"
)

// PkillBinary kills one single binaries process.
func PkillBinary(binary string) error {
	return exec.Command("pkill", "-x", binary).Run()
}

// PgrepDaemon greps one single daemon process.
func PgrepDaemon(daemon string) error {
	return exec.Command("pgrep", "-x", daemon).Run()
}

// StartDaemon starts a daemon process with specified context.
func StartDaemon(ctx context.Context, cancel context.CancelFunc, attr *syscall.SysProcAttr,
	bin string, args ...string) (int, error) {
	cmd := exec.Command(bin, args...)
	if attr != nil {
		cmd.SysProcAttr = attr
	}

	out, err := cmd.StdoutPipe()
	if err != nil {
		return -1, err
	}
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return -1, err
	}

	go io.Copy(os.Stdout, out)
	go io.Copy(os.Stderr, errPipe)

	if err := cmd.Start(); err != nil {
		return -1, err
	}

	go func(cancel context.CancelFunc) {
		cmd.Wait()
		cancel()
	}(cancel)

	go func(ctx context.Context) {
		<-ctx.Done()
		if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
			cmd.Process.Kill()
		}
	}(ctx)

	return cmd.Process.Pid, nil
}
