package handler

import (
	"archive/zip"
	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/album"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type siteSetting struct {
	AllowRegistration bool `json:"allow_registration"`
	FetchWorker int `json:"fetch_worker"`
}

func (h *Handler) getSiteSettings(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	common.WriteBody(w, siteSetting{})
}

type rqAvailable struct {
	JobDetail bool `json:"job_detail"`
}

func (h *Handler) getRQAvailable(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	common.WriteBody(w, rqAvailable{})
}


type photo struct {
	ImageHash string `json:"image_hash"`
}

type dayPhoto struct {
	Count int `json:"count"`
	Next string `json:"next"`
	Previous string `json:"previous"`
	Results *[]types.AssetsByDay `json:"results"`
}

func (h *Handler) getAllAssetsByDate(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	id := mux.Vars(r)["date"]
	if id == "list" {
		h.memdb.Lock()
		assets := h.memdb.ListAssetsCountByDate(wl.Userid)
		h.memdb.Unlock()

		common.WriteBody(w, &dayPhoto{Results: assets})
		return
	} else if id == "notimestamp" {
		common.WriteBody(w, &dayPhoto{Results: nil})
		return
	}

    y, m, d, err := types.ParseAlbumDateID(id)
    if err != nil {
	common.WriteError(w, err)
		return
	}
	h.memdb.Lock()
    assets := h.memdb.ListAssetsByDate(wl.Userid, y, m, d)
	h.memdb.Unlock()

	common.WriteBody(w, assets)
	return
}

func (h *Handler) getThumbnail(w http.ResponseWriter, r *http.Request, width int) {
	wl := w.(*logger.ResponseLogger)

	// check user disk mount status
	h.userMountLock.Lock()
	mi, ok := h.userMountStatus[wl.Username]
	h.userMountLock.Unlock()
	if ok && mi.homeErr != nil {
		common.WriteError(w, mi.homeErr)
		return
	}

	if !h.acquirePreviewSlot() {
		common.WriteError(w, common.ErrPreviewBusy)
		return
	}
	defer h.releasePreviewSlot()

	assetID := mux.Vars(r)["assetID"]
	idx := types.Index
	if len(assetID) == 40 {
		idx = types.Hash
	}

	icodec := ext.WebP
	parts := strings.Split(assetID, ".")
	if len(parts) > 1 {
		if strings.HasSuffix(parts[0], "_video") {
			aid := strings.TrimSuffix(parts[0], "_video")
			id, err := strconv.Atoi(aid)
			if err != nil {
				common.WriteError(w, err)
				return
			}
			h.getThumbnailLivePhoto(w, wl.Userid, id)
			return
		} else if ext.IsVideoFile(parts[1]) {
			icodec = ext.MP4
			width = common.DefaultVideoPreviewWidth
		}
	}
	previewFile, err := h.getAssetPreviewPath(wl.Userid, icodec, assetID, uint(width), 0, idx)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	http.ServeFile(w, r, previewFile)
}

func (h *Handler) getThumbnailSmall(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	h.getThumbnail(w, r, common.SmallPreviewWidth)
}

func (h *Handler) getThumbnailMedian(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	h.getThumbnail(w, r, common.MedianPreviewWidth)
}

func (h *Handler) getThumbnailBig(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	h.getThumbnail(w, r, common.MaxPreviewWidth)
/*
	parts := strings.Split(mux.Vars(r)["assetID"], ".")
	if len(parts) > 1 {
		if strings.HasSuffix(parts[0], "_video") {
			aid := strings.TrimSuffix(parts[0], "_video")
			id, err := strconv.Atoi(aid)
			if err != nil {
				common.WriteError(w, err)
				return
			}
			h.getThumbnailLivePhoto(w, wl.Userid, id)
			return
		} else if ext.IsLivePhotoExtension(parts[1]){
			id, err := strconv.Atoi(parts[0])
			if err != nil {
				common.WriteError(w, err)
				return
			}
			h.getThumbnailLivePhoto(w, wl.Userid, id)
			return
		} else if parts[1] == ext.HEICString || parts[1] == ext.HEIFString ||
			parts[1] == ext.BmpString || parts[1] == ext.DngString {
			values := r.URL.Query()
			values.Add(common.QueryKeyICodec, ext.WebPString)
			r.URL.RawQuery = values.Encode()
		} else if parts[1] == ext.WebPString && h.isAssetVideo(parts[0]){
			values := r.URL.Query()
			values.Add(common.QueryKeyICodec, ext.WebPString)
			values.Add(common.QueryKeyWidth, strconv.Itoa(common.MaxPreviewWidth))
			r.URL.RawQuery = values.Encode()
		}
	}

	h.downloadAsset(w, r, wl)
 */
}

