package handler

import (
	"io/ioutil"
	"log"
	"net"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"bitbucket.org/lomoware/lomo-backend/common/mountinfo"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/udev"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var defaultRemoteMountUUID = mkMntUUID(types.MountNW, mountinfo.DefaultRemoteMountUUID)

func umount(p string) error {
	if !filepath.IsAbs(p) {
		p = "/" + p
	}
	out, err := cmd.RunWithSudo("umount", p)
	if err != nil {
		return errors.Wrap(err, string(out))
	}
	return nil
}

func newMountByUUID(uuid string) types.MountDir {
	m := types.MountDir{UUID: uuid}
	if uuid != "" {
		_, err := net.ParseMAC(uuid)
		if err == nil {
			m.Type = types.MountNW
		} else {
			m.Type = types.MountUSB
		}
	} else {
		m.Type = types.MountLocal
	}
	return m
}

func listMounts(base string) (map[string]*types.MountDir, error) {
	// TODO: mainly for UT usage, and need replace with real mount
	if testMnt != nil {
		return *testMnt, nil
	}
	mounts, err := mountinfo.GetMounts(nil)
	if err != nil {
		return nil, err
	}

	mountDirs := map[string]*types.MountDir{}
	defer logrus.Tracef("found mount dirs: %+v", mountDirs)

	baseIsSubDir := false
	for _, m := range mounts {
		// skip docker overlay mount
		if m.Source == "overlay" {
			continue
		}

		if !strings.HasPrefix(m.Mountpoint, base) {
			if !strings.Contains(base, m.Mountpoint) {
				continue
			}
			baseIsSubDir = true
		}

		logrus.Tracef("process mount: %s", m.RawLine)
		uuid, err := mountinfo.GetDeviceUUID(m)
		if err != nil {
			logrus.Tracef("Unable to find uuid for %s", m.Mountpoint)
			continue
		}
		md := newMountByUUID(uuid)
		if baseIsSubDir {
			md.Dir = base
			mountDirs[base] = &md

			// create the 2nd one in case existing users use old mount point
			mh := newMountByUUID(uuid)
			mh.Dir = m.Mountpoint
			mountDirs[m.Mountpoint] = &mh
			continue
		}
		md.Dir = m.Mountpoint
		mountDirs[m.Mountpoint] = &md
	}
	if baseIsSubDir {
		return mountDirs, nil
	}

	if len(mountDirs) == 0 {
		/*
			data, err := ioutil.ReadFile(mountinfo.MountTable)
			if err != nil {
				logrus.Tracef("read %s: %s", mountinfo.MountTable, err)
			} else {
				logrus.Debug(string(data))
			}
		*/
	}

	_, ok := mountDirs[base]
	if ok {
		return mountDirs, nil
	}
	mountDirs[base] = &types.MountDir{Type: types.MountLocal, Dir: base}
	return mountDirs, nil
}

func getMount(dir, p string) (types.MountDir, error) {
	stat := syscall.Statfs_t{}
	if err := syscall.Statfs(dir, &stat); err != nil {
		return types.MountDir{}, err
	}

	return types.MountDir{Dir: p, FreeSize: uint64(stat.Bsize) * stat.Bfree / uint64(mb),
		TotalSize: uint64(stat.Bsize) * stat.Blocks / uint64(mb)}, nil
}

/*
 1. during start,
    1.1 scan all directories under mount directory, and find their mount type (local, usb, remote)+UUID
    1.2 store above result into map in memory
    1.3 migration: if each user's home and backup directory haven't corresponding UUID in DB, then find UUID in above map by disk mount directory
    1.3.1 if not found in map (USB disk lost), log one error and continue. (prompt user about the failure in system API, and deny new import with special error code)
    - if disk is broken, contact support
    - change home directory API and client? TBD
    else if it is USB mount or network mount, create this file /media/disk1/alice/.lomo/uuid
    else if it is not PI, create this file /media/disk1/alice/.lomo/uuid
    else prompt user about the failure in system API, and deny new import with special error code
    ** no need scan the user's media directory, calculate merkle tree , and see if it matches the records in DB because only raspberry pi user has the problem. (compare the whole merkle tree, or just one year? what about user import new photos in the middle of compute?)

    1.4 compare UUID in user DB with scan result
    1.4.1 if one user's disk UUID is same, continue
    1.4.2 else block user's any new import (return 500 with error code)

 2. monitor udev
    2.1 when one device or remote NFS mount is removed, run 1.1
    - if moved device or NFS mount is used by one user, block user's any new import (return 500 with error code; prompt user about the failure in system API, and deny new import with special error code
    2.2 when one device or remote NFS mount is inserted, run 1.1
    - if added device or NFS mount is used by one user, unblock user's new import
    - corner cases, if USB disks are not mounted during migration but inserted later, follow migration rules to unblock the user
*/
// extraStorageRoots is a no-op here: MountDir is already the OS-level
// /media mount pool on this platform, so every physical disk already shows
// up as one of its subfolders via listMounts, with no separate mechanism
// needed.
func extraStorageRoots(base string) []*types.MountDir {
	return nil
}

func (h *Handler) monitorMount() {
	currMountDirs := h.initMonitorMount()

	for username, mi := range h.userMountStatus {
		h.migrateUserMnt(username, mi)
	}

start:
	conn, err := udev.NewConn(udev.UdevEvent)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	done := make(chan struct{})
	queue := make(chan udev.UEvent)
	go func() {
		err := conn.Monitor(h.gCtx, queue)
		if err != nil {
			logrus.Errorf("udev monitor: %s", err)
		} else {
			logrus.Warn("udev monitor finished without error")
		}
		done <- struct{}{}
	}()

	var probeMountDirs map[string]*types.MountDir
	for {
		h.checkUserMounts(currMountDirs)

		select {
		case <-h.gCtx.Done():
			return
		case <-done:
			goto start
		case ev := <-queue:
			switch ev.Action {
			case udev.ADD:
				if ev.SubSystem != udev.SubsystemBlock && ev.SubSystem != udev.SubsystemBDI {
					logrus.Tracef("skip add udev event: %s", ev.Pretty())
				} else {
					logrus.Tracef("process add udev event: %s", ev.Pretty())
				}
			case udev.REMOVE:
				if ev.SubSystem != udev.SubsystemBlock && ev.SubSystem != udev.SubsystemBDI {
					logrus.Tracef("skip remove udev event: %s", ev.Pretty())
				} else {
					logrus.Tracef("process remove udev event: %s", ev.Pretty())
				}
			}
		}
		foundNew := false
		for i := 0; i < 15; i++ {
			probeMountDirs, err = listMounts(h.conf.MountDir)
			if err != nil {
				logrus.Tracef("find new udev event, but listing mount device: %s", err)
			} else if len(probeMountDirs) != len(currMountDirs) {
				logrus.Tracef("find new udev event, start check mountedDirs")
				foundNew = true
				currMountDirs = probeMountDirs
				break
			}
			time.Sleep(2 * time.Second)
		}
		if !foundNew {
			logrus.Tracef("No change on mount dirs: %v", probeMountDirs)
			currMountDirs = probeMountDirs
		}
	}
}

func (h *Handler) migrateUserMnt(username string, userMnt *userMountInfo) {
	if userMnt.homeErr != nil {
		logrus.Tracef("user %s's home mount has error: %v", username, userMnt.homeErr)
	} else {
		_, err := h.migrateMntUUID(username, &userMnt.homeMnt)
		if err != nil {
			logrus.Tracef("user %s's home mount migration: %v", username, err)
			userMnt.homeErr = err
		}
	}
	if userMnt.backupErr != nil {
		logrus.Tracef("user %s's backup mount has error: %v", username, userMnt.backupErr)
	} else if userMnt.backupMnt != nil {
		_, err := h.migrateMntUUID(username, userMnt.backupMnt)
		if err != nil {
			logrus.Tracef("user %s's backup mount has error: %v", username, userMnt.backupErr)
		}
	}
}

func (h *Handler) checkUserMounts(mountDirs map[string]*types.MountDir) {
	// XXX: is it possible disk no response and causing the lock never release
	// TODO:  add one context here to unlock in case disk is very bad
	h.userMountLock.Lock()
	for username, mi := range h.userMountStatus {
		if err := h.checkUserMount(mountDirs, username, &mi.homeMnt, mi.homeErr); err != nil {
			logrus.Tracef("user %s's home dir %s mount check: %v", username, mi.homeMnt.Dir, err)
			mi.homeErr = err
		} else {
			mi.homeErr = nil
		}
		if mi.backupMnt == nil {
			continue
		}
		if err := h.checkUserMount(mountDirs, username, mi.backupMnt, mi.backupErr); err != nil {
			logrus.Tracef("user %s's backup dir %s mount check: %v", username, mi.backupMnt.Dir, err)
			mi.backupErr = err
		} else {
			mi.backupErr = nil
		}
	}
	h.userMountLock.Unlock()
}

func (h *Handler) checkUserMount(mountDirs map[string]*types.MountDir, username string,
	mi *types.MountDir, prevErr error) error {
	md, ok := mountDirs[mi.Dir]
	logrus.Tracef("check user %s mount: %+v - %+v", username, mi, md)
	if prevErr == common.ErrDeviceNotMount {
		// lookup from current mountDirs and see if device is mounted or not
		if !ok {
			logrus.Tracef("user %s's dir %s is still not found in mount dirs: %+v", username, mi.Dir, mountDirs)
			return common.ErrDeviceNotMount
		}
		logrus.Tracef("user %s's unmounted dir %s is re-mounted in dirs: %+v", username, mi.Dir, mountDirs)
	}
	if !ok {
		if mi.Type == types.MountUSB || mi.Type == types.MountNW {
			logrus.Tracef("user %s's dir %s is not found in mount dirs: %+v", username, mi.Dir, mountDirs)
			return common.ErrDeviceNotMount
		}
		if !h.conf.DisableMountMon && (runtime.GOARCH == "arm" || runtime.GOARCH == "arm64") {
			logrus.Tracef("user %s's mount dir %s not found in arm machine", username, mi.Dir)
			return common.ErrInvalidMntSBC
		}
	}
	if md == nil {
		return common.ErrDeviceNotMount
	}
	// user disk should be usb, but now it is local fs, return directly
	if mi.Type != types.MountLocal && md.Type == types.MountLocal {
		return common.ErrDeviceNotMount
	}

	if (mi.Type == types.MountLocal && md.Type != types.MountLocal) ||
		(mi.UUID == "" && mi.Type == "") {
		logrus.Tracef("try migrate user %s mount info", username)
		// do migration again regardless prior error
		exist, err := h.migrateMntUUID(username, md)
		if err != nil {
			logrus.Tracef("user %s's mount has error: %v", username, err)
			return err
		} else if exist {
			return nil
		}
	}
	return h.compareMntUUID(username, mi.Dir, md)
}

func (h *Handler) compareMntUUID(username, baseDir string, curMnt *types.MountDir) error {
	f := filepath.Join(common.GetUserConfDir(baseDir, username), types.UUIDFilename)
	contents, err := ioutil.ReadFile(f)
	if err != nil {
		logrus.Tracef("error to read mount uuid file: %s", err)
		return common.ErrInvalidMntSBC
	}
	curUUID := mkMntUUID(curMnt.Type, curMnt.UUID)
	curTrimSpace := strings.TrimSpace(string(curUUID))
	cur := strings.Replace(curTrimSpace, "\"", "", -1)
	expTrimSpace := strings.TrimSpace(string(contents))
	exp := strings.Replace(expTrimSpace, "\"", "", -1)

	if cur != exp {
		logrus.Tracef("expect UUID: %s, got %s", exp, cur)
		if cur == defaultRemoteMountUUID || exp == defaultRemoteMountUUID {
			logrus.Trace("skip default remote mount")
			return nil
		}
		return common.ErrInvalidMntSBC
	}

	return nil
}
