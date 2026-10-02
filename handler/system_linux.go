package handler

import (
	"syscall"

	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/mackerelio/go-osstat/cpu"
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

	return &statfs{Bsize: uint32(stat.Bsize), Blocks: stat.Blocks, Bfree: stat.Bfree, Bavail: stat.Bavail}, err
}

func setTimeZone(tz string) error {
	_, err := cmd.RunWithSudo("timedatectl", "set-timezone", tz)
	return err
}

func getCPU() (types.CPU, error) {
	c := types.CPU{}
	cpus, err := cpu.Get()
	if err != nil {
		return c, err
	}
	c.Count = cpus.CPUCount
	return c, nil
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
