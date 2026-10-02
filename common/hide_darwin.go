package common

import (
	"os"
	"path/filepath"
)

// MkHideDir is to make directory hidden based on different OS
func MkHideDir(lomodTemp string, folderPerm os.FileMode) (string, error) {
	if err := os.MkdirAll(lomodTemp, folderPerm); err != nil {
		return lomodTemp, err
	}
	return lomodTemp, nil
}

// IsSystemHiddenFile is to check if one file is system default hidden file
func IsSystemHiddenFile(name string) bool {
	return name == ".DS_Store" || name == ".AppleDouble"
}

// IsHiddenFile reports whether the file at path name is hidden. Pass the
// path, not just the base name: on Windows hidden is a file attribute.
func IsHiddenFile(name string) bool {
	name = filepath.Base(name)
	return len(name) == 0 || name[0] == '.'
}
