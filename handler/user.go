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
	agent "bitbucket.org/lomoware/lomo-backend/common/castagent"
	"bitbucket.org/lomoware/lomo-backend/common/conf"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	lnet "bitbucket.org/lomoware/lomo-backend/common/net"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

var assetTables = []string{
	"metadata", "metadata_geo", "metadata_scene", "metadata_face", "metadata_recface", "metadata_text", "metadata_human",
	"metadata_similarity", "metadata_tag", "metadata_encrypt", "asset_album_label", "asset_album_camera_make",
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	username, password, device, err := user.GetUserLogin(r)
	if err != nil {
		q := r.URL.Query()
		username = q.Get("username")
		password = q.Get("password")
		device = q.Get("device")
	}

	var (
		token  string
		userid int
	)
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		token, userid, err = user.Login(ctx, tx, username, password, device, h.tokenDuration)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	common.WriteBody(w, user.LoginResp{Token: token, Userid: userid})
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	retUsers := []*user.User{}
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		//return home dir if request is from localhost
		users, err := user.ListUsers(ctx, tx)
		if err != nil {
			return err
		}
		for _, u := range users.Users {
			if !u.IsBotUser() {
				retUsers = append(retUsers, u)
				continue
			}
			s, ok := h.userStatus[u.ID]
			if !ok {
				logrus.Warnf("Bot user %s no keep alive", u.Name)
				retUsers = append(retUsers, u)
				continue
			}
			// chromecast last seen should be not empty if it is available
			// not add offline chromecast
			if u.IsChromecast() {
				status, ok := s[chromcastLoginDeviceID]
				if !ok {
					logrus.Warnf("Chromecast user %s not have status", u.Name)
					continue
				}
				// not return chromecast user if it is not seen last time
				if status.LastSeen != "" {
					retUsers = append(retUsers, u)
				}
				continue
			}

			u.Keepalive = map[string]user.KeepaliveStatus{}
			for deviceID, status := range s {
				deviceName, err := user.GetLoginDeviceName(ctx, tx, deviceID)
				if err != nil {
					logrus.Warnf("Bot user %s login device ID %d haven't device name", u.Name, deviceID)
					continue
				}
				u.Keepalive[deviceName] = status
			}
			retUsers = append(retUsers, u)
		}
		return nil
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, &user.Users{Users: retUsers})
}

func (h *Handler) getUserMountDir(homedir, username string) string {
	return strings.TrimSuffix(strings.TrimSuffix(homedir, username), string(filepath.Separator))
}

// validStorageRoots returns every directory a new account's home (or
// backup) directory is allowed to live under: the single configured
// MountDir, plus -- on Windows, where MountDir is hard-wired to BaseDir
// (see initConfig) rather than an OS-level mount pool like /media -- a
// "\Lomorage" root on every other local/removable drive (extraStorageRoots),
// so an account can be pointed at e.g. an external hard drive.
func (h *Handler) validStorageRoots() []string {
	roots := []string{filepath.Clean(h.conf.MountDir)}
	for _, m := range extraStorageRoots(h.conf.MountDir) {
		roots = append(roots, filepath.Clean(m.Dir))
	}
	return roots
}

// getUserDir resolves a client-supplied storage-location string (from the
// createUser request body -- POST /user is unauthenticated, so this is
// untrusted input) into an absolute home directory for username. Every
// branch is funneled through the containment check below before returning,
// since dir can contain ".." segments (e.g. "Lomorage/../../../Windows/jeromy")
// that filepath.Join would otherwise happily resolve outside a valid root --
// letting an attacker point a new account's home directory, and everything
// it writes, anywhere the process can create files.
func (h *Handler) getUserDir(dir, username string) string {
	candidate := h.resolveUserDir(dir, username)
	for _, root := range h.validStorageRoots() {
		if candidate == root || strings.HasPrefix(candidate, root+string(filepath.Separator)) {
			return candidate
		}
	}
	// candidate escaped every valid root -- fall back to a safe,
	// deterministic location rather than letting the account's files land
	// somewhere unintended.
	return filepath.Join(h.conf.MountDir, username)
}

