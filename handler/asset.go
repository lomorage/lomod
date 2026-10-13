package handler

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/album"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/exif"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/geo"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/scene"
	"bitbucket.org/lomoware/lomo-backend/common/share"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	keyLastSaveSize = "size"
	keyLastSaveSHA1 = "sha1"
)

var dupRegex = regexp.MustCompile(common.ErrDuplicate.Error())

func (h *Handler) createAsset(w http.ResponseWriter, r *http.Request) {
	h.saveAsset(w, r, &types.LastSavedAsset{FinalSHA: mux.Vars(r)["sha1"]})
}

func (h *Handler) createAssetWithQuery(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	h.saveAsset(w, r, &types.LastSavedAsset{FinalSHA: q.Get(common.QueryKeyHash)})
}

func (h *Handler) patchAsset(w http.ResponseWriter, r *http.Request) {
	hash := mux.Vars(r)["sha1"]
	parts := strings.Split(r.Header.Get("If-Match"), ",")
	if len(parts) != 2 {
		logrus.Warnf("If-Match (%s) should have two values", r.Header.Get("If-Match"))
		common.WriteError(w, common.ErrBadRequest)
		return
	}
	lsa := types.LastSavedAsset{FinalSHA: hash}
	for _, part := range parts {
		parts2 := strings.Split(part, "=")
		if len(parts2) != 2 {
			logrus.Warnf("Each If-Match (%s) should have two values", r.Header.Get("If-Match"))
			common.WriteError(w, common.ErrBadRequest)
			return
		}
		key := strings.ToLower(strings.TrimSpace(parts2[0]))
		value := strings.ToLower(strings.TrimSpace(parts2[1]))
		if key == keyLastSaveSize {
			size, err := strconv.Atoi(value)
			if err != nil {
				common.WriteError(w, err)
				return
			}
			lsa.CurrSize = int64(size)
		} else if key == keyLastSaveSHA1 {
			lsa.CurrSHA = value
		} else {
			common.WriteError(w, common.ErrBadRequest)
			return
		}
	}

	h.saveAsset(w, r, &lsa)
}

