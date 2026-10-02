package handler

import (
	"context"
	"database/sql"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/mountinfo"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/sirupsen/logrus"
)

// TODO: mainly for UT usage, and need replace with real mount
var testMnt *map[string]*types.MountDir

type userMountInfo struct {
	homeMnt   types.MountDir
	homeErr   error
	backupMnt *types.MountDir
	backupErr error
}

func (h *Handler) umountDir(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	p, err := url.QueryUnescape(r.URL.Query().Get(common.QueryKeyPath))
	if err != nil {
		common.WriteError(w, err)
		return
	}
	if err := umount(p); err != nil {
		common.WriteError(w, err)
	}
}

func (h *Handler) listMountedDir(w http.ResponseWriter, r *http.Request) {
	userDirs := map[string]interface{}{}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		// no validation if zero users
		users, err := user.ListUsers(ctx, tx)
		if err != nil {
			return err
		}
		for _, u := range users.Users {
			if u.IsBotUser() {
				continue
			}
			userDirs[u.Name] = struct{}{}
		}
		if len(userDirs) == 0 {
			return nil
		}
		return w.(*logger.ResponseLogger).Err
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	_, err := os.Stat(h.conf.MountDir)
	if err != nil {
		if os.IsNotExist(err) || strings.Contains(err.Error(), "input/output error") {
			common.WriteError(w, common.ErrDeviceNotMount)
		} else {
			common.WriteError(w, err)
		}
		return
	}
	mountBase := filepath.Base(h.conf.MountDir)
	mounts := []types.MountDir{}
	dirs := []string{"."}
	files, err := ioutil.ReadDir(h.conf.MountDir)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	mountDirs, err := listMounts(h.conf.MountDir)
	if err != nil {
		logrus.Tracef("unable to list all mounts: %v", err)
	}

	for _, file := range files {
		if !file.IsDir() {
			logrus.Tracef("skip non-dir - %s at mount path\n", file.Name())
			continue
		}
		if common.IsHiddenFile(filepath.Join(h.conf.MountDir, file.Name())) {
			logrus.Tracef("skip hidden dir - %s at mount path\n", file.Name())
			continue
		}
		if common.IsReservedBaseDirName(file.Name()) {
			logrus.Tracef("skip lomod-reserved dir - %s at mount path\n", file.Name())
			continue
		}
		begin := rune(file.Name()[0])
		if !unicode.IsLetter(begin) && !unicode.IsDigit(begin) {
			logrus.Tracef("found non-alpha-digit folder - %s at mount path\n", file.Name())
			continue
		}
		_, ok := userDirs[file.Name()]
		if ok {
			// this is user directory, skip
			continue
		}
		dirs = append(dirs, file.Name())
	}
	for _, dir := range dirs {
		fullDir := ""
		retDir := ""
		if dir == "." {
			fullDir = h.conf.MountDir
			retDir = mountBase
		} else {
			fullDir = filepath.Join(h.conf.MountDir, dir)
			retDir = filepath.Join(mountBase, dir)
		}
		m, err := mountinfo.GetMountDir(fullDir, retDir)
		if err != nil {
			mounts = append(mounts, types.MountDir{Dir: retDir, Error: err.Error()})
			continue
		}
		// probe if the directory is writable or not
		probe := filepath.Join(fullDir, "lomod_probe_write")
		if err := os.MkdirAll(probe, h.conf.FolderPerm); err != nil {
			logrus.Errorf("failed to probe mount dir %s: %v", probe, err)
			m.Error = err.Error()
		} else if err := os.RemoveAll(probe); err != nil {
			logrus.Errorf("failed to remove probe mount dir %s: %v", probe, err)
		}

		md, ok := mountDirs[fullDir]
		if ok {
			m.Type = md.Type
			m.UUID = md.UUID
		} else {
			m.Type = types.MountLocal
		}
		mounts = append(mounts, m)
	}

	for _, m := range extraStorageRoots(h.conf.MountDir) {
		mounts = append(mounts, *m)
	}

	common.WriteBody(w, mounts)
}

