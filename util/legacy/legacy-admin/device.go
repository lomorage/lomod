package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"

	"bitbucket.org/lomoware/lomo-backend/common"
	"github.com/sirupsen/logrus"

	"github.com/gorilla/mux"

	_ "github.com/mattn/go-sqlite3"
)

// Device is response structure for getmountdevice request
type Device struct {
	UUID    string
	Name    string
	Mounted bool
	Total   string
	Free    string
	Path    string
}

func isUSBStorage(device string) bool {
	deviceVerifier := "ID_USB_DRIVER=usb-storage"
	out, err := exec.Command("udevadm", "info", "-q", "property", "-n", device).CombinedOutput()

	if err != nil {
		logrus.Infof("Error checking device %s: (%s)%s", device, string(out), err)
		return false
	}

	if strings.Contains(string(out), deviceVerifier) {
		return true
	}

	return false
}

// disk usage of path/disk
func diskUsage(path string) (uint64, uint64, error) {
	fs := syscall.Statfs_t{}
	err := syscall.Statfs(path, &fs)
	if err != nil {
		return 0, 0, err
	}
	total := fs.Blocks * uint64(fs.Bsize)
	free := fs.Bfree * uint64(fs.Bsize)
	return total, free, nil
}

const (
	// B is Byte
	B = 1
	// KB is k Byte
	KB = 1024 * B
	// MB is m Byte
	MB = 1024 * KB
	// GB is g Byte
	GB = 1024 * MB
)

func readDevice(p string) (Device, error) {
	device := Device{}
	out, err := exec.Command("lsblk", "-P", "-O", p).CombinedOutput()
	if err != nil {
		return device, err
	}

	parts := strings.Split(strings.TrimSpace(string(out)), " ")
	for _, part := range parts {
		kv := strings.Split(part, "=")
		if kv[0] == "UUID" {
			device.UUID = strings.Trim(kv[1], "\"")
		} else if kv[0] == "NAME" {
			device.Name = strings.Trim(kv[1], "\"")
		} else if kv[0] == "MOUNTPOINT" {
			device.Path = strings.Trim(kv[1], "\"")
		} else if kv[0] == "SIZE" {
			device.Total = strings.Trim(kv[1], "\"")
		}
	}

	if device.UUID == "" {
		// read using blkid
		out, err := exec.Command("blkid", "-s", "UUID", p).CombinedOutput()
		if err != nil {
			return device, err
		}
		parts := strings.Split(strings.TrimSpace(string(out)), " ")
		for _, part := range parts {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) != 2 {
				continue
			}
			if kv[0] == "UUID" {
				device.UUID = strings.Trim(kv[1], "\"")
			}
		}
	}
	if device.UUID == "" {
		return device, common.ErrDeviceNotLocate
	}
	if device.Path == "" {
		return device, nil
	}
	device.Mounted = true

	// calculate disk usage if it is mounted
	all, free, err := diskUsage(device.Path)
	if err != nil {
		logrus.Infof("check device stats got error: %v", err)
		return device, nil
	}
	device.Total = fmt.Sprintf("%.2fG", float64(all)/float64(GB))
	device.Free = fmt.Sprintf("%.2fG", float64(free)/float64(GB))
	return device, nil
}

func (h *Handler) listMounts(w http.ResponseWriter, r *http.Request) {
	devices := []Device{}
	maxDevices := 10
	for i := 1; i <= maxDevices; i++ {
		for _, base := range []string{"/dev/sda", "/dev/sdb", "/dev/sdc", "/dev/sdd"} {
			name := fmt.Sprintf("%s%d", base, i)
			if _, err := os.Stat(name); err != nil {
				continue
			}
			if !isUSBStorage(name) {
				continue
			}
			device, err := readDevice(name)
			if err != nil {
				logrus.Infof("error while reading devices /dev/sda%d : %v", i, err)
				continue
			}

			devices = append(devices, device)
		}
	}
	common.WriteBody(w, devices)
}

// TODO: add auto mount after restart https://gist.github.com/etes/aa76a6e9c80579872e5f
func (h *Handler) mount(w http.ResponseWriter, r *http.Request) {
	deviceID := mux.Vars(r)["deviceID"]
	// return if mounted already
	name := fmt.Sprintf("/dev/%s", deviceID)
	device, err := readDevice(name)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	if device.Path != "" {
		logrus.Infof("%s has been mounted at %s", deviceID, device.Path)
		return
	}

	// mount dir is based on the device's UUID
	newdir := path.Join(h.mountdir, device.UUID)
	if err := os.MkdirAll(newdir, 0600); err != nil {
		common.WriteError(w, err)
		return
	}

	out, err := exec.Command("sudo", "mount", "-o", "uid=pi", "-o", "gid=pi", name, newdir).CombinedOutput()
	if err != nil {
		logrus.Warnf("mount got %s : %v", string(out), err)
		common.WriteError(w, err)
		if err := os.Remove(newdir); err != nil {
			logrus.Warnf("clean up mount dir %s got error: %v", newdir, err)
		}
	}
	common.WriteBody(w, device.UUID)
}

func (h *Handler) umount(w http.ResponseWriter, r *http.Request) {
	p := mux.Vars(r)["path"]
	// get mount point
	out, err := exec.Command("sudo", "umount", path.Join(h.mountdir, p)).CombinedOutput()
	if err != nil {
		logrus.Warnf("umount got %s : %v", string(out), err)
		common.WriteError(w, err)
		return
	}
}

func (h *Handler) connectSamba(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	remote, err := url.Parse(fmt.Sprintf("smb://%s", q.Get("url")))
	if err != nil {
		common.WriteError(w, err)
		return
	}

	basename := fmt.Sprintf("%s%s", remote.Host, strings.Replace(remote.Path, "/", "_", -1))
	dirname := path.Join(h.mountdir, basename)
	if _, err := os.Stat(dirname); err != nil {
		if !os.IsNotExist(err) {
			common.WriteError(w, err)
			return
		}
		if err := os.Mkdir(dirname, 0700); err != nil {
			common.WriteError(w, err)
			return
		}
	}

	out, err := exec.Command("mount", "-o", "ro", "-t", "smbfs", remote.String(), dirname).CombinedOutput()
	if err != nil {
		logrus.Info(string(out))
		common.WriteError(w, err)
		if err := os.RemoveAll(dirname); err != nil {
			logrus.Warnf("after mount failure, remove %s, %v", dirname, err)
		}
		return
	}
	common.WriteBody(w, basename)
}