func (h *Handler) saveAsset(w http.ResponseWriter, r *http.Request, savedAsset *types.LastSavedAsset) {
	h.CORS(w, r)

	if !savedAsset.Valid() {
		logrus.Warnf("invalid saved asset: %v", savedAsset)
		common.WriteError(w, common.ErrBadRequest)
		return
	}

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	// check user disk mount status
	h.userMountLock.Lock()
	mi, ok := h.userMountStatus[wl.Username]
	h.userMountLock.Unlock()
	if ok && mi.homeErr != nil {
		common.WriteError(w, mi.homeErr)
		return
	}

	q := r.URL.Query()
	// check extension
	extension := strings.ToLower(q.Get(common.QueryKeyExt))
	extID, err := ext.GetExtID(extension)
	if err != nil {
		common.WriteError(w, common.ErrNotImplementedFormat, "extension: "+extension)
		return
	}

	a, err := h.isExist(wl.Userid, savedAsset.FinalSHA)
	if err != nil {
		logrus.Warnf("%d - %s validate exist: %v", wl.Userid, savedAsset.FinalSHA, err)
		common.WriteError(w, err)
		return
	} else if a != nil {
		w.WriteHeader(http.StatusConflict)
		common.WriteBody(w, a)
		return
	}

	h.uploadCh <- struct{}{}
	defer func() { <-h.uploadCh }()

	// pause new preview generation
	h.previewRunner.Pause()
	defer h.previewRunner.Resume()

	// write file to uploaded user's temp dir
	baseDir := ""
	a = &types.Asset{Hash: savedAsset.FinalSHA}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		baseDir, err = user.GetHomedir(ctx, tx, wl.Userid)
		return err
	}); err != nil {
		common.WriteError(w, err, fmt.Sprintf("get %s home dir for import %s", wl.Username, savedAsset.FinalSHA))
		return
	}

	filename, filenameImg, exifTags, err := h.saveAssetValidateSHA(savedAsset,
		q.Get(common.QueryKeyFileHash), baseDir, extension, r.Body)
	if err != nil {
		common.WriteError(w, err, "save asset "+savedAsset.FinalSHA)
		return
	}

	if exifTags == nil {
		if extID == ext.ZIP {
			// JPE and HEIC should have same effect here
			exifTags, err = exif.NewTags(filenameImg, h.exiftool, extension)
		} else {
			exifTags, err = exif.NewTags(filename, h.exiftool, extension)
		}
		if err != nil {
			logrus.Warnf("extract exifTags: %v", err)
		}
	}

	if err := h.validateAsset(a, exifTags, q.Get(common.QueryKeyCreateTime), q.Get(common.QueryKeyModifiedtime)); err != nil {
		common.WriteError(w, err, "validate asset time "+savedAsset.FinalSHA)
		return
	}

	stat, err := os.Stat(filename)
	if err != nil {
		common.WriteError(w, err, "create asset "+savedAsset.FinalSHA)
		return
	} else if stat.Size() == 0 {
		common.WriteError(w, errors.New("empty size"), "create asset "+savedAsset.FinalSHA)
		return
	}

	var assetPath, previewDir string
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		assetPath, previewDir, err = asset.CreateAsset(ctx, tx, wl.Userid, wl.Deviceid, extID, filename, filenameImg,
			a, h.conf.FolderPerm, true)
		if err != nil {
			return err
		}
		if exifTags == nil {
			return nil
		}
		assetID, err := ext.GetAssetIDByName(a.Name)
		if err != nil {
			return err
		}
		err = exif.InsertAssetEXIF(ctx, tx, assetID, exifTags.RawTags)
		if err != nil {
			return err
		}

		maker := exifTags.GetMake()
		if maker == "" {
			logrus.Errorf("failed to get asset %s's make: %v", a.Name, exifTags.RawTags)
			return nil
		}
		model := exifTags.GetModel()
		if model == "" {
			logrus.Warnf("asset %s's camera model is empty: %v", a.Name, exifTags.RawTags)
		}
		makeID, err := exif.InsertOrGetCameraMakeID(ctx, tx, maker, model)
		if err != nil {
			return err
		}
		return exif.AssociateAssetWithCameraMake(ctx, tx, assetID, makeID)
	}); err != nil {
		if dupRegex.MatchString(err.Error()) {
			w.WriteHeader(http.StatusConflict)
			common.WriteBody(w, a)
			return
		}
		common.WriteError(w, err, "create asset "+savedAsset.FinalSHA)
		return
	}

	if h.conf.UseMemdb {
		h.memdb.Lock()
		h.memdb.Insert(*a, wl.Userid, a.Date.Time.Year(), int(a.Date.Time.Month()), a.Date.Time.Day())
		h.memdb.Unlock()
	}
	common.WriteBody(w, a)

	go h.updateAssetSummary(wl.Userid, extID, 1, stat.Size())

	if ext.IsSkipPreview(extension) {
		return
	}
	if !h.conf.UseJpg {
		h.previewRunner.Generate(assetPath, previewDir, "", extension, false, true)
	} else {
		h.previewRunner.Generate(assetPath, previewDir, "", extension, false, false)
	}
}

func (h *Handler) updateAssetSummary(uid, eid int, count, size int64) {
	e, err := ext.GetExtString(eid)
	if err != nil {
		logrus.Warnf("unable to find extension %d: %s\n", eid, err)
		e = strconv.Itoa(eid)
	}
	h.userAssetLock.Lock()
	uas, ok := h.userAssetSummary[uid]
	if !ok {
		uas = map[string]types.AssetSummary{}
	}
	as, ok := uas[e]
	if !ok {
		as = types.AssetSummary{Count: count, Size: size}
	} else {
		as.Count += count
		as.Size += size
	}
	uas[e] = as
	h.userAssetSummary[uid] = uas
	h.userAssetLock.Unlock()
}

