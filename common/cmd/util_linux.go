package cmd

import "github.com/pkg/errors"

// DeleteFile delete file
func DeleteFile(file string) error {
	return ExecWithSudo("rm", file)
}

// Chown changes file owner
func Chown(owner string, files ...string) error {
	return ExecWithSudo("chown", append([]string{"-h", owner}, files...)...)
}

// ChownDir changes owner of file or dire
func ChownDir(owner, dir string) error {
	return ExecWithSudo("chown", "-R", owner, dir)
}

// Link sets up soft link. Can not use system link API because we need -f option
func Link(src, target string) error {
	return ExecWithSudo("ln", "-f", "-s", src, target)
}

// Poweroff power off the server
func Poweroff(reboot bool) error {
	args := []string{"+1"}
	if reboot {
		args = append(args, "-r")
	}
	out, err := RunWithSudo("shutdown", args...)
	if err != nil {
		return errors.Wrap(err, string(out))
	}
	return nil
}
