package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func (h *Handler) deleteBackup(w http.ResponseWriter, r *http.Request) {
	statement := "update user set backup_dir = ''"
	args := []interface{}{}

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	} else if !wl.IsAdminUser() {
		// only process current user
		statement += " where id = ?"
		args = append(args, wl.Userid)
	}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, statement, args...)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
}

func (h *Handler) startBackup(w http.ResponseWriter, r *http.Request) {
	h.backupCh <- struct{}{}
}

func (h *Handler) setBackup(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.RemoteAddr, ":")

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		if parts[0] != "127.0.0.1" {
			common.WriteError(w, wl.Err)
			return
		}
	}

	defer r.Body.Close()
	req := &types.BackupCreateRequest{}
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		common.WriteError(w, err)
		return
	}

	if req.DestDisk == "" {
		common.WriteError(w, errors.New("empty backup directory is provided"))
		return
	} else if req.DestDisk == filepath.Base(h.conf.MountDir) {
		common.WriteError(w, errors.New("user's backup directory can not be in the same mount directory"))
		return
	}
	backupDir := h.getUserDir(req.DestDisk, req.Username)
	if err := os.MkdirAll(filepath.Join(backupDir, common.AppPhoto), 0744); err != nil {
		common.WriteError(w, err)
		return
	}

	mountDirs, err := listMounts(h.conf.MountDir)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	md := h.getUserMountDir(backupDir, req.Username)
	m, ok := mountDirs[md]
	if !ok {
		logrus.Warnf("%s backup dir %s is not found in the mount: %+v", req.Username, md, mountDirs)
		if !h.conf.DisableMountMon && (runtime.GOARCH == "arm" || runtime.GOARCH == "arm64") {
			common.WriteError(w, common.ErrDeviceNotMount)
			return
		}
		m = &types.MountDir{Type: types.MountLocal, Dir: md}
	}
	_, err = h.migrateMntUUID(req.Username, m)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	h.userMountLock.Lock()
	mi, ok := h.userMountStatus[req.Username]
	if !ok {
		if !h.conf.DisableMountMon && (runtime.GOARCH == "arm" || runtime.GOARCH == "arm64") {
			h.userMountLock.Unlock()
			common.WriteError(w, errors.Errorf("user %s is not in the system yet %v", req.Username, h.userMountStatus))
			return
		}
	}
	mi.backupMnt = m
	h.userMountStatus[req.Username] = mi
	h.userMountLock.Unlock()

	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "update user set backup_dir = ? where user_name = ?", backupDir, req.Username)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
}

func (h *Handler) backupResult(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.RemoteAddr, ":")
	if parts[0] != "127.0.0.1" {
		common.WriteError(w, errors.New("backup api can only triggered from localhost"))
		return
	}
	typ := mux.Vars(r)["typ"]
	user := mux.Vars(r)["user"]
	code := mux.Vars(r)["code"]
	if typ == "" || user == "" || code == "" {
		common.WriteError(w, common.ErrBadRequest)
		return
	}
	if typ != "db" {
		common.WriteError(w, common.ErrBadRequest)
		return
	}
	bak, ok := h.lastBackup[user]
	if !ok {
		bak = types.BackupResult{}
	}
	now := time.Now()
	if typ == "db" {
		bak.LastDBBackup = &now
		bak.DBRetCode = &code
		if code == "0" {
			bak.LastDBSuccess = &now
		}
	}
	h.lastBackup[user] = bak
}

type backupDir struct {
	src string
	dst string
}

func (h *Handler) wrapErr(user, target string, retErr, newErr error) error {
	errMsg := fmt.Sprintf("while backup %s to %s, got %v", target, user, newErr)
	logrus.Warn(errMsg)
	h.backupLogger.Warn(errMsg)
	if retErr == nil {
		return errors.New(errMsg)
	}
	return errors.Wrap(retErr, errMsg)
}

