package main

import (
	"fmt"
	"os"
)

func main() {
	/* Open and parse the file. */
	info, err := os.Stat(os.Args[1])
	if err != nil {
		fmt.Println(err)
		return
	}
	dumpInfo(info)
}
