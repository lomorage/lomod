package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/check"
	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"bitbucket.org/lomoware/lomo-backend/common/conf"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	lnet "bitbucket.org/lomoware/lomo-backend/common/net"
	"bitbucket.org/lomoware/lomo-backend/common/release"
	"bitbucket.org/lomoware/lomo-backend/common/timezone"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/gorilla/mux"
	"github.com/leslie-wang/govips/pkg/vips"
	"github.com/mackerelio/go-osstat/uptime"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	mb  = 1024 * 1024
	day = 24 * time.Hour
)

type statfs struct {
	Bsize  uint32
	Blocks uint64
	Bfree  uint64
	Bavail uint64
}

type setTimeZoneRequest struct {
	Timezone string
}

func (h *Handler) setEthNW(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)

	go func() {
		err := h.setwifi(wl, "eth", "", "")
		if err != nil {
			logrus.Warn(err)
		}
	}()
}

func (h *Handler) setWifiAP(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	go func() {
		err := h.setwifi(wl, "ap", "", "")
		if err != nil {
			logrus.Warn(err)
		}
	}()
}

func (h *Handler) setWifiClient(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)

	go func() {
		err := h.setwifi(wl, "client", mux.Vars(r)["ssid"], mux.Vars(r)["password"])
		if err != nil {
			logrus.Warn(err)
		}
	}()
}

func (h *Handler) setwifi(wl *logger.ResponseLogger, typ, ssid, wifiPassword string) error {
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		count, err := user.Counts(ctx, tx, false)
		if err != nil {
			return err
		}
		if count != 0 {
			return wl.Err
		}
		return nil
	}); err != nil {
		return err
	}
	if typ == "ap" {
		out, err := cmd.Run("/sbin/wifi_switch.sh", "ap")
		logrus.Info(out)
		return err
	} else if typ == "eth" {
		out, err := cmd.Run("/sbin/wifi_switch.sh", "eth")
		logrus.Info(out)
		return err
	}
	out, err := cmd.Run("/sbin/wifi_switch.sh", "client", ssid, wifiPassword)
	logrus.Info(out)
	return err
}

func (h *Handler) factoryReset(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		for _, tbl := range []string{"user", "groups", "member", "asset", "share", "token", "device",
			"assets_hide", "album", "asset_album", "sqlite_sequence",
			"metadata", "metadata_scene", "metadata_face", "metadata_recface", "metadata_text", "metadata_human",
			"metadata_similarity", "metadata_tag", "metadata_encrypt",
		} {
			if _, err := tx.ExecContext(ctx, "delete from "+tbl); err != nil {
				return err
			}
		}
		for _, dir := range []string{h.conf.LogDir} {
			fis, err := ioutil.ReadDir(dir)
			if err != nil {
				logrus.Warnf("while factory reset, remove files in %s: %v", dir, err)
				continue
			}
			for _, fi := range fis {
				f := filepath.Join(dir, fi.Name())
				if err := os.RemoveAll(f); err != nil {
					logrus.Warnf("while factory reset, remove %s: %v", f, err)
				}
			}
		}
		return nil
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}
	// quit after factory reset
	go func() {
		// set maintenance to deny new request
		h.mu.Lock()
		h.lastMaintStartTime = time.Now()
		h.mu.Unlock()

		logrus.Info("Lomod exit due to factory reset")
		h.Close()
		time.Sleep(5 * time.Second)
		os.Exit(0)

	}()
}

func (h *Handler) poweroff(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	if h.previewRunner.PendingJobCount() != 0 {
		common.WriteError(w, common.ErrMaintenance)
		return
	}

	reboot := r.URL.Query().Get(common.QueryKeyReboot) != ""

	// set maintenance to deny new request
	h.mu.Lock()
	h.lastMaintStartTime = time.Now()
	h.mu.Unlock()

	if reboot {
		logrus.Info("User signal reboot")
	} else {
		logrus.Info("User signal poweroff")
	}

	if err := cmd.Poweroff(reboot); err != nil {
		common.WriteError(w, err)
	}
	go func() {
		time.Sleep(time.Second)
		h.Close()
	}()
}

