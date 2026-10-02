package handler

import (
	"context"
	"database/sql"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	agent "bitbucket.org/lomoware/lomo-backend/common/castagent"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/net"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/gorilla/mux"
	"github.com/leslie-wang/zeroconf"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	chromcastNamePrefix    = "chromecast-"
	chromcastLoginDevice   = "lomod"
	chromcastLoginDeviceID = -1
)

var (
	errNoChromecastIP        = errors.New("chrome cast IP not exist")
	errNoChromecastInvalidIP = errors.New("chrome cast IP is not string")
	errNoChromecastPort      = errors.New("chrome cast port not exist")
)

type castUser struct {
	name    string
	token   string
	cast    *agent.Cast
	ctx     context.Context
	notify  chan agent.Notify
	newItem chan agent.CastItem
}

func (h *Handler) createCastUser(ctx context.Context, u *user.User, ch chan agent.Notify) (*agent.Cast, error) {
	meta, ok := u.Metadatas[types.DeviceIP]
	if !ok {
		return nil, errNoChromecastIP
	}
	ip, ok := meta.(string)
	if !ok {
		return nil, errNoChromecastInvalidIP
	}
	meta, ok = u.Metadatas[types.DevicePort]
	if !ok {
		return nil, errNoChromecastPort
	}
	portStr, ok := meta.(string)
	if !ok {
		return nil, errors.Errorf("chrome cast port %v is not int, %v", portStr, reflect.TypeOf(portStr))
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}
	if ip == "" && port == 0 {
		return nil, nil
	} else if ip == "" || port == 0 {
		return nil, errors.Errorf("chromecast should have both IP (%s) and port (%d)", ip, port)
	}
	return agent.New(ctx, ip, port, ch), nil
}

func (h *Handler) castToUser(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	receiverID, err := strconv.Atoi(mux.Vars(r)["userID"])
	if err != nil {
		common.WriteError(w, err)
		return
	}

	var (
		cu     *castUser
		record *types.Record
	)
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		receiver, err := user.Get(ctx, tx, receiverID)
		if err != nil {
			return err
		}
		record, cu, err = h.shareToUser(ctx, tx, wl.Userid,
			receiver, mux.Vars(r)["assetID"], r.URL.Query().Get("byhash") != "",
			types.CastToUser, h.parseDuration(""), nil)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	// try to close old one firstly
	cu.cast.Close()
	if err := cu.cast.Connect(); err != nil {
		common.WriteError(w, err)
		return
	}
	item := h.mkCastItem(cu.token, shareURI, record)
	if err := cu.cast.Cast(item); err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, record)
}

/*
func (h *Handler) slideshow(u *castUser, items []agent.CastItem) {
	logrus.Infof("start slide show for chromecast %s", u.name)

retry:
	for {
		err := u.cast.Connect()
		if err == nil {
			break
		}
		logrus.Errorf("connect %s get: %v", u.name, err)
		t := time.After(time.Second * 10)
		select {
		case <-u.ctx.Done():
			logrus.Errorf("user %s connect context: %v", u.name, u.ctx.Err())
			return
		case <-t:
		}
	}
	slideShowRet := make(chan error)
	go func() {
		slideShowRet <- u.cast.SlideShow(items, u.newItem)
	}()
	for {
		select {
		case <-u.ctx.Done():
			logrus.Errorf("user %s slideshow context: %v", u.name, u.ctx.Err())
			return
		case err := <-slideShowRet:
			logrus.Errorf("user %s's slide show return: %v", u.name, err)
			if err := u.cast.Close(); err != nil {
				logrus.Errorf("user %s close got: %v", u.name, err)
			}
			goto retry
		case notify := <-u.notify:
			switch notify.Status {
			case agent.CastFinished:
				logrus.Infof("user %s slide show finished", u.name)
			default:
				logrus.Infof("user %s slide show got %s", u.name, notify.Status.String())
			}
		}
	}
}

func (h *Handler) startChromecast() {
	ids := []int{}
	castSharedAssets := map[int]*share.Records{}
	h.castLock.Lock()
	for id := range h.castUsers {
		ids = append(ids, id)
	}
	h.castLock.Unlock()
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		for id := range ids {
			records, err := share.GetAllReceiveHistoryByAssets(ctx, tx, id)
			if err != nil {
				logrus.Warnf("list all shared assets for chromecast %d got: %v", id, err)
				continue
			}
			castSharedAssets[id] = records
		}
		return nil
	}); err != nil {
		logrus.Warnf("start chromecast got: %v", err)
		return
	}

	for uid, cas := range castSharedAssets {
		h.castLock.Lock()
		cu, ok := h.castUsers[uid]
		h.castLock.Unlock()
		if !ok {
			// it is possible that case users are changed
			logrus.Warnf("time racing caused %d missing", uid)
			continue
		}
		items := make([]agent.CastItem, len(cas.Records))
		for i, ca := range cas.Records {
			items[i] = h.mkCastItem(cu.token, shareURI, ca)
		}
		go h.slideshow(cu, items)
	}
}
*/