func (h *Handler) isExist(userid int, hash string) (*types.Asset, error) {
	if h.conf.UseMemdb {
		h.memdb.Lock()
		defer h.memdb.Unlock()
		a := h.memdb.ExistByHash(userid, hash)
		if a != nil {
			return a, nil
		}
	}

	var a *types.Asset
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		// TODO: removed if we can trust memdb validation
		aid, eid, err := asset.GetAssetIDByHash(ctx, tx, userid, hash)
		if err != nil {
			return err
		}
		a = &types.Asset{}
		a.Name, err = ext.MkAssetNameByID(aid, eid)
		return err
	}); err != nil && err != common.ErrAssetNotExistForUser {
		return nil, err
	}

	if a != nil {
		logrus.Warnf("%d - %s not exist in memdb, but exist in asset db", userid, hash)
	}
	return a, nil
}

func (h *Handler) saveAssetValidateSHA(savedAsset *types.LastSavedAsset, fileSHAReq, baseDir, extension string,
	r io.ReadCloser) (string, string, *exif.Tags, error) {
	if extension == ext.ZIPString {
		filename, filenameImg, err := asset.SaveLivephoto(baseDir, savedAsset, r, sha1.New(), h.conf.FolderPerm, h.conf.FilePerm)
		return filename, filenameImg, nil, err
	}
	contentSHA := ""
	// only handle mov file type
	if extension == ext.MOVString && fileSHAReq != "" {
		// SHA in path is content based sha, not file based sha, which may be different by data/time
		// replace with this parameter for integrity validation
		contentSHA = savedAsset.FinalSHA
		savedAsset.FinalSHA = fileSHAReq
	}
	filename, err := asset.SaveAsset(baseDir, savedAsset, r, sha1.New(), h.conf.FolderPerm, h.conf.FilePerm)
	if err != nil {
		return "", "", nil, err
	}
	if contentSHA == "" {
		return filename, "", nil, nil
	}
	exifTags, err := exif.NewTags(filename, h.exiftool, extension)
	if err != nil {
		return "", "", nil, err
	}

	sha := exifTags.GetLomoOriginSHA()
	if sha == "" {
		return "", "", nil, common.ErrAssetLomoOriginSHANotFound
	} else if sha != contentSHA {
		return "", "", nil, errors.Errorf("ValidateLomorageOriginSHA read SHA: %s, but expect %s", sha, contentSHA)
	}

	savedAsset.FinalSHA = contentSHA
	return filename, "", exifTags, nil
}

func (h *Handler) validateAsset(a *types.Asset, exifTags *exif.Tags, createTime, lastModified string) error {
	var (
		dt  time.Time
		err error
	)
	if createTime != "" {
		// skip validate and use input create time
		dt, err = time.Parse(common.TimeFormatLomod, createTime)
		if err != nil {
			return err
		}
	} else if exifTags != nil {
		// analysis picture's timestamp
		dt, err = exifTags.GetCreateTime()
		if err != nil {
			logrus.Warnf("probe %s's create time got %v", filepath.Base(exifTags.Filename()), err)
		}
	}
	if dt.IsZero() {
		if lastModified == "" {
			logrus.Warnf("No create time for %s-%s", a.Name, a.Hash)
			return common.ErrNoCreateTime
		}
		// try last_modified_time
		dt, err = time.Parse(common.TimeFormatLomod, lastModified)
		if err != nil {
			logrus.Warnf("Invalid last modified time for %s-%s: %v", a.Name, a.Hash, err)
			return common.ErrNoCreateTime
		}
	}

	a.Date = types.LomoTime{Time: dt}
	if exifTags != nil {
		exifTags.SetCreateTime(dt)

		a.Latitude = exifTags.GetLatitude()
		a.Longitude = exifTags.GetLongitude()
	} else {
		a.Latitude = exif.UnknownGPS
		a.Longitude = exif.UnknownGPS
	}
	return nil
}

