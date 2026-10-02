package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"time"

	agent "bitbucket.org/lomoware/lomo-backend/common/castagent"
	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	lnet "bitbucket.org/lomoware/lomo-backend/common/net"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/share"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

func (h *Handler) unshareAsset(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	// get asset info
	shareID, err := strconv.Atoi(mux.Vars(r)["shareID"])
	if err != nil {
		common.WriteError(w, err)
		return
	}

	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		return share.DeleteShare(ctx, tx, shareID, wl.Userid)
	}); err != nil {
		if common.IsErrNoRows(err) {
			common.WriteError(w, common.ErrBadRequest, err.Error())
		} else {
			common.WriteError(w, err)
		}
		return
	}
}

func (h *Handler) mkCastItem(token, path string, cs *types.Record) agent.CastItem {
	// TODO: detect real length for video
	url := "http://" + h.listenIPs[0].String() + ":" + strconv.Itoa(h.conf.ListenPort) +
		path + strconv.FormatUint(cs.ID, 10) + "?token=" + token
	if cs.Codec != "" {
		url = url + "&" + cs.Codec
	}
	return agent.CastItem{
		PreloadTime:      10,
		PlaybackDuration: 10,
		ContentURL:       url,
		ContentType:      cs.MIME,
	}
}

func (h *Handler) sendChromecast(cu *castUser, cs *types.Record) {
	item := h.mkCastItem(cu.token, shareURI, cs)
	logrus.Infof("share %s: %v", cu.name, item)
	cu.newItem <- item
}

func (h *Handler) sendMultiple(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	defer r.Body.Close()
	records := &types.Records{}
	if err := json.NewDecoder(r.Body).Decode(records); err != nil {
		common.WriteError(w, err)
		return
	}

	duration := h.parseDuration(mux.Vars(r)["duration"])
	castAssets := map[int][]*types.Record{}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		receivers := map[int]*user.User{}
		files := map[string]*[]string{}
		for _, r := range records.Records {
			_, ok := receivers[r.ReceiverID]
			if ok {
				continue
			}
			u, err := user.Get(ctx, tx, r.ReceiverID)
			if err != nil {
				return err
			}
			receivers[r.ReceiverID] = u
			files[u.Name] = &[]string{}

			h.castLock.Lock()
			_, ok = h.castUsers[r.ReceiverID]
			h.castLock.Unlock()
			if ok {
				castAssets[r.ReceiverID] = []*types.Record{}
			}
		}

		for _, r := range records.Records {
			receiver := receivers[r.ReceiverID]
			records, cu, err := h.shareToUser(ctx, tx, wl.Userid, receiver, r.AssetID,
				r.AssetIDType != nil && *r.AssetIDType == types.Hash,
				r.Type, duration, files[receiver.Name])
			if err != nil {
				return err
			}
			r.ID = records.ID
			r.SenderID = wl.Userid
			r.ReadFlag = false
			r.ShareTime = types.LomoTime{Time: time.Now().UTC()}
			if cu != nil {
				castAssets[r.ReceiverID] = append(castAssets[r.ReceiverID], records)
			}
		}
		for name, fs := range files {
			if fs == nil || len(*fs) == 0 {
				continue
			}
			// TODO: roll back and remove share ?
			if err := cmd.Chown(name+":"+common.GetLomoGroupName(), (*fs)...); err != nil {
				logrus.Errorf("chown %s - %v: %v", name, *fs, err)
			}
		}
		return nil
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, records)

	for uid, cas := range castAssets {
		h.castLock.Lock()
		cu, ok := h.castUsers[uid]
		h.castLock.Unlock()
		if !ok {
			logrus.Warnf("unable to find cast user: %d", uid)
			continue
		}
		for _, ca := range cas {
			h.sendChromecast(cu, ca)
		}
	}
}

func (h *Handler) resolveAsset(ctx context.Context, tx *sql.Tx, userID int, assetID string,
	isHash bool) (int, int, error) {
	if isHash {
		return asset.GetAssetIDByHash(ctx, tx, userID, assetID)
	}
	aid, err := asset.ParseAssetID(assetID)
	if err != nil {
		return -1, -1, err
	}
	_, extid, err := asset.GetAssetHashByID(ctx, tx, userID, aid)
	return aid, extid, err
}