func (h *Handler) enableCloudIPHelper(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	if err := h.setCloudIPHelper(true); err != nil {
		common.WriteError(w, err)
	}
}

func (h *Handler) disableCloudIPHelper(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	if err := h.setCloudIPHelper(false); err != nil {
		common.WriteError(w, err)
	}
}

func (h *Handler) setCloudIPHelper(enable bool) error {
	val := "1"
	if !enable {
		val = "0"
	}

	return dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		if err := conf.SetConfValue(ctx, tx, common.ConfCloudIPHelper, val); err != nil {
			return err
		}
		h.useCloudIPHelper = enable
		return nil
	})
}

func (h *Handler) getSystemConf(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	confs := map[string]string{}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		val, err := conf.GetConfValue(ctx, tx, common.ConfCloudIPHelper)
		if err != nil {
			if !common.IsErrNoRows(err) {
				return err
			}
			val = "0"
		}
		confs[common.ConfCloudIPHelper] = val

		val, err = conf.GetConfValue(ctx, tx, common.ConfWebdavDirLayout)
		if err != nil {
			if !common.IsErrNoRows(err) {
				return err
			}
			val = "0"
		}
		confs[common.ConfWebdavDirLayout] = val
		return nil
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, confs)
}

func (h *Handler) setSystemConf(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	key := mux.Vars(r)["key"]
	value := mux.Vars(r)["value"]
	if key != common.ConfWebdavDirLayout {
		common.WriteError(w, common.ErrNotImplementedFormat)
		return
	}
	l, err := strconv.Atoi(value)
	if err != nil {
		common.WriteError(w, errors.Wrapf(err, "invalid webdav layout: %s", value))
		return
	} else if l > dirLayoutYyyy || l < 0 {
		common.WriteError(w, errors.Errorf("invalid webdav layout: %s", value))
		return
	}
	if l == h.webdavDirLayout {
		return
	}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		return conf.SetConfValue(ctx, tx, common.ConfWebdavDirLayout, value)
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	h.webdavDirLayout = l
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	status, err := h.getSystemStatus()
	if err != nil {
		common.WriteError(w, err)
		return
	}
	w.Write([]byte(fmt.Sprintf("%d", status)))
}

func (h *Handler) getSystemStatus() (common.SystemStatus, error) {
	status := common.SystemStatusAbnormal
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		count, err := user.Counts(ctx, tx, false)
		if err != nil {
			return err
		}
		if count == 0 {
			status = common.SystemStatusNew
		} else {
			status = common.SystemStatusInited
		}
		return nil
	})
	return status, err
}

func (h *Handler) getFreeDiskSize(dir string) (uint64, error) {
	stat, err := getStatfs(dir)
	if err != nil {
		return 0, err
	}

	return uint64(stat.Bsize) * stat.Bfree / mb, nil
}

func (h *Handler) reportVIPS(w http.ResponseWriter, r *http.Request) {
	vips.PrintObjectReport("report vips object")
	vips.VipsPrintCache()
}