func (h *Handler) getAsset(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	if r.URL.Query().Get(common.QueryKeyInfo) != "" {
		if !h.conf.UseMemdb {
			common.WriteError(w, common.ErrNotImplementedFormat)
			return
		}
		h.memdb.Lock()
		a := h.memdb.ExistByHash(wl.Userid, mux.Vars(r)["assetID"])
		h.memdb.Unlock()
		if a == nil {
			logrus.Warnf("asset %s not exist", mux.Vars(r)["assetID"])
			common.WriteError(w, common.ErrNotExistAsset)
		} else {
			common.WriteBody(w, a)
		}

		return
	}

	h.downloadAsset(w, r, wl)
}

func (h *Handler) downloadAsset(w http.ResponseWriter, r *http.Request, wl *logger.ResponseLogger) {
	// check user disk mount status
	h.userMountLock.Lock()
	mi, ok := h.userMountStatus[wl.Username]
	h.userMountLock.Unlock()
	if ok && mi.homeErr != nil {
		common.WriteError(w, mi.homeErr)
		return
	}

	assetID := mux.Vars(r)["assetID"]
	icodec := ext.JPG
	if r.URL.Query().Get(common.QueryKeyICodec) != "" {
		var err error
		icodec, err = ext.GetExtID(r.URL.Query().Get(common.QueryKeyICodec))
		if err != nil {
			common.WriteError(w, err)
			return
		}
	}
	masterfile := ""
	previewfile := ""
	idx := types.Index
	if r.URL.Query().Get("byhash") != "" || len(assetID) == 40 {
		idx = types.Hash
	}
	width := 0
	if r.URL.Query().Get(common.QueryKeyWidth) != "" {
		// skip error check because it is zero if parsing failure
		var err error
		width, err = strconv.Atoi(r.URL.Query().Get(common.QueryKeyWidth))
		if err != nil {
			logrus.Warnf("invalid width query parameter %s during download asset %s",
				r.URL.Query().Get(common.QueryKeyWidth), assetID)
		}
	}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		assetid, _, err := asset.NormalizeAssetID(ctx, tx, wl.Userid, mux.Vars(r)["assetID"], idx)
		if err != nil {
			return err
		}
		if r.URL.Query().Get(common.QueryKeyICodec) != "" {
			masterfile, previewfile, err = asset.GetAssetMasterPreviewPath(ctx, tx, wl.Userid, assetid,
				uint(width), 0, &icodec, h.previewRunner, h.conf.FolderPerm)
		} else if len(h.previewVideoDims) != 0 {
			masterfile, previewfile, err = asset.GetAssetMasterPreviewPath(ctx, tx, wl.Userid, assetid,
				h.previewVideoDims[0].Width, h.previewVideoDims[0].Height, nil, h.previewRunner, h.conf.FolderPerm)
		} else {
			masterfile, previewfile, err = asset.GetAssetMasterPreviewPath(ctx, tx, wl.Userid, assetid,
				0, 0, nil, h.previewRunner, h.conf.FolderPerm)
		}
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	// asset content is addressed by hash (+ codec/dimension query params) and never mutates in
	// place, so it's safe to let browsers cache it indefinitely without revalidation. Set only
	// once a file has actually been resolved, so error responses above stay uncached.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

	if r.URL.Query().Get(common.QueryKeyOrig) != "" || previewfile == "" {
		http.ServeFile(w, r, masterfile)
		return
	}

	// only reply if video or jitt asset is created
	if ok, err := common.IsFileExist(previewfile); err == nil && ok {
		http.ServeFile(w, r, previewfile)
		return
	}
	http.ServeFile(w, r, masterfile)
}

func (h *Handler) parsePreviewDimension(r *http.Request) (width uint, height uint) {
	q := r.URL.Query()
	width = common.DefaultPreviewWidth
	if w, err := strconv.Atoi(q.Get(common.QueryKeyWidth)); err == nil && w > 0 {
		width = uint(w)
	}
	if he, err := strconv.Atoi(q.Get(common.QueryKeyHeight)); err == nil && he > 0 {
		height = uint(he)
	}
	return
}

