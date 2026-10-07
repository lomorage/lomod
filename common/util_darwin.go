package common

import (
	"os"

	"bitbucket.org/lomoware/lomo-backend/common/cmd"
)

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
	return cmd.Exec("mv", src, dst)
}

// SyncDir flushes a directory entry (e.g. a file just renamed into it) to disk, so the
// rename survives a power loss.
func SyncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
