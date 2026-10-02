//go:build !windows
// +build !windows

package handler

func windowsDriveRoots() []string {
	return nil
}