func (h *Handler) getAssetPreview(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	// check user disk mount status
	h.userMountLock.Lock()
	mi, ok := h.userMountStatus[wl.Username]
	h.userMountLock.Unlock()
	if ok && mi.homeErr != nil {
		common.WriteError(w, mi.homeErr)
		return
	}

	width, height := h.parsePreviewDimension(r)

	if !h.acquirePreviewSlot() {
		common.WriteError(w, common.ErrPreviewBusy)
		return
	}
	defer h.releasePreviewSlot()

	var err error
	icodec := ext.JPG
	ic := r.URL.Query().Get(common.QueryKeyICodec)
	if ic != "" {
		icodec, err = ext.GetExtID(ic)
		if err != nil {
			common.WriteError(w, err)
			return
		}
	}
	assetID := mux.Vars(r)["assetID"]
	idx := types.Index
	if len(assetID) == 40 {
		idx = types.Hash
	} else {
		assetID = strings.Split(assetID, ".")[0]
	}

	previewFile, err := h.getAssetPreviewPath(wl.Userid, icodec, assetID, width, height, idx)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	// preview content is addressed by hash + requested dimensions/codec and never mutates in
	// place, so it's safe to let browsers cache it indefinitely without revalidation. Set only
	// once a file has actually been resolved, so error responses above stay uncached.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

	http.ServeFile(w, r, previewFile)
}

func (h *Handler) getAssetPreviewPath(userID, icodec int, assetID string, width, height uint,
	typ types.AssetIDType) (string, error) {
	var (
		aid                                         int
		masterFile, previewPath, assetPreviewPrefix string
		extID                                       int
		skip                                        bool
	)
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var eid int
		var err error
		aid, eid, err = asset.NormalizeAssetID(ctx, tx, userID, assetID, typ)
		if err != nil {
			return err
		}
		if ext.IsSkipPreviewByID(eid) {
			skip = true
			return nil
		}
		masterFile, previewPath, assetPreviewPrefix, extID, err = asset.GetAssetPath(ctx, tx, userID, aid,
			h.conf.FolderPerm)
		if err != nil && common.IsErrNoRows(err) {
			err = common.ErrNotExistAsset
		}
		return err
	})
	if err != nil || skip {
		return "", err
	}

	// Generate (or find cached) the preview outside the DB transaction above: on a cache
	// miss this decodes/transcodes the master file, which is slow relative to this NAS's
	// single DB connection (handler/dbtrace.go SetMaxOpenConns(1)) -- holding the
	// transaction across it stalls every other DB-touching request behind this one until
	// it finishes. See GenerateAssetPreview.
	_, previewfile, err := asset.GenerateAssetPreview(context.Background(), masterFile, previewPath,
		assetPreviewPrefix, aid, extID, width, height, &icodec, h.previewRunner, h.conf.FolderPerm)
	return previewfile, err
}

