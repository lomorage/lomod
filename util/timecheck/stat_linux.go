package main

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

func dumpInfo(info os.FileInfo) {
	stat := info.Sys().(*syscall.Stat_t)
	fmt.Println(time.Unix(stat.Atim.Sec, stat.Atim.Nsec))
	fmt.Println(time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec))
	fmt.Println(time.Unix(stat.Mtim.Sec, stat.Mtim.Nsec))
}