func (h *Handler) getSystemInfo(getDetail bool) types.SystemInfo {
	var err error
	info := types.SystemInfo{OS: runtime.GOOS, Arch: runtime.GOARCH, APIVersion: "1.1", WebpPreview: !h.conf.UseJpg,
		LomodVersion: release.Version, UUID: h.uuid, UserStatus: map[string]types.UserStatus{},
		ListenIPs: []net.IP{}, PublicAddr: []string{}, UserDisks: []types.UserDisk{}}

	// TODO: need determine tunnel usage
	//if h.tunnel != nil {
	//	info.TunnelURL = h.tunnel.URL()
	//}
	/* TODO: the website is slow sometimes
	_, err := lnet.GetPublicIP()
	if err != nil {
		info.NetworkStatus = err.Error()
	} else {
	}
	*/
	info.SystemStatus, err = h.getSystemStatus()
	if err != nil {
		logrus.Warnf("while getting system status, got error: %v", err)
	}

	if !getDetail {
		return info
	}

	info.LastBackup = h.lastBackup

	info.OSStatus, err = h.getOSStatus()
	if err != nil {
		logrus.Warnf("while getting OS status, got error: %v", err)
	}

	uids := map[string]int{}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		users, err := user.ListUsers(ctx, tx)
		if err != nil {
			return err
		}
		for _, u := range users.Users {
			if u.IsBotUser() {
				continue
			}
			uids[u.Name] = u.ID
			ud := types.UserDisk{Username: u.Name}
			ud.FreeSize, err = h.getFreeDiskSize(u.HomeDir)
			if err != nil {
				logrus.Warnf("while getting user %s's disk free size, got error: %v", u.Name, err)
				ud.Error = err.Error()
			}
			info.UserDisks = append(info.UserDisks, ud)
		}
		return err
	}); err != nil {
		logrus.Warnf("while getting user disk free size, got error: %v", err)
	}

	// always try to probe and check current mount status because of race condition.
	// it is possible that monitor mount is not quick enough to recover yet
	probeMountDirs, err := listMounts(h.conf.MountDir)
	if err != nil {
		logrus.Warnf("while listing mount in system API, got error: %v", err)
	} else if !h.conf.DisableMountMon && (runtime.GOARCH == "arm" || runtime.GOARCH == "arm64") {
		h.checkUserMounts(probeMountDirs)
	}

	for username, mi := range h.userMountStatus {
		us := types.UserStatus{HomeDisk: types.Disk{}}
		uid, ok := uids[username]
		if !ok {
			logrus.Warnf("unable to find uid for username: %s", username)
		} else {
			as, ok := h.userAssetSummary[uid]
			if ok {
				us.AssetSummary = as
			}
		}
		if mi.homeErr != nil {
			us.HomeDisk.Status = mi.homeErr.Error()
			us.HomeDisk.ErrorCode = common.GetErrID(mi.homeErr)
		} else {
			us.HomeDisk.FreeSizeInMB, err = h.getFreeDiskSize(mi.homeMnt.Dir)
			if err != nil {
				logrus.Warnf("while getting %s free size status: %v", mi.homeMnt.Dir, err)
				us.HomeDisk.Status = err.Error()
			}
		}
		if mi.backupMnt != nil {
			us.BackupDisk = &types.Disk{}
			if mi.backupErr != nil {
				if mi.backupErr != nil {
					us.BackupDisk.Status = mi.backupErr.Error()
					us.BackupDisk.ErrorCode = common.GetErrID(mi.backupErr)
				}
			} else {
				us.BackupDisk.FreeSizeInMB, err = h.getFreeDiskSize(mi.backupMnt.Dir)
				if err != nil {
					logrus.Warnf("while getting %s free backup disk size status: %v", username, err)
					us.BackupDisk.Status = err.Error()
				}
			}
		}
		info.UserStatus[username] = us
	}

	if h.lomocloudConf.SubDomain != "" {
		u := ""
		if h.lomocloudConf.publicPort == 443 {
			u = fmt.Sprintf("https://%s", h.lomocloudConf.SubDomain)
		} else {
			u = fmt.Sprintf("http://%s:%d", h.lomocloudConf.SubDomain, h.lomocloudConf.publicPort)
		}
		info.PublicAddr = []string{u}
	}

	// deprecated part
	info.DiskStatus = info.OSStatus.Disk.Status
	info.OSDiskFreeSize = info.OSStatus.Disk.FreeSizeInMB
	info.NetworkStatus = info.OSStatus.Network.Status
	info.PublicAddr = info.OSStatus.Network.PublicAddrs
	info.ListenIPs = info.OSStatus.Network.ListenIPs
	info.TimezoneName = info.OSStatus.TimeZone.Name
	info.TimezoneOffset = info.OSStatus.TimeZone.Offset

	return info
}