func (h *Handler) parseDuration(durArg string) time.Duration {
	// TODO: support expired time
	if durArg == "" {
		return 10000 * time.Hour
	}
	duration, err := time.ParseDuration(durArg)
	if err != nil {
		duration = 10000 * time.Hour
		logrus.Warnf("receive invalid duration (%s): %v", durArg, err)
	}
	return duration
}

func (h *Handler) sendToUser(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	duration := h.parseDuration(mux.Vars(r)["duration"])
	receiverID, err := strconv.Atoi(mux.Vars(r)["userID"])
	if err != nil {
		common.WriteError(w, err)
		return
	}

	var (
		record *types.Record
		cu     *castUser
	)
	err = dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		receiver, err := user.Get(ctx, tx, receiverID)
		if err != nil {
			return err
		}
		record, cu, err = h.shareToUser(ctx, tx, wl.Userid, receiver, mux.Vars(r)["assetID"],
			r.URL.Query().Get("byhash") != "", types.ShareToUser, duration, nil)
		return err
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}
	if cu != nil {
		h.sendChromecast(cu, record)
	}
	common.WriteBody(w, record)
}

func (h *Handler) shareToUser(ctx context.Context, tx *sql.Tx, senderID int, receiver *user.User,
	assetID string, isHash bool, shareType types.ShareType, duration time.Duration,
	shareFiles *[]string) (*types.Record, *castUser, error) {
	var err error
	record := &types.Record{Type: shareType, AssetID: assetID, ReceiverID: receiver.ID,
		SenderID: senderID, ShareTime: types.LomoTime{Time: time.Now().UTC()}}
	h.castLock.Lock()
	cu, ok := h.castUsers[receiver.ID]
	h.castLock.Unlock()
	if ok {
		if len(h.listenIPs) == 0 {
			h.listenIPs, err = lnet.ListIPs()
			if err != nil {
				logrus.Warnf("while listing ips: %v", err)
				return nil, nil, err
			}
		}
	} else {
		logrus.Warnf("Cast user %d is not cached yet", receiver.ID)
	}

	aid, extid, err := h.resolveAsset(ctx, tx, senderID, assetID, isHash)
	if err != nil {
		return nil, nil, err
	}
	if ok {
		record.MIME, record.Codec, err = ext.ChromecastMIMEType(extid)
		if err != nil {
			return nil, nil, err
		}
	}
	record.ID, err = share.Share(ctx, tx, senderID, aid, receiver, shareType, time.Now().Add(duration), shareFiles)
	return record, cu, err
}

func (h *Handler) sendToGroup(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	// TODO: support expired time
	var duration time.Duration
	if mux.Vars(r)["duration"] == "" {
		duration = 10000 * time.Hour
	} else {
		var err error
		duration, err = time.ParseDuration(mux.Vars(r)["duration"])
		if err != nil {
			duration = 10000 * time.Hour
			logrus.Warnf("receive invalid duration: %s", mux.Vars(r)["duration"])
		}
	}

	groupID, err := strconv.Atoi(mux.Vars(r)["groupID"])
	if err != nil {
		common.WriteError(w, err)
		return
	}

	var shareid uint64
	assetID := mux.Vars(r)["assetID"]
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		// TODO: support chromecast in the group
		aid, _, err := h.resolveAsset(ctx, tx, wl.Userid, assetID, r.URL.Query().Get("byhash") != "")
		if err != nil {
			return err
		}
		shareid, err = share.Share(ctx, tx, wl.Userid, aid, &user.User{ID: groupID}, types.ShareToGroup,
			time.Now().Add(duration), nil)
		return err
	}); err != nil {
		if common.IsErrNoRows(err) {
			common.WriteError(w, common.ErrBadRequest, err.Error())
		} else {
			common.WriteError(w, err)
		}
		return
	}

	common.WriteBody(w, types.Record{ID: shareid, Type: types.ShareToGroup, AssetID: assetID,
		ReceiverID: groupID, SenderID: wl.Userid, ShareTime: types.LomoTime{Time: time.Now().UTC()}})
}

func (h *Handler) listSharedAssets(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	q := r.URL.Query()

	// TODO: check hidden asset or not?
	var records interface{}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if q.Get("byassets") != "" {
			records, err = share.GetAllReceiveHistoryByAssets(ctx, tx, wl.Userid)
		} else {
			records, err = share.GetAllReceiveHistory(ctx, tx, wl.Userid)
		}
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, records)
}