func (h *Handler) resolveUserDir(dir, username string) string {
	if filepath.Base(dir) == username {
		if filepath.IsAbs(dir) {
			return filepath.Clean(dir)
		}
		// dir is a display path from /mount, shaped like
		// "<mountBase>\<...>\username" (see listMountedDir's retDir) -- e.g.
		// picking an already-existing folder that happens to match the
		// username being (re)created, such as reclaiming a home dir that
		// survived a reset. Resolve it against the real absolute MountDir
		// instead of returning it as-is: an unresolved relative path here
		// would later be made "absolute" against the process's working
		// directory (wherever it happened to be launched from), not BaseDir,
		// silently misplacing every file this user ever writes.
		mountBase := filepath.Base(h.conf.MountDir)
		rel := strings.TrimPrefix(dir, mountBase)
		rel = strings.TrimPrefix(rel, string(filepath.Separator))
		return filepath.Join(h.conf.MountDir, rel)
	}
	if filepath.IsAbs(dir) {
		return filepath.Join(dir, username)
	} else if dir == filepath.Base(h.conf.MountDir) {
		return filepath.Join(h.conf.MountDir, username)
	}
	return filepath.Join(filepath.Join(h.conf.MountDir, filepath.Base(dir)), username)
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	u := &user.User{}
	if err := json.NewDecoder(r.Body).Decode(u); err != nil {
		common.WriteError(w, err)
		return
	}

	u.Name = strings.TrimSpace(u.Name)
	if err := u.Validate(); err != nil {
		common.WriteError(w, err)
		return
	}

	mountDirs, err := listMounts(h.conf.MountDir)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	mi := &userMountInfo{}
	u.HomeDir = h.getUserDir(u.HomeDir, u.Name)
	if !filepath.IsAbs(u.HomeDir) {
		// must be windows or mac environment
		mi.homeMnt = types.MountDir{Type: types.MountLocal, Dir: h.getUserMountDir(u.HomeDir, u.Name)}
	} else {
		md := h.getUserMountDir(u.HomeDir, u.Name)
		m, ok := mountDirs[md]
		if !ok {
			logrus.Warnf("%s home dir %s is not found in the mount: %+v", u.Name, u.HomeDir, md)
			if !h.conf.DisableMountMon && (runtime.GOARCH == "arm" || runtime.GOARCH == "arm64") {
				common.WriteError(w, common.ErrDeviceNotMount)
				return
			}
			m = &types.MountDir{Type: types.MountLocal, Dir: md}
		}
		mi.homeMnt = *m
	}

	// create user directory firstly
	var (
		cu    castUser
		admin int
		dirs  []string
	)
	if u.BotUser {
		admin = admin | user.BotFlag
		u.HomeDir = common.GetSambaUserHomeDir(h.conf.BaseDir, u.Name)
		dirs = []string{common.GetUserPhotoSharedDir(u.HomeDir)}

		var err error
		cu.ctx = context.Background()
		cu.name = u.Name
		cu.notify = make(chan agent.Notify)
		cu.cast, err = h.createCastUser(cu.ctx, u, cu.notify)
		if err != nil && err != errNoChromecastIP {
			common.WriteError(w, err)
			return
		}
		if cu.cast != nil {
			if err := cu.cast.Connect(); err != nil {
				common.WriteError(w, err)
				return
			}
			if err := cu.cast.Close(); err != nil {
				common.WriteError(w, err)
				return
			}
			admin = admin | user.CastFlag
		}
	} else {
		admin = 1
	}
	u.SetAdminFlag(admin)

	master, preview := common.GetUserPhotoDir(u.HomeDir)
	dirs = append(dirs, master, preview)

	// create user backup directory if it is not empty
	if u.BackupDir != "" {
		m, ok := mountDirs[u.BackupDir]
		if !ok {
			logrus.Warnf("%s backup dir %s is not found in the mount: %+v", u.Name, u.BackupDir, mountDirs)
			if !h.conf.DisableMountMon && (runtime.GOARCH == "arm" || runtime.GOARCH == "arm64") {
				common.WriteError(w, common.ErrDeviceNotMount)
				return
			}
			m = &types.MountDir{Type: types.MountLocal, Dir: u.BackupDir}
		}
		mi.backupMnt = m
		u.BackupDir = h.getUserDir(u.BackupDir, u.Name)
		dirs = append(dirs, filepath.Join(u.BackupDir, common.AppPhoto))
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, h.conf.FolderPerm); err != nil {
			common.WriteError(w, err)
			return
		}
	}

	if !u.BotUser {
		_, err = h.migrateMntUUID(u.Name, &mi.homeMnt)
		if err != nil {
			common.WriteError(w, err)
			return
		}
		err = common.CopyDocs(common.GetDocDir(h.conf.BaseDir), u.HomeDir, h.conf.FolderPerm)
		if err != nil {
			common.WriteError(w, err)
			return
		}
	}
	if u.BackupDir != "" {
		_, err := h.migrateMntUUID(u.Name, mi.backupMnt)
		if err != nil {
			logrus.Warnf("%s migrate backup dir %+v: %s", u.Name, mi.backupMnt, err)
			common.WriteError(w, err)
			return
		}
	}

	if !u.BotUser {
		h.userMountLock.Lock()
		h.userMountStatus[u.Name] = mi
		h.userMountLock.Unlock()
	}

	err = dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		// no validation if zero users
		count, err := user.Counts(ctx, tx, false)
		if err != nil {
			return err
		}
		wl := w.(*logger.ResponseLogger)
		if count != 0 && wl.Err != nil {
			if !lnet.IsSameSubnet(net.ParseIP(common.GetRequestAddress(r)), h.listenIPs) {
				return wl.Err
			}
			u.SetAdminFlag(0)
		}

		if err := user.CreateUser(ctx, tx, u, h.conf.SambaConf); err != nil {
			return err
		}
		if cu.cast == nil {
			return nil
		}
		cu.token, _, err = user.Login(ctx, tx, u.Name, u.Password, chromcastLoginDevice, h.tokenDuration)
		return err
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}

	// start replicate database to the user's media directory
	if h.conf.ReplicaDuration != time.Duration(0) {
		go func() {
			h.replicaCh <- replicateUser{add: true, users: map[string]string{u.Name: u.HomeDir}}
		}()
	}

	if cu.cast != nil {
		cu.newItem = make(chan agent.CastItem)
		h.castLock.Lock()
		h.castUsers[u.ID] = &cu
		h.castLock.Unlock()
		// TODO: disable auto start chromecast because not sure if user like to auto play
		//go h.slideshow(&cu, nil)
	}
}

