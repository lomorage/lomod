package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"bitbucket.org/lomoware/lomo-backend/common/testutil"
)

func main() {
	if len(os.Args) != 3 {
		log.Fatalf("usage: create-dummy-usb <mount dir> <serial no>")
	}
	if err := testutil.Unmount(filepath.Join(os.Args[1], os.Args[2])); err != nil {
		log.Println(err)
	}
	if err := testutil.UnloadMassStorage(); err != nil {
		log.Println(err)
	}

	done := make(chan testutil.MountNotify)
	go func() {
		testutil.MonitorMassStorageMount(context.Background(), os.Args[1], done)
	}()
	filename, err := testutil.CreateAndMount(64*1024*1024, os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("dummy file is created: %s", filename)

	notify := <-done

	log.Printf("%s is mounted to %v", filename, notify)
}
