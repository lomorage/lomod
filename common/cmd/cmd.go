package cmd

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"os/user"
	"strings"

	"github.com/pkg/errors"
)

// Exec runs the command without sudo and output.
func Exec(cmd string, args ...string) error {
	_, err := Run(cmd, args...)
	return err
}

// ExecContext runs the command without sudo and output.
func ExecContext(ctx context.Context, cmd string, args ...string) error {
	_, err := RunContext(ctx, cmd, args...)
	return err
}

// ExecWithSudo runs the command without sudo and output.
func ExecWithSudo(cmd string, args ...string) error {
	_, err := RunWithSudo(cmd, args...)
	return err
}

// RunWithSudo runs the command in sudo mode
// If user is already sudo, not run sudo command. Useful in container env.
func RunWithSudo(cmd string, args ...string) ([]byte, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	if u.Username == "root" {
		return Run(cmd, args...)
	}
	return Run("sudo", append([]string{cmd}, args...)...)
}

func retErr(err error, out, cmd string, args ...string) error {
	t := cmd
	if len(args) > 0 {
		t = t + " " + strings.Join(args, " ")
	}
	return errors.Wrapf(err, "%s:  %s", t, out)
}

// Run runs the command without sudo.
func Run(cmd string, args ...string) ([]byte, error) {
	return RunContext(context.Background(), cmd, args...)
}

// RunContext runs the command with context.
func RunContext(ctx context.Context, cmd string, args ...string) ([]byte, error) {
	buf := &bytes.Buffer{}
	c := exec.CommandContext(ctx, cmd, args...)
	c.Stdout = buf
	c.Stderr = buf
	if err := c.Run(); err != nil {
		return nil, retErr(err, buf.String(), cmd, args...)
	}
	return buf.Bytes(), nil
}

// RunContextWriter runs the command with context.
func RunContextWriter(ctx context.Context, w io.Writer, cmd string, args ...string) error {
	c := exec.CommandContext(ctx, cmd, args...)
	c.Stdout = w
	c.Stderr = w
	return c.Run()
}

// CombinedOutputLowPriority behaves like exec.Cmd's CombinedOutput, but runs the
// command at reduced OS scheduling priority so heavy, non-interactive work
// (media transcoding, EXIF extraction) doesn't starve interactive request
// handling for CPU during bursts of activity.
func CombinedOutputLowPriority(c *exec.Cmd) ([]byte, error) {
	buf := &bytes.Buffer{}
	c.Stdout = buf
	c.Stderr = buf
	applyLowPriority(c)
	if err := c.Start(); err != nil {
		return buf.Bytes(), err
	}
	lowerRunningPriority(c.Process.Pid)
	err := c.Wait()
	return buf.Bytes(), err
}

// RunLowPriorityContext runs the command with context at reduced OS scheduling
// priority. See CombinedOutputLowPriority.
func RunLowPriorityContext(ctx context.Context, cmd string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, cmd, args...)
	out, err := CombinedOutputLowPriority(c)
	if err != nil {
		return nil, retErr(err, string(out), cmd, args...)
	}
	return out, nil
}

// ExecLowPriorityContext runs the command with context at reduced OS scheduling
// priority, discarding output. See CombinedOutputLowPriority.
func ExecLowPriorityContext(ctx context.Context, cmd string, args ...string) error {
	_, err := RunLowPriorityContext(ctx, cmd, args...)
	return err
}

// ExecLowPriority runs the command at reduced OS scheduling priority,
// discarding output. See CombinedOutputLowPriority.
func ExecLowPriority(cmd string, args ...string) error {
	return ExecLowPriorityContext(context.Background(), cmd, args...)
}

// RunWithStdin runs the command with given stdin.
func RunWithStdin(reader io.Reader, cmd string, args ...string) ([]byte, error) {
	buf := &bytes.Buffer{}
	c := exec.Command(cmd, args...)
	c.Stdout = buf
	c.Stdin = reader
	if err := c.Run(); err != nil {
		return nil, retErr(err, buf.String(), cmd, args...)
	}
	return buf.Bytes(), nil
}

// RunWithStdinSudo runs the command with given stdin in sudo mode.
func RunWithStdinSudo(reader io.Reader, cmd string, args ...string) ([]byte, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	if u.Username == "root" {
		return RunWithStdin(reader, cmd, args...)
	}
	return RunWithStdin(reader, "sudo", append([]string{cmd}, args...)...)
}