func (h *Handler) backupDB(users []user.User) error {
	var retErr error
	dbf, err := os.Open(h.conf.DbFilename)
	if err != nil {
		return err
	}
	defer dbf.Close()
	for _, u := range users {
		for _, dir := range []string{u.HomeDir, u.BackupDir} {
			if dir == "" {
				continue
			}
			if _, err := dbf.Seek(0, io.SeekStart); err != nil {
				return err
			}
			dest, err := os.Create(filepath.Join(dir, common.LomodDbFilename))
			if err != nil {
				retErr = h.wrapErr(u.Name, "db", retErr, err)
				continue
			}
			if _, err := io.Copy(dest, dbf); err != nil {
				retErr = h.wrapErr(u.Name, "db", retErr, err)
			}
			if err := dest.Close(); err != nil {
				retErr = h.wrapErr(u.Name, "db", retErr, err)
			}
		}
	}
	return retErr
}

func (h *Handler) backup(users []user.User) error {
	var retErr error
	for _, u := range users {
		if u.BackupDir == "" {
			logrus.Infof("%s doesn't have backup dir", u.Name)
			h.backupLogger.Infof("%s doesn't have backup dir", u.Name)
			continue
		}
		h.userMountLock.Lock()
		mi, ok := h.userMountStatus[u.Name]
		h.userMountLock.Unlock()
		if ok {
			if mi.homeErr != nil {
				logrus.Warnf("%s has unhealthy home dir: %s", u.Name, mi.homeErr)
				h.setBackupResult(u.Name, errors.Wrapf(mi.homeErr, mi.homeMnt.Dir), time.Now())
				continue
			}
			if mi.backupErr != nil {
				logrus.Warnf("%s has unhealthy backup dir: %s", u.Name, mi.backupErr)
				h.setBackupResult(u.Name, errors.Wrapf(mi.backupErr, mi.backupMnt.Dir), time.Now())
				continue
			}
		}
		master, _ := common.GetUserPhotoDir(u.HomeDir)
		trashDir := filepath.Join(filepath.Join(u.HomeDir, common.AppPhoto), common.AppPhotoTrashDir)
		bakdir := filepath.Join(u.BackupDir, common.AppPhoto)

		dirs := []backupDir{
			{src: master, dst: bakdir},
			{src: trashDir, dst: bakdir},
		}

		begin := time.Now()
		out, err := h.backupDir(dirs)
		h.setBackupResult(u.Name, err, begin)
		if err != nil {
			retErr = h.wrapErr(u.Name, "asset", retErr, err)
			h.backupLogger.Warnf("while backup %s's %v got\n%s", u.Name, dirs, out)
		} else {
			h.backupLogger.Infof("success backup %s's %v.\n%s", u.Name, dirs, out)
		}
	}
	return retErr
}

func (h *Handler) setBackupResult(username string, err error, begin time.Time) {
	bak, ok := h.lastBackup[username]
	if !ok {
		bak = types.BackupResult{}
	}

	bak.LastAssetBackupBegin = begin
	if err != nil {
		bak.AssetRetCode = err.Error()
	} else {
		bak.AssetRetCode = ""
		bak.LastAssetSuccess = time.Now()
	}
	bak.LastAssetBackupEnd = time.Now()
	h.lastBackup[username] = bak
}

func (h *Handler) backupDir(dirs []backupDir) (string, error) {
	retStrings := []string{}
	var retErr error
	for _, dir := range dirs {
		exist, err := common.IsFileExist(dir.src)
		if err != nil {
			if retErr == nil {
				retErr = err
			} else {
				retErr = errors.Wrapf(retErr, "check file %s stat got %v", dir.src, err)
			}
			continue
		}
		if !exist {
			continue
		}
		if err := os.MkdirAll(dir.dst, h.conf.FolderPerm); err != nil {
			return "", err
		}
		content, err := rsync(h.conf.ExeDir, dir.src, dir.dst)
		retStrings = append(retStrings, string(content))
		if err == nil {
			continue
		}
		if retErr == nil {
			retErr = err
		} else {
			retErr = errors.Wrapf(retErr, "rsync %s --> %s got %v", dir.src, dir.dst, err)
		}
	}
	return strings.Join(retStrings, "\n"), retErr
}