func (h *Handler) isAssetVideo(id string) (isVideo bool) {
	dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		eid, err := asset.GetAssetExtensionByID(ctx, tx, id)
		if err != nil {
			return err
		}
		isVideo = ext.IsVideoFileByID(eid)
		return nil
	})
	return
}

func (h *Handler) getThumbnailLivePhoto(w http.ResponseWriter, userID, assetID int) {
	masterFile := ""
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		masterFile, _, err = asset.GetAssetMasterPreviewPath(ctx, tx, userID, assetID, 0, 0, nil,
			h.previewRunner, h.conf.FolderPerm)
		return err
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}
	r, err := zip.OpenReader(masterFile)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	defer r.Close()

	found := false
	files := []string{}
	for _, f := range r.File {
		files = append(files, f.Name)
		e := filepath.Ext(f.Name)
		if e != ".mov" && e != ".mp4" {
			continue
		}
		found = true
		rc, err := f.Open()
		if err != nil {
			common.WriteError(w, err)
			return
		}
		defer rc.Close()
		_, err = io.Copy(w, rc)
		if err != nil {
			common.WriteError(w, err)
		}
		break
	}

	if !found {
		common.WriteError(w, errors.Errorf("invalid live photo, no video file found: %v", files))
	}
	return
}

type assetUpdateReply struct {
	Status bool `json:"status"`
	Results []*assetDetail `json:"results"`
	Updated []*assetDetail `json:"updated"`
	NotUpdated []*assetDetail `json:"not_updated"`
}

func (h *Handler) assetUpdateStatusLibre(w http.ResponseWriter, uid int, flag types.AssetStatus,
	assetIDs []string, isSet bool) {
	assets, err := h.assetUpdateStatus(uid, flag, assetIDs, isSet)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	reply := &assetUpdateReply{Status: true, NotUpdated: []*assetDetail{}}
	for _, ra := range assets {
		a := h.newAssetDetail(ra)
		reply.Updated = append(reply.Updated, a)
		reply.Results = append(reply.Results, a)
	}
	common.WriteBody(w, reply)
}

func (h *Handler) assetHide(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	ad := &types.AssetsEditHidden{}
	if err := json.NewDecoder(r.Body).Decode(ad); err != nil {
		common.WriteError(w, err)
		return
	}

	h.assetUpdateStatusLibre(w, wl.Userid, types.AssetStatusHidden, ad.AssetIDs, ad.Hidden)
}

func (h *Handler) assetFavorite(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	af := &types.AssetsEditFavorite{}
	if err := json.NewDecoder(r.Body).Decode(af); err != nil {
		common.WriteError(w, err)
		return
	}

	h.assetUpdateStatusLibre(w, wl.Userid, types.AssetStatusFavorite, af.AssetIDs, af.Favorite)
}

func (h *Handler) assetPublic(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	ap := &types.AssetsEditPublic{}
	if err := json.NewDecoder(r.Body).Decode(ap); err != nil {
		common.WriteError(w, err)
		return
	}

	h.assetUpdateStatusLibre(w, wl.Userid, types.AssetStatusPublic, ap.AssetIDs, ap.Public)
}

type Place struct {
	Attributes []string `json:"attributes"`
	Categories []string `json:"categories"`
}

type aiText struct {
	ImgText string `json:"im2txt"`
	Place   Place `json:"places365"`
}

