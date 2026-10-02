// +build !linux

package preview

// lowerWorkerThreadPriority is only implemented on Linux, where per-thread
// niceness is available; it's a no-op elsewhere.
func lowerWorkerThreadPriority() {}
