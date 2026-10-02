package handler

import (
	"syscall"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/mackerelio/go-osstat/memory"
)

const (
	ffprobeBin  = "ffprobe"
	ffmpegBin   = "ffmpeg"
	avconvBin   = "avconv"
	exiftoolBin = "exiftool"
)

func getStatfs(dir string) (*statfs, error) {
	stat := syscall.Statfs_t{}
	err := syscall.Statfs(dir, &stat)

	return &statfs{Bsize: stat.Bsize, Blocks: stat.Blocks, Bfree: stat.Bfree, Bavail: stat.Bavail}, err
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
	m.CachedInMB = mem.Cached / mb
	m.FreeInMB = mem.Free / mb

	return m, nil
}
