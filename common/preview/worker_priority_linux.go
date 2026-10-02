// +build linux

package preview

import (
	"runtime"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// lowerWorkerThreadPriority pins the preview worker goroutine to its OS thread
// and lowers that thread's scheduling priority, so libvips's synchronous,
// CPU-bound thumbnail generation doesn't compete evenly with interactive HTTP
// handling for CPU time on weak hardware like a Raspberry Pi.
func lowerWorkerThreadPriority() {
	runtime.LockOSThread()
	if err := unix.Setpriority(unix.PRIO_PROCESS, unix.Gettid(), 10); err != nil {
		logrus.Debugf("lower preview worker thread priority: %v", err)
	}
}
