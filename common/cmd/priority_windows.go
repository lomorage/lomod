package cmd

import (
	"os/exec"
	"syscall"
)

// belowNormalPriorityClass lowers a Windows process's scheduling priority one
// notch so background media-processing subprocesses (ffmpeg, exiftool) don't
// starve interactive request handling for CPU during bulk uploads.
const belowNormalPriorityClass = 0x00004000

func applyLowPriority(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: belowNormalPriorityClass}
}

func lowerRunningPriority(pid int) {}
