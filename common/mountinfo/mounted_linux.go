package mountinfo

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func normalizePath(path string) (realPath string, err error) {
	if realPath, err = filepath.Abs(path); err != nil {
		return "", fmt.Errorf("unable to get absolute path for %q: %v", path, err)
	}
	if realPath, err = filepath.EvalSymlinks(realPath); err != nil {
		return "", fmt.Errorf("failed to canonicalise path for %q: %v", path, err)
	}
	if _, err := os.Stat(realPath); err != nil {
		return "", fmt.Errorf("failed to stat target of %q: %v", path, err)
	}
	return realPath, nil
}

func mounted(path string) (bool, error) {
	path, err := normalizePath(path)
	if err != nil {
		if err == unix.ENOENT {
			// treat ENOENT as "not mounted"
			return false, nil
		}
		return false, err
	}
	entries, err := GetMounts(SingleEntryFilter(path))
	if err != nil {
		return false, err
	}

	return len(entries) > 0, nil
}