func (h *Handler) getAssetHashInfo(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	// check user disk mount status
	h.userMountLock.Lock()
	mi, ok := h.userMountStatus[wl.Username]
	h.userMountLock.Unlock()
	if ok && mi.homeErr != nil {
		common.WriteError(w, mi.homeErr)
		return
	}

	var (
		lsa     *types.LastSavedAsset
		homeDir string
	)
	id := mux.Vars(r)["sha1"]
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		_, _, err := asset.GetAssetIDByHash(ctx, tx, wl.Userid, id)
		if err == nil {
			return nil
		} else if err != common.ErrAssetNotExistForUser {
			return err
		}
		homeDir, err = user.GetHomedir(ctx, tx, wl.Userid)
		return err
	})
	if err == nil && homeDir != "" {
		// check uploaded partial content -- outside the transaction: without a saved resume
		// state this hashes the partial file, and lomod has a single DB connection
		lsa, err = asset.GetPartialUploadContent(homeDir, id)
	}
	if err != nil {
		logrus.Warnf("check %d's hash %s: %v", wl.Userid, id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if lsa == nil {
		// content is exist
		return
	}
	if lsa.CurrSize == 0 {
		w.WriteHeader(http.StatusNotFound)
	} else {
		common.SetHeaderIfMatch(w.Header(), lsa.CurrSize, lsa.CurrSHA)
		w.WriteHeader(http.StatusPartialContent)
	}
}

func (h *Handler) deleteAsset(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	// check user disk mount status
	h.userMountLock.Lock()
	mi, ok := h.userMountStatus[wl.Username]
	h.userMountLock.Unlock()
	if ok && mi.homeErr != nil {
		common.WriteError(w, mi.homeErr)
		return
	}

	q := r.URL.Query()

	// get asset info
	typ := types.Index
	if q.Get(common.QueryKeyBahash) != "" {
		typ = types.Hash
	}
	force := false
	if q.Get(common.QueryKeyForce) != "" {
		force = true
	}
	id := mux.Vars(r)["assetID"]
	if err := h.deleteAsset1(wl.Userid, id, typ, force, false); err != nil {
		common.WriteError(w, err)
	}
}

// skipAsset is used when master file is already removed by human
func (h *Handler) deleteAsset1(uid int, aid string, typ types.AssetIDType, force, skipAsset bool) error {
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		assetID, _, err := asset.NormalizeAssetID(ctx, tx, uid, aid, typ)
		if err != nil {
			logrus.Warnf("while delete asset %s, normalize asset ID: %v", aid, err)
			return err
		}
		err = h.deleteAssetByID(ctx, tx, uid, assetID, force)
		if err != nil {
			logrus.Warnf("while delete asset %s from DB and file system: %v", aid, err)
		}
		if !skipAsset {
			return err
		} else if err != nil && strings.Contains(err.Error(), "no such file or directory") {
			// caller ignore this failure during delete
			return nil
		}
		return err
	}); err != nil {
		return err
	}

	if h.conf.UseMemdb {
		h.memdb.Lock()
		err := h.memdb.Remove(uid, aid, typ)
		h.memdb.Unlock()
		if err != nil {
			logrus.Warnf("delete %s from memdb got %v", aid, err)
		}
	}
	return nil
}

// skipAsset is used when master file is already removed by human
func (h *Handler) deleteAssetByID(ctx context.Context, tx *sql.Tx, uid, assetID int, force bool) error {
	// delete share firstly
	shareIDs, err := share.GetShareIDs(ctx, tx, uid, assetID)
	if err != nil {
		logrus.Warnf("while delete asset %d, get share ID: %v", assetID, err)
		return err
	}
	for _, sid := range shareIDs {
		if err := share.DeleteShare(ctx, tx, sid, uid); err != nil {
			logrus.Warnf("while delete asset %d, delete share ID %d: %v", assetID, sid, err)
			return err
		}
	}
	err = album.DeleteAssets(ctx, tx, -1, []int{assetID})
	if err != nil {
		logrus.Warnf("while delete asset %d, delete album: %s", assetID, err)
		return err
	}
	err = asset.DeleteMetadatas(ctx, tx, assetID)
	if err != nil {
		logrus.Warnf("while delete asset %d, delete metadata: %s", assetID, err)
		return err
	}

	err = geo.DeAssociateAsset(ctx, tx, assetID)
	if err != nil {
		logrus.Warnf("while delete asset %d, delete geo album: %s", assetID, err)
		return err
	}
	err = scene.DeAssociateAsset(ctx, tx, assetID)
	if err != nil {
		logrus.Warnf("while delete asset %d, delete scene album: %s", assetID, err)
		return err
	}
	err = exif.DeAssociateAsset(ctx, tx, assetID)
	if err != nil {
		logrus.Warnf("while delete asset %d, delete camera make album: %s", assetID, err)
		return err
	}

	err = asset.RemoveLabelForAssets(ctx, tx, -1, []int{assetID})
	if err != nil {
		logrus.Warnf("while delete asset %d, delete scene label: %s", assetID, err)
		return err
	}

	return asset.DeleteAsset(ctx, tx, uid, assetID, force, h.conf.FolderPerm)
}

