package mountinfo

import (
	"syscall"
	"unsafe"

	"bitbucket.org/lomoware/lomo-backend/common/types"
)

func parseMountTable(_ FilterFunc) ([]*Info, error) {
	// Do NOT return an error!
	return nil, nil
}

func mounted(_ string) (bool, error) {
	return false, nil
}

func getSpace(dir string) (uint64, uint64, uint64, error) {
	kernel32, err := syscall.LoadLibrary("Kernel32.dll")
	if err != nil {
		return 0, 0, 0, err
	}
	defer syscall.FreeLibrary(kernel32)

	GetDiskFreeSpaceEx, err := syscall.GetProcAddress(syscall.Handle(kernel32), "GetDiskFreeSpaceExW")
	if err != nil {
		return 0, 0, 0, err
	}

	lpFreeBytesAvailable := uint64(0)
	lpTotalNumberOfBytes := uint64(0)
	lpTotalNumberOfFreeBytes := uint64(0)

	_, _, errno := syscall.Syscall6(uintptr(GetDiskFreeSpaceEx), 4,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(dir))),
		uintptr(unsafe.Pointer(&lpFreeBytesAvailable)),
		uintptr(unsafe.Pointer(&lpTotalNumberOfBytes)),
		uintptr(unsafe.Pointer(&lpTotalNumberOfFreeBytes)), 0, 0)

	if errno != 0 {
		err = errno
	}
	return lpFreeBytesAvailable, lpTotalNumberOfBytes, lpTotalNumberOfFreeBytes, err
}

// GetMountDir returns mount dir information
func GetMountDir(dir, p string) (types.MountDir, error) {
	_, total, free, err := getSpace(dir)
	return types.MountDir{Dir: p, FreeSize: free / uint64(mb), TotalSize: total / uint64(mb)}, err
}

const (
	driveRemovable = 2
	driveFixed     = 3
)

// DriveInfo describes one local Windows drive letter.
type DriveInfo struct {
	// Root is the drive's root path, e.g. "E:\\".
	Root      string
	Removable bool
	FreeSize  uint64 // MB
	TotalSize uint64 // MB
}

// ListLocalDrives enumerates fixed and removable drive letters (skipping
// network and optical drives), with their free/total space. Windows has no
// /media-style mount pool the way Linux does, so this is how a desktop
// install discovers other physical/removable drives an account could be
// stored on beyond the single configured BaseDir/MountDir.
func ListLocalDrives() ([]DriveInfo, error) {
	kernel32, err := syscall.LoadLibrary("Kernel32.dll")
	if err != nil {
		return nil, err
	}
	defer syscall.FreeLibrary(kernel32)

	getLogicalDrives, err := syscall.GetProcAddress(syscall.Handle(kernel32), "GetLogicalDrives")
	if err != nil {
		return nil, err
	}
	getDriveType, err := syscall.GetProcAddress(syscall.Handle(kernel32), "GetDriveTypeW")
	if err != nil {
		return nil, err
	}

	bitmask, _, _ := syscall.Syscall(uintptr(getLogicalDrives), 0, 0, 0, 0)

	var drives []DriveInfo
	for i := 0; i < 26; i++ {
		if bitmask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + ":\\"
		driveType, _, _ := syscall.Syscall(uintptr(getDriveType), 1,
			uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(root))), 0, 0)
		removable := driveType == driveRemovable
		if driveType != driveFixed && !removable {
			continue
		}
		free, total, _, err := getSpace(root)
		if err != nil {
			// e.g. a card reader slot with no card inserted
			continue
		}
		drives = append(drives, DriveInfo{
			Root:      root,
			Removable: removable,
			FreeSize:  free / uint64(mb),
			TotalSize: total / uint64(mb),
		})
	}
	return drives, nil
}