type aiData struct {
	ExifTimestamp string `json:"exif_timestamp"`
	ImageHash string `json:"image_hash"`
	SearchLocation string `json:"search_location"`
	SearchCaption string `json:"search_captions"`   // multiple captions separated by " , "
}

type assetDetail struct {
	Hidden bool `json:"hidden"`
	Public bool `json:"public"`
	Video bool `json:"video"`
	Rating int `json:"rating"`
	ImageExtension string `json:"image_ext"`
	ImageHash string `json:"image_hash"`
	ImagePath string `json:"image_path"`
	GpsLat float64 `json:"exif_gps_lat"`
	GpsLon float64 `json:"exif_gps_lon"`
	ImageURL string `json:"image_url"`
	ThumbnailWidth int `json:"thumbnail_width"`
	ThumbnailHeight int `json:"thumbnail_height"`
	Thumbnail string `json:"square_thumbnail"`
	ThumbnailURL string `json:"thumbnail_url"`
	ThumbnailURLSmall string `json:"small_thumbnail_url"`
	ThumbnailURLBig string `json:"big_thumbnail_url"`
	ThumbnailURLSquare string `json:"square_thumbnail_url"`
	ThumbnailURLSquareTiny string `json:"tiny_square_thumbnail_url"`
	ThumbnailURLSquareSmall string `json:"small_square_thumbnail_url"`
	ThumbnailURLSquareBig string `json:"big_square_thumbnail_url"`
	People []album.AlbumLibre `json:"people"`
	Caption aiText `json:"captions_json"`
	SimilarPhotos []string `json:"similar_photos"`  // hash slice
	SharedTo []string `json:"shared_to"`
	ExifTimestamp string `json:"exif_timestamp"`
	SearchLocation string `json:"search_location"`
	SearchCaption string `json:"search_captions"`   // multiple captions separated by " , "
	XXXX aiData
}

func (h *Handler) newAssetDetail(a *types.Asset) *assetDetail {
	parts := strings.Split(a.Name, ".")
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "select latitude, longitude from asset where id=" + parts[0]).Scan(&a.Latitude, &a.Longitude)
	})
	if err != nil {
		logrus.Warnf("read %s from DB: %v", a.Name, err)
	}
	d := &assetDetail{
		ImageHash: a.Name,
		ImagePath:  fmt.Sprintf("%d/%2d/%2d", a.Date.Year(), a.Date.Month(), a.Date.Day()),
		GpsLat: a.Latitude,
		GpsLon: a.Longitude,
		People: []album.AlbumLibre{},
		SimilarPhotos: []string{},
		SharedTo: []string{},
		Caption:        aiText{
			Place: Place{
				Attributes: []string{},
				Categories: []string{},
			},
		},
		ExifTimestamp:  a.Date.String(),
		//SearchLocation: "SJC",
		//SearchCaption:  "test1 , zoo",
	}

	if len(parts) > 1 {
		d.ImageExtension = parts[1]
		d.Video = ext.IsLivePhotoExtension(parts[1]) || ext.IsVideoFile(d.ImageExtension)
	}

	if types.IsAssetStatus(a.Status, types.AssetStatusHidden) {
		d.Hidden = true
	}
	if types.IsAssetStatus(a.Status, types.AssetStatusPublic) {
		d.Public = true
	}
	if types.IsAssetStatus(a.Status, types.AssetStatusFavorite) {
		d.Rating = 1
	}
	return d
}

func (h *Handler) getAssetDetail(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	aid := mux.Vars(r)["assetID"]
	if aid == "notimestamp" {
		common.WriteBody(w, &dayPhoto{Results: &[]types.AssetsByDay{}})
		return
	}
	statusMap := map[string]types.AssetStatus {
		"hidden": types.AssetStatusHidden,
		"favorites": types.AssetStatusFavorite,
		"public": types.AssetStatusPublic,
	}
	flag, ok := statusMap[aid]
	if ok {
		h.memdb.Lock()
		assets := h.memdb.ListAllAssetsByStatus(wl.Userid, flag)
		h.memdb.Unlock()
		common.WriteBody(w, &dayPhoto{Results: assets})
		return
	}

	parts := strings.Split(aid, ".")
	id, err := strconv.Atoi(strings.TrimSuffix(parts[0], "_video"))
	if err != nil {
		common.WriteError(w, err)
		return
	}

	h.memdb.Lock()
	asset := h.memdb.ExistByID(wl.Userid, id)
	h.memdb.Unlock()
	if asset == nil {
		common.WriteError(w, common.ErrAssetNotExistForUser)
		return
	}

	common.WriteBody(w, h.newAssetDetail(asset))
}

