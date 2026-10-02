package handler

import (
	"syscall"
	"unsafe"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/mackerelio/go-osstat/memory"
)

const (
	ffprobeBin  = "ffprobe.exe"
	ffmpegBin   = "ffmpeg.exe"
	avconvBin   = "avconv.exe"
	exiftoolBin = "exiftool.exe"
)

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

func getStatfs(dir string) (*statfs, error) {
	_, total, free, err := getSpace(dir)
	return &statfs{Bsize: 1, Blocks: total, Bfree: free}, err
}

func setTimeZone(tz string) error {
	return common.ErrNotImplementedFormat
}

func getCPU() (types.CPU, error) {
	return types.CPU{}, nil
}

func getMemory() (types.Memory, error) {
	m := types.Memory{}
	mem, err := memory.Get()
	if err != nil {
		return m, err
	}
	m.TotalInMB = mem.Total / mb
	m.UsedInMB = mem.Used / mb
	m.FreeInMB = mem.Free / mb

	return m, nil
}