func (h *Handler) system(w http.ResponseWriter, r *http.Request) {
	info := h.getSystemInfo(w.(*logger.ResponseLogger).Err == nil ||
		lnet.IsSameSubnet(net.ParseIP(common.GetRequestAddress(r)), h.listenIPs))
	common.WriteBody(w, info)
}

func (h *Handler) setTimeZone(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	defer r.Body.Close()
	req := &setTimeZoneRequest{}
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		common.WriteError(w, err)
		return
	}

	tzs, err := timezone.List()
	if err != nil {
		common.WriteError(w, err)
		return
	}
	found := false
	for _, tz := range tzs {
		if tz == req.Timezone {
			found = true
			break
		}
	}
	if !found {
		common.WriteError(w, errors.Wrapf(common.ErrNotImplementedFormat, "timezone: %s", req.Timezone))
		return
	}

	loc, err := time.LoadLocation(req.Timezone)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	t := time.Now()
	tl := t.In(loc)
	if t.Hour() == tl.Hour() {
		logrus.Infof("current timezone is same as required timezone: %s", req.Timezone)
		return
	}

	if err := setTimeZone(req.Timezone); err != nil {
		common.WriteError(w, err)
		return
	}
}

func (h *Handler) getTimeZone(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	tzs, err := timezone.List()
	if err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, tzs)
}

type jobTicker struct {
	timer    *time.Timer
	nextTick time.Time
}

func (t *jobTicker) updateTimer(startHour, startMinute, startSecond int, interval time.Duration) {
	t.nextTick = time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(),
		startHour, startMinute, startSecond, 0, time.Local)
	for !t.nextTick.After(time.Now()) {
		t.nextTick = t.nextTick.Add(interval)
	}
	diff := time.Until(t.nextTick)
	if t.timer == nil {
		t.timer = time.NewTimer(diff)
	} else {
		t.timer.Reset(diff)
	}
}
func (h *Handler) maintenance(startHour, startMinute, startSecond int, interval time.Duration,
	done chan struct{}, skipInitialScan bool) {
	// maintenance
	jobTicker := &jobTicker{}

	for {
		backupOnly := false
		ccheckOnly := false

	loop:
		if skipInitialScan {
			skipInitialScan = false
		} else {
			h.maintenanceScan(backupOnly, ccheckOnly)
		}

		jobTicker.updateTimer(startHour, startMinute, startSecond, interval)
		logrus.Infof("next backup is scheduled at %v", jobTicker.nextTick)
		select {
		case <-jobTicker.timer.C:
		case <-h.backupCh:
			backupOnly = true
			goto loop
		case <-h.ccheckCh:
			ccheckOnly = true
			goto loop
		case <-done:
			return
		}
	}
}

