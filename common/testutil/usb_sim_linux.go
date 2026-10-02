package testutil

import (
	"bytes"
	"context"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"bitbucket.org/lomoware/lomo-backend/common/udev"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	defaultVendor = "Lomorage"
)

// MountNotify is notification structure of monitor mount
type MountNotify struct {
	UUID      string
	MountPath string
}

// CreateLoopDevice creates loop device
func CreateLoopDevice(size int) (filename, loopDevice string, err error) {
	f, err := ioutil.TempFile("", "mount-")
	if err != nil {
		return
	}
	filename = f.Name()
	err = f.Close()
	if err != nil {
		return
	}

	logrus.Infof("sudo dd if=/dev/zero of=" + filename + " bs=512 count=" + strconv.Itoa(size/512))
	err = cmd.ExecWithSudo("dd", "if=/dev/zero", "of="+filename, "bs=512", "count="+strconv.Itoa(size/512))
	if err != nil {
		return
	}

	buf := bytes.NewBufferString("o\nn\np\n1\n\n\nw\n")
	_, err = cmd.RunWithStdinSudo(buf, "fdisk", filename)
	if err != nil {
		return
	}

	var ld []byte
	logrus.Infof("sudo losetup --show -f -o " + strconv.Itoa(2048*512) + " " + filename)
	ld, err = cmd.RunWithSudo("losetup", "--show", "-f", "-o", strconv.Itoa(2048*512), filename)
	if err != nil {
		return
	}

	loopDevice = strings.TrimSpace(string(ld))

	logrus.Infof("sudo /usr/sbin/mke2fs -t ext4 " + loopDevice)

	err = cmd.ExecWithSudo("/usr/sbin/mke2fs", "-t", "ext4", loopDevice)

	return
}

// CreateAndMount creates disk and mount to specified path.
func CreateAndMount(size int, serialNo string) (filename string, err error) {
	var loopDevice string
	filename, loopDevice, err = CreateLoopDevice(size)
	if err != nil {
		return
	}
	// detach loop device to mount using modprobe
	logrus.Infof("sudo losetup -d " + loopDevice)
	if err2 := cmd.ExecWithSudo("losetup", "-d", loopDevice); err2 != nil {
		if err == nil {
			err = err2
		} else {
			err = errors.Wrap(err, err2.Error())
		}
	}
	err = MountWithFile(filename, serialNo)
	return
}

// MountWithFile mount to specified path with given file
func MountWithFile(filename, serialNo string) error {
	logrus.Infof("sudo modprobe g_mass_storage file=" + filename +
		" stall=0 removable=1 idVendor=0x0781 idProduct=0x5572 bcdDevice=0x011a iManufacturer=\"" +
		defaultVendor + "\" iProduct=\"Simulate USB Drive\" iSerialNumber=\"" + serialNo + "\"")
	return cmd.ExecWithSudo("modprobe", "g_mass_storage", "file="+filename, "stall=0", "removable=1",
		"idVendor=0x0781", "idProduct=0x5572", "bcdDevice=0x011a", `iManufacturer="`+defaultVendor+`"`,
		`iProduct="Simulate USB Drive"`, `iSerialNumber="`+serialNo+`"`)
}

// UnloadMassStorage remove mass storage module
func UnloadMassStorage() error {
	return cmd.ExecWithSudo("rmmod", "g_mass_storage")
}

// Unmount unmount one folder
func Unmount(p string) error {
	return cmd.ExecWithSudo("umount", p)
}

// MonitorMassStorageMount monitor udev bus, mount new mass storage device and send notification
func MonitorMassStorageMount(ctx context.Context, mountDir string, notify chan MountNotify) error {
	conn, err := udev.NewConn(udev.UdevEvent)
	if err != nil {
		return err
	}
	defer conn.Close()

	done := make(chan error)
	queue := make(chan udev.UEvent)
	go func() {
		err := conn.Monitor(ctx, queue)
		if err != nil {
			logrus.Errorf("testutil monitor udev: %v", err)
		}
		done <- err
	}()

	newStorageDevices := map[string]string{}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-done:
			return err
		case ev := <-queue:
			// only monitor add action
			if ev.Action != udev.ADD {
				logrus.Printf("skip %s action event: %+v", ev.Action, ev)
				continue
			}
			switch ev.SubSystem {
			case udev.SubsystemUSB:
				logrus.Printf("Add new usb device: %v", ev)
				vendor, ok := ev.Env[udev.KeyVendor]
				if !ok {
					logrus.Printf("skip usb device without vendor: %+v", ev)
					continue
				}
				if vendor != defaultVendor {
					logrus.Printf("skip usb device not default one: %+v", ev)
					continue
				}
				serialno, ok := ev.Env[udev.KeySerialNo]
				if !ok {
					logrus.Printf("skip usb device without serial number: %+v", ev)
					continue
				}
				newStorageDevices[serialno] = vendor
			case udev.SubsystemBlock:
				logrus.Printf("Add new block device: %v", ev)
				devName, ok := ev.Env[udev.KeyDevName]
				if !ok || devName == "" {
					logrus.Printf("skip block device without device name: %+v", ev)
					continue
				}
				fsType, ok := ev.Env[udev.KeyFsType]
				if !ok || fsType == "" {
					logrus.Printf("skip block device without fstype: %+v", ev)
					continue
				}
				serialno, ok := ev.Env[udev.KeySerialNo]
				if !ok {
					logrus.Printf("skip block device without serial number: %+v", ev)
					continue
				}
				_, ok = newStorageDevices[serialno]
				if !ok {
					logrus.Printf("skip block device without corresponding usb device: %+v", ev)
					continue
				}
				p := filepath.Join(mountDir, serialno)
				err := cmd.ExecWithSudo("mkdir", "-p", p)
				if err != nil {
					logrus.Warnf("create mount directory %s: %v", p, err)
					continue
				}
				logrus.Infof("start mount %s -> %s", devName, p)
				err = cmd.ExecWithSudo("mount", devName, p)
				if err != nil {
					logrus.Warnf("mount %s: %v", p, err)
				}
				notify <- MountNotify{UUID: ev.Env[udev.KeyFsUUID], MountPath: p}
			}
		}
	}
}