func (h *Handler) loadChromeCastUsers() error {
	return dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		//return home dir if request is from localhost
		users, err := user.ListUsers(ctx, tx)
		if err != nil {
			if !common.IsErrNoRows(err) {
				return nil
			}
			logrus.Warnf("list all user got: %v", err)
			return err
		}
		for _, u := range users.Users {
			if !u.IsChromecast() {
				continue
			}
			cu := &castUser{ctx: h.gCtx, name: u.Name, notify: make(chan agent.Notify), newItem: make(chan agent.CastItem)}
			cu.cast, err = h.createCastUser(cu.ctx, u, cu.notify)
			if err != nil {
				logrus.Warnf("check chromcast user %d got: %v", u.ID, err)
				continue
			} else if cu.cast == nil {
				logrus.Warnf("chromcast user %d has empty ip, port", u.ID)
				continue
			}
			cu.token, err = user.GetToken(ctx, tx, u.ID, chromcastLoginDevice)
			if err != nil {
				logrus.Warnf("check chromcast user %d token got: %v", u.ID, err)
				continue
			}
			h.castLock.Lock()
			h.castUsers[u.ID] = cu
			h.castLock.Unlock()

			h.userStatus[u.ID] = map[int]user.KeepaliveStatus{
				chromcastLoginDeviceID: {LastSeen: ""}}
		}
		return nil
	})
}
func (h *Handler) discoverChromeCast() {
	ids := map[int]*castUser{}
	entries, err := net.DiscoverMDNSServices(h.gCtx,
		common.MdnsChromecastService, common.MdnsChromecastDomain, 10*time.Second)
	if err != nil {
		logrus.Warnf("discover chromecast got %v", err)
		return
	}
	if len(entries) == 0 {
		logrus.Trace("no chromecast devices")
		goto resetChromcastStatus
	}

	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		for _, entry := range entries {
			id, cu, err := h.insertOrUpdateChromecastUser(ctx, tx, entry)
			if err != nil {
				logrus.Warnf("insert/update chromecast user %v got: %v", *entry, err)
				continue
			} else if id != 0 {
				// id == 0 means the dns entry may be invalid
				ids[id] = cu
			}
		}
		return nil
	}); err != nil {
		logrus.Warnf("insert/update chromecast users got: %v", err)
		return
	}

	for id, cu := range ids {
		h.userStatus[id] = map[int]user.KeepaliveStatus{
			chromcastLoginDeviceID: {LastSeen: time.Now().String()}}
		if cu == nil {
			// this means no change, so no need update cast user
			continue
		}
		h.castLock.Lock()
		h.castUsers[id] = cu
		h.castLock.Unlock()
	}
resetChromcastStatus:
	for id, s := range h.userStatus {
		_, exist := ids[id]
		if exist {
			continue
		}
		_, existBefore := s[chromcastLoginDeviceID]
		if !existBefore {
			continue
		}
		// set empty if it shows up before, but not this time. must be offline
		h.userStatus[id] = map[int]user.KeepaliveStatus{
			chromcastLoginDeviceID: {LastSeen: ""}}
	}
}

func (h *Handler) discoverChromeCastLoop() {
	// discover before loop
	h.discoverChromeCast()

	t := time.NewTicker(h.conf.CastPollDuration)
	for {
		select {
		case <-h.gCtx.Done():
			logrus.Warnf("chromecast discover return: %v", h.gCtx.Err())
			return
		case <-t.C:
			h.discoverChromeCast()
		}
	}
}

func (h *Handler) parseChromecastMDNS(entry *zeroconf.ServiceEntry) (device string, deviceName string,
	deviceUUID string, ip string, port int, err error) {
	for _, value := range entry.Text {
		if kv := strings.SplitN(value, "=", 2); len(kv) == 2 {
			switch kv[0] {
			case "md":
				device = kv[1]
			case "fn":
				deviceName = kv[1]
			case "id":
				deviceUUID = kv[1]
			}
		}
	}

	if len(entry.AddrIPv4) > 0 {
		ip = entry.AddrIPv4[0].String()
	} else if len(entry.AddrIPv6) > 0 {
		ip = entry.AddrIPv6[0].String()
	} else {
		err = errors.Errorf("chromecast %s doesn't have IP", deviceName)
		return
	}

	port = entry.Port
	return
}