func (h *Handler) maintenanceScan(backupOnly, ccheckOnly bool) {
	var allUsers *user.Users
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		allUsers, err = user.ListUsers(ctx, tx)
		return err
	}); err != nil {
		logrus.Warnf("while maintenance, got %v", err)
		return
	}

	users := []user.User{}
	for _, u := range allUsers.Users {
		if !u.IsBotUser() {
			users = append(users, *u)
		}
	}

	// wait until no preview jobs
	noPreviewJobs := false
	for i := 0; i < 12; i++ {
		count := h.previewRunner.PendingJobCount()
		if count == 0 {
			noPreviewJobs = true
			break
		}
		logrus.Infof("%d inqueue preview jobs", count)
		time.Sleep(h.previewPendingJobCheckTimeout)
	}
	if !noPreviewJobs {
		logrus.Info("seems some inqueue preview jobs, retry tomorrow")
		return
	}

	h.mu.Lock()
	h.lastMaintStartTime = time.Now()
	h.mu.Unlock()

	logrus.Info("start backup")
	if !ccheckOnly {
		if err := h.backupDB(users); err != nil {
			logrus.Warnf("while backup db, got %v", err)
		} else {
			logrus.Info("complete backup db")
		}

		if err := h.backup(users); err != nil {
			logrus.Warnf("while backup asset, got %v", err)
		} else {
			logrus.Info("complete backup asset")
		}
	}

	// consistency check
	if h.conf.CheckLeftDays == 0 || ccheckOnly {
		h.conf.CheckLeftDays = h.conf.CheckInterval
		logrus.Info("start consistency check")
		if err := h.check(users); err != nil {
			logrus.Warnf("while consistency check, got %v", err)
		} else {
			logrus.Info("complete consistency")
			h.generateCCheckMissPreview(users)
		}
	} else {
		h.conf.CheckLeftDays--

		// always scan and fix missing files
		if !ccheckOnly && !backupOnly {
			logrus.Info("start generate missed preview")
			h.fixMissMasterPreviewAssets(users)
		}
	}

	h.mu.Lock()
	h.lastMaintEndTime = time.Now()
	h.mu.Unlock()
}

func (h *Handler) fixMissMasterPreviewAssets(users []user.User) {
	for _, user := range users {
		assets, err := h.memdb.GetUserAssets(user.ID)
		if err != nil && err != common.ErrEmptyAsset {
			logrus.Warnf("get all assets for user %s: %s", user.Name, err)
			continue
		}
		masterDir, _ := common.GetUserPhotoDir(user.HomeDir)
		for y, assetsY := range assets {
			for m, assetsM := range assetsY {
				for d, assetsD := range assetsM {
					if len(assetsD) == 0 {
						continue
					}
					// check dir is exist or not
					// XXX: do we need clean DB if folder is not exist, or introduce some algorithms to make the decision
					dir := common.NormalDatedDirName(masterDir, y, m+1, d+1)
					_, err = os.Stat(dir)
					if err != nil {
						logrus.Warnf("Error to get user %s's dir %s: %s", user.NickName, dir, err)
						continue
					}
					for _, assetData := range assetsD {
						master, preview, err := common.GetUserPhotoMasterPreviewDir(user.HomeDir, y, m+1, d+1, h.conf.FolderPerm)
						if err != nil {
							logrus.Warnf("Error to get master/preview dir for user %s's %d-%d-%d: %s", user.NickName, y, m+1, d+1, err)
							continue
						}
						var assetFilename, previewPrefix string
						extension := filepath.Ext(assetData.Name)
						assetID := strings.TrimSuffix(assetData.Name, extension)
						if types.IsAssetStatus(assetData.Status, types.AssetStatusScanLink) {
							err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
								return tx.QueryRowContext(ctx,
									"select a.title, aa.orig_filename from asset_album as aa inner join album as a on aa.album_id = a.id where aa.asset_id=? and a.author=? and a.user_id=?",
									assetID, common.AlbumAuthorInternal, user.ID).Scan(&master, &assetFilename)
							})
							if err != nil {
								logrus.Warnf("Error to get original asset dir for user %s's %d-%d-%d: %s", user.NickName, y, m+1, d+1, err)
								continue
							}
							previewPrefix = ext.NormalizeAssetNameString(y, m+1, d+1, assetID)
						} else {
							assetFilename = ext.NormalizeAssetNameString(y, m+1, d+1, assetData.Name)
						}
						masterFile := filepath.Join(master, assetFilename)
						if ext.IsSkipPreview(extension) {
							continue
						}
						_, err = os.Stat(masterFile)
						if err != nil {
							if os.IsNotExist(err) {
								if h.conf.DbClean {
									logrus.Warnf("User %s's %s is not found in file system, remove from DB",
										user.NickName, masterFile)
									err = h.deleteAsset1(user.ID, assetData.Name, types.Index, false, true)
									if err != nil {
										logrus.Warnf("Error to delete user %s's non exist asset: %s", user.NickName, masterFile)
									}
								}
							} else {
								logrus.Warnf("Error to probe user %s's asset: %s", user.NickName, masterFile)
							}
							continue
						}

						if extension == "."+ext.ZIPString {
							masterFile = strings.TrimSuffix(masterFile, extension)
							if ok, err := common.IsFileExist(ext.MkLivePhotoImageNameJPG(masterFile)); err != nil || !ok {
								if ok, err := common.IsFileExist(ext.MkLivePhotoImageNameHEIC(masterFile)); err != nil || !ok {
									logrus.Warnf("No live photo image file for user %s's %s: %s", user.NickName, assetData.Name, err)
									continue
								}
								masterFile = ext.MkLivePhotoImageNameHEIC(masterFile)
							} else {
								masterFile = ext.MkLivePhotoImageNameJPG(masterFile)
							}
						}

						if !h.conf.UseJpg {
							h.previewRunner.Generate(masterFile, preview, previewPrefix, extension, false, true)
						} else {
							h.previewRunner.Generate(masterFile, preview, previewPrefix, extension, false, false)
						}
					}
				}
			}
		}
	}
}

