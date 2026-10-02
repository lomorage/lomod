// +build linux darwin

package cmd

import (
	"os/exec"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// niceValue is how much lower priority background media-processing subprocesses
// (ffmpeg, exiftool) get relative to normal (0), so a burst of jobs doesn't
// starve interactive request handling for CPU on weak hardware like a Raspberry Pi.
const niceValue = 10

func applyLowPriority(c *exec.Cmd) {}

func lowerRunningPriority(pid int) {
	if err := unix.Setpriority(unix.PRIO_PROCESS, pid, niceValue); err != nil {
		logrus.Debugf("lower priority for pid %d: %v", pid, err)
	}
}
