// +build !windows,!linux,!freebsd,!openbsd freebsd,!cgo openbsd,!cgo

package mountinfo

import (
	"syscall"

	"bitbucket.org/lomoware/lomo-backend/common/types"
)

func parseMountTable(_ FilterFunc) ([]*Info, error) {
	return nil, nil
}

func mounted(path string) (bool, error) {
	return false, nil
}

// GetMountDir returns mount dir information
func GetMountDir(dir, p string) (types.MountDir, error) {
	stat := syscall.Statfs_t{}
	if err := syscall.Statfs(dir, &stat); err != nil {
		return types.MountDir{}, err
	}

	return types.MountDir{Dir: p, FreeSize: uint64(stat.Bsize) * stat.Bfree / uint64(mb),
		TotalSize: uint64(stat.Bsize) * stat.Blocks / uint64(mb)}, nil
}
