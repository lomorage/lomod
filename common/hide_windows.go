package common

import (
	"os"
	"syscall"
)

// MkHideDir is to make directory hidden based on different OS
func MkHideDir(lomodTemp string, folderPerm os.FileMode) (string, error) {
	if err := os.MkdirAll(lomodTemp, folderPerm); err != nil {
		return lomodTemp, err
	}
	lomodTempW, err := syscall.UTF16PtrFromString(lomodTemp)
	if err != nil {
		return lomodTemp, err
	}
	err = syscall.SetFileAttributes(lomodTempW, syscall.FILE_ATTRIBUTE_HIDDEN)
	if err != nil {
		return lomodTemp, err
	}
	return lomodTemp, nil
}

// IsSystemHiddenFile is to check if one file is system default hidden file
func IsSystemHiddenFile(name string) bool {
	return false
}

// IsHiddenFile reports whether the file at path name has the hidden attribute.
// name must be the file's path: a bare base name is looked up relative to the
// working directory.
func IsHiddenFile(name string) bool {
	pointer, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return false
	}
	attributes, err := syscall.GetFileAttributes(pointer)
	if err != nil {
		return false
	}
	return attributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0
}