func (h *Handler) deleteAssetByJSON(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	// check user disk mount status
	h.userMountLock.Lock()
	mi, ok := h.userMountStatus[wl.Username]
	h.userMountLock.Unlock()
	if ok && mi.homeErr != nil {
		common.WriteError(w, mi.homeErr)
		return
	}

	dr := &types.DeleteAssetItems{}
	if err := json.NewDecoder(r.Body).Decode(dr); err != nil {
		common.WriteError(w, err)
		return
	}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		for i, req := range dr.List {
			assetID, _, err := asset.NormalizeAssetID(ctx, tx, wl.Userid, req.ID, req.Type)
			if err != nil {
				logrus.Warnf("delete %v from real db got: %v", req, err)
				dr.List[i].Result = false
				dr.List[i].Reason = err.Error()
				continue
			}
			if err := h.deleteAssetByID(ctx, tx, wl.Userid, assetID, req.Force); err != nil {
				logrus.Warnf("delete %v from real db got: %v", req, err)
				dr.List[i].Result = false
				dr.List[i].Reason = err.Error()
			} else {
				dr.List[i].Result = true
			}
		}
		return nil
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	// This error most likely won't happen
	for _, req := range dr.List {
		if h.conf.UseMemdb {
			h.memdb.Lock()
			err := h.memdb.Remove(wl.Userid, req.ID, req.Type)
			h.memdb.Unlock()
			if err != nil {
				logrus.Warnf("delete %v from memdb got %v", req, err)
			}
		}
	}

	common.WriteBody(w, dr)
}

func daysInMonth(year int, m time.Month) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func validateMonth(yStr, mStr string) (int, int, error) {
	y, err := strconv.Atoi(yStr)
	if err != nil {
		return 0, 0, err
	}
	m, err := strconv.Atoi(mStr)
	if err != nil {
		return 0, 0, err
	}
	if m > 12 || m <= 0 {
		return 0, 0, common.ErrInvalidMonth
	}
	return y, m, err
}

func validateDate(yStr, mStr, dStr string) (int, int, int, error) {
	y, m, err := validateMonth(yStr, mStr)
	if err != nil {
		return 0, 0, 0, err
	}
	d, err := strconv.Atoi(dStr)
	if err != nil {
		return 0, 0, 0, err
	}

	days := daysInMonth(y, time.Month(m))
	if d > days || d <= 0 {
		return 0, 0, 0, common.ErrInvalidDay
	}
	return y, m, d, nil
}

func (h *Handler) listAssetsByDay(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	y, m, d, err := validateDate(mux.Vars(r)["y"], mux.Vars(r)["m"], mux.Vars(r)["d"])
	if err != nil {
		common.WriteError(w, err)
		return
	}

	if h.conf.UseMemdb {
		h.memdb.Lock()
		day := h.memdb.GetAssetsByDay(wl.Userid, y, m, d)
		h.memdb.Unlock()
		common.WriteBody(w, day)
		return
	}

	var day types.Day
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		day, err = asset.GetAssetsByDay(ctx, tx, wl.Userid, y, m, d)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, day)
}

func (h *Handler) listAssetsByMonth(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	y, m, err := validateMonth(mux.Vars(r)["y"], mux.Vars(r)["m"])
	if err != nil {
		common.WriteError(w, err)
		return
	}

	if h.conf.UseMemdb {
		h.memdb.Lock()
		month := h.memdb.GetAssetsByMonth(wl.Userid, y, m)
		h.memdb.Unlock()
		common.WriteBody(w, month)
		return
	}

	var month types.Month
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		month, err = asset.GetAssetsByMonth(ctx, tx, wl.Userid, y, m)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, month)
}

func (h *Handler) listAssetsByYear(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	y, err := strconv.Atoi(mux.Vars(r)["y"])
	if err != nil {
		common.WriteError(w, err)
		return
	}

	if h.conf.UseMemdb {
		h.memdb.Lock()
		year := h.memdb.GetAssetsByYear(wl.Userid, y, true)
		h.memdb.Unlock()
		common.WriteBody(w, year)
		return
	}

	var year types.Year
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		// get all days at that month
		year, err = asset.GetAssetsByYear(ctx, tx, wl.Userid, y, true)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, year)
}