func (h *Handler) receiveAsset(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	shareID, err := strconv.ParseUint(mux.Vars(r)["shareID"], 10, 64)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	var runner types.PreviewRunner
	if r.URL.Query().Get(common.QueryKeyICodec) != "" {
		runner = h.previewRunner
	}

	var (
		masterFile, previewPath, assetPreviewPrefix string
		assetID, extID                              int
	)
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		masterFile, previewPath, assetPreviewPrefix, assetID, extID, err =
			share.LocateAssetPath(ctx, tx, shareID, wl.Userid, h.conf.FolderPerm)
		return err
	}); err != nil {
		if common.IsErrNoRows(err) {
			common.WriteError(w, common.ErrNotExistAsset)
		} else {
			common.WriteError(w, err)
		}
		return
	}

	// Generate outside the DB transaction above -- see ResolveAssetPreview.
	p, err := share.ResolveAssetPreview(context.Background(), masterFile, previewPath, assetPreviewPrefix,
		assetID, extID, 0, 0, runner, h.conf.FolderPerm)
	if err != nil {
		if common.IsErrNoRows(err) {
			common.WriteError(w, common.ErrNotExistAsset)
		} else {
			common.WriteError(w, err)
		}
		return
	}

	if path.Ext(p) == ".liph" {
		// TODO: use zip???
		fmt.Println("consider capability exchange")
	}
	http.ServeFile(w, r, p)
}

func (h *Handler) receiveAssetPreview(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	q := r.URL.Query()

	shareID, err := strconv.ParseUint(mux.Vars(r)["shareID"], 10, 64)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	width := common.DefaultPreviewWidth
	height := 0
	if w, err := strconv.Atoi(q.Get("width")); err == nil && w > 0 {
		width = w
	}
	if he, err := strconv.Atoi(q.Get("height")); err == nil && he > 0 {
		height = he
	}

	var (
		masterFile, previewPath, assetPreviewPrefix string
		assetID, extID                              int
	)
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		masterFile, previewPath, assetPreviewPrefix, assetID, extID, err =
			share.LocateAssetPath(ctx, tx, shareID, wl.Userid, h.conf.FolderPerm)
		return err
	}); err != nil {
		if common.IsErrNoRows(err) {
			common.WriteError(w, common.ErrNotExistAsset)
		} else {
			common.WriteError(w, err)
		}
		return
	}

	// Generate outside the DB transaction above -- see ResolveAssetPreview.
	p, err := share.ResolveAssetPreview(context.Background(), masterFile, previewPath, assetPreviewPrefix,
		assetID, extID, width, height, h.previewRunner, h.conf.FolderPerm)
	if err != nil {
		if common.IsErrNoRows(err) {
			common.WriteError(w, common.ErrNotExistAsset)
		} else {
			common.WriteError(w, err)
		}
		return
	}

	http.ServeFile(w, r, p)
}

func (h *Handler) receiveMetaFromUser(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	q := r.URL.Query()

	senderID, err := strconv.Atoi(mux.Vars(r)["userID"])
	if err != nil {
		common.WriteError(w, err)
		return
	}

	includeme := false
	if q.Get("includeme") == "1" {
		includeme = true
	}
	var records *types.Records
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		records, err = share.ReceiveHistoryByUser(ctx, tx, senderID, wl.Userid, includeme)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, records)
}

func (h *Handler) receiveMetaFromGroup(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	q := r.URL.Query()

	groupID, err := strconv.Atoi(mux.Vars(r)["groupID"])
	if err != nil {
		common.WriteError(w, err)
		return
	}

	includeme := false
	if q.Get("includeme") == "1" {
		includeme = true
	}

	var records *types.Records
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		records, err = share.ReceiveHistoryByGroup(ctx, tx, groupID, wl.Userid, includeme)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, records)
}

func (h *Handler) hideReceivedAsset(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	shareID, err := strconv.ParseUint(mux.Vars(r)["shareID"], 10, 64)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		return share.HideReceivedShare(ctx, tx, wl.Userid, shareID)
	}); err != nil {
		common.WriteError(w, err)
		return
	}
}