func (h *Handler) getMe(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	var u *user.User
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		//return home dir if request is from localhost
		var err error
		u, err = user.Get(ctx, tx, wl.Userid)
		return err
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, u)
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	u := &user.User{}
	if err := json.NewDecoder(r.Body).Decode(u); err != nil {
		common.WriteError(w, err)
		return
	}

	if u.Name == "" {
		common.WriteError(w, common.ErrEmptyUsername)
		return
	}
	if u.Email == "" && u.Phone == "" && u.Password == "" && u.NickName == "" {
		common.WriteError(w, common.ErrBadRequest)
		return
	}

	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		// no validation if zero users
		wl := w.(*logger.ResponseLogger)
		if wl.Err != nil {
			return wl.Err
		}

		if err := user.UpdateUser(ctx, tx, u); err != nil {
			return err
		}
		if u.Password != "" {
			// Invalidate the target account's sessions (forcing it to log
			// back in with the new password) -- not the caller's. The
			// caller may be a different, still-logged-in account changing
			// someone else's password (see the Users page), so wl.Userid
			// (the caller) is the wrong id here whenever that isn't u.Name.
			target, err := user.GetByName(ctx, tx, u.Name)
			if err != nil {
				return err
			}
			return user.DeleteTokenByUserID(ctx, tx, target.ID)
		}
		return nil
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	username := mux.Vars(r)["userName"]
	if username == "" {
		common.WriteError(w, common.ErrNotExistUser)
		return
	}
	isBotUser := true
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		u, err := user.GetByName(ctx, tx, username)
		if err != nil {
			return err
		}
		if !u.IsBotUser() {
			isBotUser = false
			for _, tbl := range []string{"asset_album", "asset_album_geo"} {
				_, err = tx.ExecContext(ctx, "delete from "+tbl+
					" where album_id in (select id from album where user_id = ?)", u.ID)
				if err != nil {
					return err
				}
			}
			for _, tbl := range assetTables {
				_, err = tx.ExecContext(ctx, "delete from "+tbl+
					" where asset_id in (select id from asset where user_id = ?)", u.ID)
				if err != nil {
					return err
				}
			}
			for _, tbl := range []string{"asset", "member", "device", "album"} {
				_, err = tx.ExecContext(ctx, "delete from "+tbl+" where user_id = ?", u.ID)
				if err != nil {
					return err
				}
			}
		}
		if _, err := tx.ExecContext(ctx, "delete from groups where owner_id = ?", u.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "delete from share where sender_id = ? or receiver_id = ?",
			u.ID, u.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "delete from assets_hide where receiver_id = ?", u.ID); err != nil {
			return err
		}
		return user.DeleteUser(ctx, tx, username)
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	// start remove replicate database to the user's media directory
	if h.conf.ReplicaDuration != time.Duration(0) {
		go func() {
			h.replicaCh <- replicateUser{add: true, users: map[string]string{username: ""}}
		}()
	}

	if !isBotUser {
		h.userMountLock.Lock()
		delete(h.userMountStatus, username)
		h.userMountLock.Unlock()
	}
}

