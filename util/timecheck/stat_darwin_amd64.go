package main

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

func dumpInfo(info os.FileInfo) {
	stat := info.Sys().(*syscall.Stat_t)
	fmt.Println(time.Unix(stat.Atimespec.Sec, stat.Atimespec.Nsec))
	fmt.Println(time.Unix(stat.Ctimespec.Sec, stat.Ctimespec.Nsec))
	fmt.Println(time.Unix(stat.Mtimespec.Sec, stat.Mtimespec.Nsec))
}