// returned user id = 0, means the service entry has invalid entry
// returned castUser = nil, means data is not change.
func (h *Handler) insertOrUpdateChromecastUser(ctx context.Context, tx *sql.Tx,
	entry *zeroconf.ServiceEntry) (int, *castUser, error) {
	cu := &castUser{ctx: h.gCtx, notify: make(chan agent.Notify), newItem: make(chan agent.CastItem)}
	device, deviceName, deviceUUID, ip, port, err := h.parseChromecastMDNS(entry)
	if err != nil {
		logrus.Warnf("parse mdns chromecast record %v: %v", entry, err)
		return 0, nil, err
	} else if deviceUUID == "" || deviceName == "" {
		logrus.Warnf("empty mdns chromecast record: %s(%s)", deviceUUID, deviceName)
		return 0, nil, nil
	}
	cu.name = chromcastNamePrefix + deviceUUID
	u, err := user.GetByName(ctx, tx, cu.name)
	if err != nil {
		if err != common.ErrNotExistUser {
			return 0, nil, err
		}
		u := &user.User{
			Name: cu.name, Password: deviceUUID, NickName: deviceName, BotUser: true,
			Metadatas: map[string]interface{}{
				types.DeviceUUID:    deviceUUID,
				types.DeviceType:    types.DeviceTypeChromecast,
				types.DeviceSubType: device,
				types.DeviceName:    deviceName,
				types.DeviceIP:      ip,
				types.DevicePort:    strconv.Itoa(port),
			}}
		u.SetAdminFlag(user.BotFlag | user.CastFlag)
		if err := user.CreateUser(ctx, tx, u, h.conf.SambaConf); err != nil {
			return 0, nil, err
		}

		logrus.Infof("insert chromecast user: %v", *u)
		// login after create user immediately to get one token
		// TODO: need change once we verify token expiration
		cu.cast = agent.New(h.gCtx, ip, port, cu.notify)
		cu.token, _, err = user.Login(ctx, tx, u.Name, u.Password, chromcastLoginDevice, h.tokenDuration)
		if err != nil {
			return 0, nil, err
		}
		return u.ID, cu, nil
	}

	// compare IP
	var (
		ok      bool
		meta    interface{}
		ipStr   string
		portStr string
		portInt int
	)
	meta, ok = u.Metadatas[types.DeviceIP]
	if !ok {
		logrus.Infof("chromecast IP was not exist")
		goto update
	}
	ipStr, ok = meta.(string)
	if !ok {
		logrus.Infof("chromecast IP was not string")
		goto update
	}
	if ipStr != ip {
		logrus.Infof("chromecast IP changed from %s to %s", ipStr, ip)
		goto update
	}
	// compare port
	meta, ok = u.Metadatas[types.DevicePort]
	if !ok {
		logrus.Infof("chromecast port was not exist")
		goto update
	}
	portStr, ok = meta.(string)
	if !ok {
		logrus.Infof("chromecast port was not string")
		goto update
	}
	portInt, err = strconv.Atoi(portStr)
	if err != nil {
		return 0, nil, err
	}
	if portInt == port {
		logrus.Tracef("no change for chromecast user: %v", *u)
		return u.ID, nil, nil
	}
	logrus.Infof("chromecast port changed from %d to %d", portInt, port)

update:
	// set not updated field as empty
	logrus.Tracef("update chromecast user: %v", *u)
	// set empty means no update
	u.Password = ""
	u.Phone = ""
	u.Email = ""
	u.NickName = ""
	u.Metadatas[types.DeviceIP] = ip
	u.Metadatas[types.DevicePort] = strconv.Itoa(port)

	h.castLock.Lock()
	oldCu, ok := h.castUsers[u.ID]
	h.castLock.Unlock()
	if ok {
		if err := oldCu.cast.Close(); err != nil {
			logrus.Warnf("during update (%s:%d), close user %d: %v", ip, port, u.ID, err)
		} else {
			cu.token = oldCu.token
			cu.cast = agent.New(h.gCtx, ip, port, cu.notify)
		}
		return u.ID, cu, user.UpdateUser(ctx, tx, u)
	}
	logrus.Warnf("during update (%s:%d), unable to find user %d", ip, port, u.ID)

	return u.ID, nil, user.UpdateUser(ctx, tx, u)
}