func (h *Handler) getSpace(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	m := types.MountDir{}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		homedir, err := user.GetHomedir(ctx, tx, wl.Userid)
		if err != nil {
			return err
		}
		stat, err := getStatfs(homedir)
		if err != nil {
			return err
		}

		m.Dir = homedir
		m.FreeSize = uint64(stat.Bsize) * stat.Bfree / uint64(mb)
		m.TotalSize = uint64(stat.Bsize) * stat.Blocks / uint64(mb)
		return nil
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, m)
}

func (h *Handler) validateToken(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
	}
}

func (h *Handler) resetBotUserState() error {
	return dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		users, err := user.ListUsers(ctx, tx)
		if err != nil {
			return err
		}
		for _, u := range users.Users {
			if !u.BotUser || u.Status == common.UserStatusOffline {
				continue
			}
			if err := user.UpdateUserStatus(ctx, tx, u.ID, common.UserStatusOffline); err != nil {
				logrus.Warnf("reset bot user %s offline status got: %v", u.Name, err)
			}
		}
		return nil
	})
}

func (h *Handler) userKeepalive(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	userStatus, ok := h.userStatus[wl.Userid]
	if !ok {
		userStatus = map[int]user.KeepaliveStatus{}
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// skip error
		logrus.Warnf("split client address %s: %v", r.RemoteAddr, err)
	}
	s := user.KeepaliveStatus{IP: ip, LastSeen: time.Now().Format(common.TimeFormatLomod)}
	if len(r.URL.Query()["port"]) > 0 {
		s.Port, err = strconv.Atoi(r.URL.Query()["port"][0])
		if err != nil {
			// skip error
			logrus.Warnf("convert client port %s: %v", r.URL.Query()["port"], err)
		}
	}
	userStatus[wl.Deviceid] = s
	h.userStatus[wl.Userid] = userStatus

	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		return user.UpdateUserStatus(ctx, tx, wl.Userid, common.UserStatusOnline)
	}); err != nil {
		logrus.Warnf("%s - %s update online status: %v", wl.Username, wl.Devicename, err)
		common.WriteError(w, err)
		return
	}

	defer func() {
		if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
			return user.UpdateUserStatus(ctx, tx, wl.Userid, common.UserStatusOffline)
		}); err != nil {
			logrus.Warnf("%s - %s update offline status: %v", wl.Username, wl.Devicename, err)
		}
	}()

	// get ping interval
	var interval time.Duration
	if mux.Vars(r)["interval"] == "" {
		interval = time.Minute
	} else {
		interval, err = time.ParseDuration(mux.Vars(r)["interval"])
		if err != nil {
			logrus.Warnf("wrong ping interval: %s", mux.Vars(r)["interval"])
			interval = time.Minute
		}
	}

	// upgrade to websocket
	header := http.Header{}
	header.Add("Access-Control-Allow-Origin", "*")

	upgrader := websocket.Upgrader{
		ReadBufferSize:  64,
		WriteBufferSize: 64,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(w, r, header)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	defer conn.Close()

	// Time allowed to write the file to the client.
	closeChan := make(chan struct{})
	pingChan := make(chan struct{})
	conn.SetCloseHandler(func(code int, text string) error {
		logrus.Warnf("%s - %s closed keep-alive: (%d) - %s", wl.Username, wl.Devicename, code, text)
		closeChan <- struct{}{}
		return nil
	})
	conn.SetPingHandler(func(text string) error {
		pingChan <- struct{}{}
		return conn.WriteMessage(websocket.PongMessage, []byte(text))
	})

	go func(c *websocket.Conn) {
		for {
			if _, _, err := c.NextReader(); err != nil {
				logrus.Warnf("next reader: %v", err)
				closeChan <- struct{}{}
				return
			}
		}
	}(conn)

	for {
		after := time.After(2 * interval)
		select {
		case <-after:
			logrus.Infof("no heartbeat within %s", 2*interval)
			return
		case <-closeChan:
			return
		case <-h.gCtx.Done():
			return
		case <-pingChan:
		}
	}
}

func makeUserDeviceConfKey(uid, did int) string {
	return fmt.Sprintf("%s-%d-%d", common.ConfUserPrefix, uid, did)
}

func (h *Handler) putUserConf(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	defer r.Body.Close()
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		return conf.SetConfValue(ctx, tx, makeUserDeviceConfKey(wl.Userid, wl.Deviceid), string(data))
	}); err != nil {
		common.WriteError(w, err)
	}
}

func (h *Handler) getUserConf(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	value := ""
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		value, err = conf.GetConfValue(ctx, tx, makeUserDeviceConfKey(wl.Userid, wl.Deviceid))
		return err
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}
	w.Write([]byte(value))
}