func (h *Handler) listAssetsYearMonth(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	if h.conf.UseMemdb {
		h.memdb.Lock()
		years := h.memdb.GetAssetsByYears(wl.Userid, r.URL.Query().Get(common.QueryKeyAll) == "1")
		h.memdb.Unlock()
		common.WriteBody(w, years)
		return
	}

	var years types.Years
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		years, err = asset.GetAssetsByYears(ctx, tx, wl.Userid, r.URL.Query().Get(common.QueryKeyAll) == "1")
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, years)
}

func (h *Handler) updateAssetTime(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	// check user disk mount status
	h.userMountLock.Lock()
	mi, ok := h.userMountStatus[wl.Username]
	h.userMountLock.Unlock()
	if ok && mi.homeErr != nil {
		common.WriteError(w, mi.homeErr)
		return
	}

	hash := mux.Vars(r)["sha1"]
	year, err := strconv.Atoi(mux.Vars(r)["year"])
	if err != nil {
		common.WriteError(w, err)
		return
	} else if year < common.StartYear {
		common.WriteError(w, fmt.Errorf("invalid year, must start from 1970"))
		return
	}
	month, err := strconv.Atoi(mux.Vars(r)["month"])
	if err != nil {
		common.WriteError(w, err)
		return
	} else if month < 1 || month > 12 {
		common.WriteError(w, fmt.Errorf("invalid month, must be >=1 and <=12"))
		return
	}
	day, err := strconv.Atoi(mux.Vars(r)["day"])
	if err != nil {
		common.WriteError(w, err)
		return
	} else if day < 1 || day > common.DaysInMonth[month-1] {
		common.WriteError(w, fmt.Errorf("invalid day in the month"))
		return
	}

	var a *types.Asset
	err = dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		aid, _, err := asset.GetAssetIDByHash(ctx, tx, wl.Userid, hash)
		if err != nil {
			return err
		}
		a, err = asset.UpdateAssetTime(ctx, tx, wl.Userid, aid, year, month, day, h.conf.FolderPerm)
		return err
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if h.conf.UseMemdb {
		h.memdb.Lock()
		err := h.memdb.Remove(wl.Userid, hash, types.Hash)
		if err == nil {
			h.memdb.Insert(*a, wl.Userid, year, month, day)
		}
		h.memdb.Unlock()
		if err != nil {
			logrus.Warnf("delete %s from memdb got %v", hash, err)
		}
	}
}

func (h *Handler) postPreviewGeneration(ctx context.Context, notify chan types.PreviewRequest,
	imgWidth, videoWidth uint) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-notify:
			if !req.IsWebp && !req.GenVideo {
				// only webp preview or video preview
				continue
			}
			if (req.IsWebp && req.Width != imgWidth) ||
				(req.GenVideo && req.Width != videoWidth) {
				continue
			}
			previewFile, _ := req.MkPreviewFileName()
			tags := req.MasterTags
			if tags == nil {
				var err error
				logrus.Debugf("post preview analysis parse exif %s", previewFile)
				tags, err = exif.NewTags(previewFile, h.exiftool, "")
				if err != nil {
					logrus.Warnf("during post preview analysis, probe exif %s: %v", previewFile, err)
					continue
				}
			}
			id, err := ext.ExtractAssetIDByPreviewFile(previewFile)
			if err != nil {
				logrus.Warnf("during post preview analysis, extract asset ID: %v", err)
				continue
			}
			err = dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, "update asset set aspect_ratio = ? where id = ?",
					float32(tags.GetWidth())/float32(tags.GetHeight()), id)
				return err
			})
			if err != nil {
				logrus.Warnf("during post preview analysis, update %s aspect ration: %v", previewFile, err)
			}
		}
	}
}