func (h *Handler) downloadAssets(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	ad := &types.AssetsDownload{}
	if err := json.NewDecoder(r.Body).Decode(ad); err != nil {
		common.WriteError(w, err)
		return
	}

	masterFiles := []string{}
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		for _, id := range ad.AssetIDs {
			aid, err := asset.ParseAssetID(id)
			if err != nil {
				return err
			}
			masterFile, _, err := asset.GetAssetMasterPreviewPath(ctx, tx, wl.Userid, aid, 0, 0,
				nil, h.previewRunner, h.conf.FolderPerm)
			if err != nil {
				return err
			}
			masterFiles = append(masterFiles, masterFile)
		}
		return nil
	})
	if err != nil {
		logrus.Warnf("read master paths %v from DB: %v", ad, err)
		common.WriteError(w, err)
		return
	}

	// use general octect-stream to avoid client decompressing the file
	w.Header().Set("Content-Type", "application/octet-stream")

	zw := zip.NewWriter(w)
	defer zw.Close()

	for _, f := range masterFiles {
		if err := writeZip(f, zw); err != nil {
			logrus.Warnf("compress master file %s: %v", f, err)
		}
	}
}

func writeZip(filename string, zw *zip.Writer) error {
	logrus.Infof("Downloading asset file: %s", filename)
	f, err := os.Open(filename)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	defer f.Close()

	_, base := filepath.Split(filename)
	zf, err := zw.Create(base)
	if err != nil {
		return err
	}

	_, err = io.Copy(zf, f)
	return err
}

type userLiberty struct {
	MinRating int `json:"favorite_min_rating"`
}

func (h *Handler) getUserLibre(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	common.WriteBody(w, &userLiberty{MinRating: 1})
}

type obtainTokenRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type obtainTokenReply struct {
	Refresh string `json:"refresh"`
	Access string `json:"access"`
}

type refreshTokenRequest struct {
	Refresh string `json:"refresh"`
}


func (h *Handler) obtainToken(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	otr := &obtainTokenRequest{}
	if err := json.NewDecoder(r.Body).Decode(otr); err != nil {
		common.WriteError(w, err)
		return
	}

	var (
		token string
		uid int
		reply obtainTokenReply
	)
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// skip error
		logrus.Warnf("split client address %s: %v", r.RemoteAddr, err)
	}
	ua := r.Header.Get("User-Agent")
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		token, uid, err = user.Login(ctx, tx, otr.Username, otr.Password, ip+"("+ua+")", h.tokenDuration)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	reply.Access, err = user.EncodeJWTClaim(uid, otr.Username, token, "access", h.tokenDuration)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	reply.Refresh, err = user.EncodeJWTClaim(uid, otr.Username, token, "refresh", h.tokenDuration)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, reply)
}

func (h *Handler) refreshToken(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	rtr := &refreshTokenRequest{}
	if err := json.NewDecoder(r.Body).Decode(rtr); err != nil {
		common.WriteError(w, err)
		return
	}

	var (
		token string
		reply obtainTokenReply
	)
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// skip error
		logrus.Warnf("split client address %s: %v", r.RemoteAddr, err)
	}
	ua := r.Header.Get("User-Agent")

	claim, err := user.DecodeJWTClaim(rtr.Refresh)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		token, err = user.ValidateAndRenewToken(ctx, tx, claim.UserID,claim.Token,ip+"("+ua+")")
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	reply.Access, err = user.EncodeJWTClaim(claim.UserID, claim.Name, token, "access", h.tokenDuration)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	reply.Refresh, err = user.EncodeJWTClaim(claim.UserID, claim.Name, token, "refresh", h.tokenDuration)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, reply)
}
