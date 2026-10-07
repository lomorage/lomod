package common

import "bitbucket.org/lomoware/lomo-backend/common/cmd"

// GetLomoGroupName returns lomogroup name
func GetLomoGroupName() string {
	return lomoGroupName
}

// SetLomoGroupName sets lomo group name
func SetLomoGroupName(g string) {
	lomoGroupName = g
}

// MoveFile move files using system command
func MoveFile(src, dst string) error {
	return cmd.Exec("move", src, dst)
}

// SyncDir is a no-op on Windows, which can't open a directory for flushing; NTFS journals
// renames itself.
func SyncDir(dir string) error {
	return nil
}