func (h *Handler) initMonitorMount() map[string]*types.MountDir {
	var (
		mountDirs map[string]*types.MountDir
		err       error
	)
	for {
		mountDirs, err = listMounts(h.conf.MountDir)
		if err != nil {
			logrus.Tracef("listing mount device: %s", err)
			time.Sleep(30 * time.Second)
			continue
		}

		logrus.Tracef("initMonitorMount, mountDirs: %v", mountDirs)

		if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
			users, err := user.ListUsers(ctx, tx)
			if err != nil {
				return err
			}
			for _, u := range users.Users {
				if u.IsBotUser() {
					continue
				}
				mi := &userMountInfo{}
				mountDir := h.getUserMountDir(u.HomeDir, u.Name)
				logrus.Tracef("initMonitorMount for user %s, homeDir: %v, mountDir: %v", u.Name, u.HomeDir, mountDir)
				md, ok := mountDirs[mountDir]
				if !ok {
					if !h.conf.DisableMountMon && (runtime.GOARCH == "arm" || runtime.GOARCH == "arm64") {
						logrus.Tracef("user %s's home dir %s is not found in mount dirs: %v", u.Name, u.HomeDir, mountDirs)
						mi.homeErr = common.ErrDeviceNotMount
						mi.homeMnt = types.MountDir{Dir: mountDir}
					} else {
						mi.homeMnt = types.MountDir{Type: types.MountLocal, Dir: mountDir}
					}
				} else {
					mi.homeMnt = *md
				}
				if u.BackupDir != "" {
					mountDir = h.getUserMountDir(u.BackupDir, u.Name)
					md, ok = mountDirs[mountDir]
					if !ok {
						if !h.conf.DisableMountMon && (runtime.GOARCH == "arm" || runtime.GOARCH == "arm64") {
							logrus.Tracef("user %s's backup dir %s is not found in mount dirs: %v", u.Name, u.BackupDir, mountDirs)
							mi.backupErr = common.ErrDeviceNotMount
							mi.backupMnt = &types.MountDir{Dir: mountDir}
						} else {
							mi.backupMnt = &types.MountDir{Type: types.MountLocal, Dir: mountDir}
						}
					} else {
						mi.backupMnt = md
					}
				}

				h.userMountLock.Lock()
				logrus.Tracef("add userMountStatus for %s: %v", u.Name, mi)
				h.userMountStatus[u.Name] = mi
				h.userMountLock.Unlock()
			}
			return nil
		}); err == nil {
			break
		}
		logrus.Tracef("monitor mount try to get user disks, but got: %v", err)
		time.Sleep(30 * time.Second)
	}

	return mountDirs
}

func (h *Handler) migrateMntUUID(username string, mi *types.MountDir) (bool, error) {
	lomoUserConfDir := common.GetUserConfDir(mi.Dir, username)
	f := filepath.Join(lomoUserConfDir, types.UUIDFilename)
	stat, err := os.Stat(f)
	if err == nil {
		if stat.Size() != 0 {
			return true, nil
		}
		logrus.Tracef("user %s's %s uuid file is empty, retry migrate", username, f)
	} else if !os.IsNotExist(err) {
		logrus.Tracef("probe user %s's %s uuid file: %s", username, f, err)
	}

	// if Photos directory is not exist, it must not mounted yet
	// these directories should be create during user creation
	_, err = os.Stat(filepath.Join(filepath.Join(mi.Dir, username), common.AppPhoto))
	if err != nil && os.IsNotExist(err) {
		return false, common.ErrDeviceNotMount
	}

	if err := os.MkdirAll(lomoUserConfDir, h.conf.FolderPerm); err != nil {
		logrus.Tracef("Error to make user conf directory: %v", err)
		mi.Type = ""
		mi.UUID = ""
		return false, common.ErrMkdir
	}
	uuid := mkMntUUID(mi.Type, mi.UUID)
	if mi.Type == types.MountLocal && !h.conf.DisableMountMon && (runtime.GOARCH == "arm" || runtime.GOARCH == "arm64") {
		// raspberry pi or other SBC should not use local disk as home directory
		mi.Type = ""
		mi.UUID = ""
		return false, common.ErrInvalidMntSBC
	}
	if err := ioutil.WriteFile(f, []byte(uuid), h.conf.FilePerm); err != nil {
		logrus.Tracef("Error to create file: %v", err)
		mi.Type = ""
		mi.UUID = ""
		return false, common.ErrCreateFile
	}
	logrus.Tracef("write %s in file %s", uuid, f)
	return false, nil
}

func mkMntUUID(typ, uuid string) string {
	return typ + types.UUIDDelimiter + uuid
}