func (h *Handler) generateCCheckMissPreview(us []user.User) {
	if h.ccheckRunner.BadAssets == nil {
		return
	}
	for uid, as := range h.ccheckRunner.BadAssets {
		nickName := ""
		var usr user.User
		for _, u := range us {
			if u.ID != uid {
				continue
			}
			nickName = u.NickName
			usr = u
			break
		}
		if len(as[check.PreviewDirMiss]) > 0 {
			// whole preview directory is missing
			h.fixMissMasterPreviewAssets([]user.User{usr})
			continue
		}

		if len(as) < check.PreviewMiss {
			logrus.Warnf("%s's consistent check result doesn't have preview", nickName)
			continue
		}
		for _, a := range as[check.PreviewMiss] {
			if !h.conf.UseJpg {
				h.previewRunner.Generate(a.Asset1Path, a.Asset2Path, "", filepath.Ext(a.Asset1Path), false, true)
			} else {
				h.previewRunner.Generate(a.Asset1Path, a.Asset2Path, "", filepath.Ext(a.Asset1Path), false, false)
			}
		}
	}
}

func (h *Handler) getOSStatus() (types.OSStatus, error) {
	stats := types.OSStatus{Network: types.Network{ListenIPs: []net.IP{}, PublicAddrs: []string{}}}

	ut, err := uptime.Get()
	if err != nil {
		logrus.Warnf("Error to get uptime: %s", err)
	} else {
		if ut < day {
			stats.Uptime = ut.String()
		} else {
			days := ut / day
			ut -= days * day
			stats.Uptime = fmt.Sprintf("%dd%s", days, ut)
		}
	}

	stats.CPU, err = getCPU()
	if err != nil {
		return stats, err
	}

	stats.Memory, err = getMemory()
	if err != nil {
		return stats, err
	}

	stats.Disk.FreeSizeInMB, err = h.getFreeDiskSize(filepath.Dir(h.conf.DbFilename))
	if err != nil {
		logrus.Warnf("while getting base disk free size, got error: %v", err)
	}

	stats.Network.Status = "ok"

	if len(h.listenIPs) == 0 {
	} else {
		h.listenIPs, err = lnet.ListIPs()
		if err != nil {
			logrus.Warnf("while listing ips: %v", err)
		}
	}
	stats.Network.ListenIPs = h.listenIPs

	stats.TimeZone.Name, stats.TimeZone.Offset = time.Now().Zone()
	stats.TimeZone.Offset = stats.TimeZone.Offset / 3600

	return stats, nil
}
