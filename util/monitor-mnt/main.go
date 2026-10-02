package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common/mountinfo"
	"bitbucket.org/lomoware/lomo-backend/common/udev"
)

func main() {
	base := "/media"
	if len(os.Args) > 1 {
		base = os.Args[1]
	}
	conn, err := udev.NewConn(udev.UdevEvent)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	done := make(chan struct{})
	queue := make(chan udev.UEvent)
	go func() {
		err := conn.Monitor(context.Background(), queue)
		if err != nil {
			log.Println(err)
		}
		done <- struct{}{}
	}()

	mountDirs, err := listMounts(base)
	if err != nil {
		log.Fatal(err)
	}
	deviceUUIDs := map[string]string{}
	for _, dir := range mountDirs {
		uuid, err := mountinfo.GetDeviceUUID(dir)
		if err != nil {
			log.Printf("get %s UUID: %v", dir.Source, err)
			continue
		}
		deviceUUIDs[dir.Source] = uuid
	}
	log.Printf("current mounted device: %v", deviceUUIDs)

	for {
		select {
		case <-done:
			return
		case ev := <-queue:
			switch ev.Action {
			case udev.ADD:
				log.Printf("Add new device: %v, sleep 10 seconds to probe", ev)
				time.Sleep(10 * time.Second)
				currMountDirs, err := listMounts(base)
				if err != nil {
					log.Fatal(err)
				}
				for n, info := range currMountDirs {
					_, ok := mountDirs[n]
					if ok {
						log.Printf("%s is exist, skip", n)
						continue
					}
					deviceID, err := mountinfo.GetDeviceUUID(info)
					if err != nil {
						log.Printf("get %s device ID: %v", info.Source, err)
					}
					log.Printf("%s - %v - %s is mounted", n, *info, deviceID)
				}
				mountDirs = currMountDirs
			case udev.REMOVE:
				log.Printf("Remove device: %v, sleep 10 seconds to probe", ev)
				time.Sleep(10 * time.Second)
				currMountDirs, err := listMounts(base)
				if err != nil {
					log.Fatal(err)
				}
				for n, info := range mountDirs {
					_, ok := currMountDirs[n]
					if ok {
						log.Printf("%s is still exist", n)
						continue
					}
					log.Printf("%s - %s is unmounted", n, info.Source)
				}
				mountDirs = currMountDirs
			}
		}
		log.Printf("current mount dirs: %v", mountDirs)
	}
}

func listMounts(base string) (map[string]*mountinfo.Info, error) {
	mounts, err := mountinfo.GetMounts(nil)
	if err != nil {
		return nil, err
	}

	mountDirs := map[string]*mountinfo.Info{}
	for _, m := range mounts {
		//logrus.Printf("---- %+v\n", m)
		if strings.HasPrefix(m.Mountpoint, base) {
			mountDirs[m.Mountpoint] = m
		}
	}

	return mountDirs, nil
}
