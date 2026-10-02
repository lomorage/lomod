package handler

import (
	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/album"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/geo"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/scene"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"net/http"
	"strconv"
	"time"
)

type libreAlbums struct {
	Count int `json:"count"`
	Results []libreAlbumBrief `json:"results"`
}

type libreAlbumCover struct {
	ImageHash string `json:"image_hash"`
	Video bool `json:"video"`
}

type libreAlbumOwner struct {
	ID int `json:"id"`
	Username string `json:"username"`
	FirstName string `json:"first_name"`
	LastName string `json:"last_name"`
}

type libreAlbumBrief struct {
	ID int `json:"id"`
	Title string `json:"title"`
	CreateTime string `json:"created_on"`
	Favorite bool `json:"favorited"`
	CoverPhotos []libreAlbumCover `json:"cover_photos"`
	SharedTo []int `json:"shared_to"`
	Owner libreAlbumOwner `json:"owner"`
	PhotoCount int `json:"photo_count"`
	GeoLevel int `json:"geolocation_level"`
}

type libreAlbumPhoto struct {
	ID string `json:"id"`
	Color string `json:"dominantColor"`
	URL string `json:"url"`
	Location string `json:"location"`
	CreateTime string `json:"date"`
	BirthTime string `json:"birthTime"`
	AspectRatio float32 `json:"aspectRatio"`
	Type string `json:"type"`
	Rating int `json:"rating"`
	SharedTo []int `json:"shared_to"`
	Owner libreAlbumOwner `json:"owner"`
}

type libreAlbumPhotoGroup struct {
	CreateTime string `json:"date"`
	Location string `json:"location"`
	Items []libreAlbumPhoto `json:"items"`
}

type libreAlbumDetail struct {
	ID string `json:"id"`
	Title string `json:"title"`
	CreateTime string `json:"date"`
	SharedTo []int `json:"shared_to"`
	Owner libreAlbumOwner `json:"owner"`
	Location string `json:"location"`
	GroupedPhotos []libreAlbumPhotoGroup `json:"grouped_photos"`
}

type libreAlbumDetails struct {
	Results libreAlbumDetail `json:"results"`
}

type libreAlbumEditRequest struct {
	Title string `json:"title"`
	AddPhotos []string `json:"photos"`
	RemovePhotos []string `json:"removedPhotos"`
}

type libreAlbumEditReply struct {
	ID int `json:"id"`
	Title string `json:"title"`
	CreateTime time.Time `json:"created_on"`
	Favorite bool `json:"favorited"`
	Photos []string `json:"photos"`
}

func (h *Handler) createAlbumLibre(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	req := &libreAlbumEditRequest{}
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		common.WriteError(w, err)
		return
	}
	reply := &libreAlbumEditReply{Title: req.Title, CreateTime: time.Now(), Photos: req.AddPhotos}
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		id, err := album.CreateAlbum(ctx, tx, wl.Userid, album.Album{Title: req.Title, Author: wl.Username})
		reply.ID = int(id)

		ids, notExistIDs, err := asset.ValidateAssetIDs(ctx, tx, wl.Userid, req.AddPhotos)
		if err != nil {
			return err
		}
		if len(notExistIDs) != 0 {
			logrus.Warnf("assets %v is not exist for user %s", notExistIDs, wl.Username)
		}
		idFilenameMap := map[int]string{}
		for _, id := range ids {
			idFilenameMap[id] = ""
		}
		return album.AddAssets(ctx, tx, reply.ID, idFilenameMap)
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, reply)
}

func (h *Handler) editAlbumLibre(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	aid := mux.Vars(r)["id"]
	id, err := strconv.Atoi(aid)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	req := &libreAlbumEditRequest{}
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		common.WriteError(w, err)
		return
	}

	if len(req.AddPhotos) != 0 {
		h.editAlbumAddLibre(w, wl.Userid, id, req)
	} else if len(req.RemovePhotos) != 0 {
		h.editAlbumRemoveLibre(w, wl.Userid, id, req)
	} else {
		common.WriteError(w, errors.New("no photos in edit album request"))
	}
	return
}

func (h *Handler) editAlbumAddLibre(w http.ResponseWriter, uid, id int, req *libreAlbumEditRequest) {
	reply := &libreAlbumEditReply{ID: id, Title: req.Title, CreateTime: time.Now(), Photos: req.AddPhotos}
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		ids, notExistIDs, err := asset.SplitAssetIDs(ctx, tx, id, req.AddPhotos)
		if err != nil {
			return err
		}
		if len(notExistIDs) == 0 {
			logrus.Warnf("not insert because assets %v is existed for album %d", ids, id)
			return nil
		}
		if len(ids) != 0 {
			logrus.Warnf("assets %v is existed for album %d", ids, id)
		}
		ids, notExistIDs, err = asset.ValidateAssetIDs(ctx, tx, uid, notExistIDs)
		if err != nil {
			return err
		}
		if len(notExistIDs) != 0 {
			logrus.Warnf("assets %v is not exist for user %d", notExistIDs, uid)
		}

		idFilenameMap := map[int]string{}
		for _, id := range ids {
			idFilenameMap[id] = ""
		}
		return album.AddAssets(ctx, tx, reply.ID, idFilenameMap)
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, reply)
}

func (h *Handler) editAlbumRemoveLibre(w http.ResponseWriter, uid, id int, req *libreAlbumEditRequest) {
	reply := &libreAlbumEditReply{ID: id, Title: req.Title, CreateTime: time.Now(), Photos: req.RemovePhotos}
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		ids, notExistIDs, err := asset.SplitAssetIDs(ctx, tx, id, req.RemovePhotos)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			logrus.Warnf("not delete because assets %v is not existed for album %d", req.RemovePhotos, id)
			return nil
		}
		if len(notExistIDs) != 0 {
			logrus.Warnf("assets %v is not existed for album %d", notExistIDs, id)
		}

		return album.DeleteAssets(ctx, tx, reply.ID, ids)
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, reply)
}

func (h *Handler) listAlbumUsers(w http.ResponseWriter, uid int, uname string) {
	reply := &libreAlbums{}
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		albums, err := album.ListAlbums(ctx, tx, uid)
		if err != nil {
			return err
		}

		for _, a := range albums.Albums {
			count, err := album.GetTotalAssets(ctx, tx, a.ID)
			if err != nil {
				return err
			}
			if count == 0 {
				continue
			}
			assetID, err := album.GetCoverPhoto(ctx, tx, a.ID)
			if err != nil {
				return err
			}
			ab :=  libreAlbumBrief{
				ID: a.ID, Title: a.Title, CreateTime: a.CreateTime, SharedTo: []int{}, PhotoCount: count,
				CoverPhotos: []libreAlbumCover{{ImageHash: strconv.Itoa(assetID) + ".webp"}},
				Owner: libreAlbumOwner{ID: uid, Username:  uname},
			}
			reply.Results = append(reply.Results, ab)
		}
		return nil
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}

	reply.Count = len(reply.Results)
	common.WriteBody(w, reply)
	return
}

func (h *Handler) listAlbumUser(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	aid := mux.Vars(r)["id"]
	if aid == "list" {
		h.listAlbumUsers(w, wl.Userid, wl.Username)
		return
	}
	id, err := strconv.Atoi(aid)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	var names []string
	reply := &libreAlbumDetail{ID: aid, Owner: libreAlbumOwner{ID: wl.Userid, Username: wl.Username}, SharedTo: []int{}}
	err = dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		al, err := album.GetAlbum(ctx, tx, id)
		if err != nil {
			return err
		}
		reply.CreateTime = al.CreateTime
		reply.Title = al.Title

		names, err = album.ListAssetNames(ctx, tx, id, 0, 500)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}

	h.memdb.Lock()
	assetGroups := h.memdb.GetAssetHashGroupByIDs(wl.Userid, names)
	h.memdb.Unlock()

	for _, ag := range *assetGroups {
		group := libreAlbumPhotoGroup{CreateTime: ag.CreateDate, Location: ag.Location}
		for _, a := range ag.Assets {
			group.Items = append(group.Items, libreAlbumPhoto{
				ID: a.ID,
				URL: a.URL,
				CreateTime: ag.CreateDate,
				BirthTime: ag.CreateDate,
				AspectRatio: a.AspectRatio,
				Type: a.Type,
				SharedTo: []int{},
				Owner: libreAlbumOwner{ID: wl.Userid, Username: wl.Username},
			})
		}
		reply.GroupedPhotos = append(reply.GroupedPhotos, group)
	}
	common.WriteBody(w, reply)
}

func (h *Handler) listAlbumThings(w http.ResponseWriter, userID int) {
	albums := libreAlbums{}
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		things, err := scene.GetScenesByUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		for _, p := range things {
			names, err := scene.GetAssetsInScene(ctx, tx, userID, p.ID, 1)
			if err != nil {
				return err
			}
			if len(names) == 0 {
				logrus.Warnf("no assets found at scene %+v", p)
				continue
			}
			albums.Count++
			albums.Results = append(albums.Results, libreAlbumBrief{
				ID: p.ID,
				Title: p.Label,
				PhotoCount: p.AssetCount,
				CoverPhotos: []libreAlbumCover{{ImageHash: names[0]}},
			})
		}
		return nil
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, albums)
}

func (h *Handler) listAlbumThing(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	aid := mux.Vars(r)["id"]
	if aid == "list" {
		h.listAlbumThings(w, wl.Userid)
		return
	}
	id, err := strconv.Atoi(aid)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	var names []string
	reply := &libreAlbumDetails{
		Results: libreAlbumDetail{
			ID: aid, Owner: libreAlbumOwner{ID: wl.Userid, Username: wl.Username}, SharedTo: []int{},
		},
	}
	err = dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		reply.Results.Title, err = scene.GetScene(ctx, tx, id)
		if err != nil {
			return err
		}

		names, err = scene.ListAssetNames(ctx, tx, wl.Userid, id, 0, 1000)
		return err
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}

	h.memdb.Lock()
	assetGroups := h.memdb.GetAssetHashGroupByIDs(wl.Userid, names)
	h.memdb.Unlock()

	for _, ag := range *assetGroups {
		group := libreAlbumPhotoGroup{CreateTime: ag.CreateDate, Location: ag.Location}
		for _, a := range ag.Assets {
			group.Items = append(group.Items, libreAlbumPhoto{
				ID: a.ID,
				URL: a.URL,
				CreateTime: ag.CreateDate,
				BirthTime: ag.CreateDate,
				AspectRatio: a.AspectRatio,
				Type: a.Type,
				SharedTo: []int{},
				Owner: libreAlbumOwner{ID: wl.Userid, Username: wl.Username},
			})
		}
		reply.Results.GroupedPhotos = append(reply.Results.GroupedPhotos, group)
	}
	common.WriteBody(w, reply)
}

func (h *Handler) listAlbumPlaces(w http.ResponseWriter, userID int) {
	albums := libreAlbums{}
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		places, err := geo.GetGeoLocationsByUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		albums.Count = len(places)
		for _, p := range places {
			// skip the place without coordinate yet
			if p.Longitude == 0.0 || p.Latitude == 0.0 {
				continue
			}
			count, err := geo.GetAssetsCountInPlace(ctx, tx, userID, p.ID)
			if err != nil {
				return err
			}
			names, err := geo.GetAssetsInPlace(ctx, tx, userID, p.ID, 1)
			if err != nil {
				return err
			}
			if len(names) == 0 {
				logrus.Warnf("no assets found at place %+v", p)
				continue
			}
			albums.Results = append(albums.Results, libreAlbumBrief{
				ID: p.ID,
				Title: p.NameByLevel(),
				PhotoCount: count,
				GeoLevel: int(p.Level),
				CoverPhotos: []libreAlbumCover{{ImageHash: names[0]}},
			})
		}
		return nil
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, albums)
}

func (h *Handler) listAlbumPlace(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	aid := mux.Vars(r)["id"]
	if aid == "list" {
		h.listAlbumPlaces(w, wl.Userid)
		return
	}
	id, err := strconv.Atoi(aid)
	if err != nil {
		common.WriteError(w, err)
		return
	}

	var names []string
	reply := &libreAlbumDetails{
		Results: libreAlbumDetail{
			ID: aid, Owner: libreAlbumOwner{ID: wl.Userid, Username: wl.Username}, SharedTo: []int{},
		},
	}
	err = dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		place, err := geo.GetGeoLocation(ctx, tx, id)
		if err != nil {
			return err
		}
		reply.Results.Title = place.NameByLevel()

		names, err = geo.ListAssetNames(ctx, tx, wl.Userid, id, 0, 1000)
		return err
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}

	h.memdb.Lock()
	assetGroups := h.memdb.GetAssetHashGroupByIDs(wl.Userid, names)
	h.memdb.Unlock()

	for _, ag := range *assetGroups {
		group := libreAlbumPhotoGroup{CreateTime: ag.CreateDate, Location: ag.Location}
		for _, a := range ag.Assets {
			group.Items = append(group.Items, libreAlbumPhoto{
				ID: a.ID,
				URL: a.URL,
				CreateTime: ag.CreateDate,
				BirthTime: ag.CreateDate,
				AspectRatio: a.AspectRatio,
				Type: a.Type,
				SharedTo: []int{},
				Owner: libreAlbumOwner{ID: wl.Userid, Username: wl.Username},
			})
		}
		reply.Results.GroupedPhotos = append(reply.Results.GroupedPhotos, group)
	}
	common.WriteBody(w, reply)
}

func (h *Handler) listAlbumPlaceClusters(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	locations := [][]interface{}{}
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		places, err := geo.GetGeoLocationsByUser(ctx, tx, wl.Userid)
		if err != nil {
			return err
		}

		for _, place := range places {
			// skip the place without coordinate yet
			if place.Longitude == 0.0 || place.Latitude == 0.0 {
				continue
			}
			// orders: latitude, longitude, place name
			locations = append(locations, []interface{}{place.Latitude, place.Longitude, place.NameByLevel()})
		}
		return nil
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, locations)
}

func (h *Handler) searchAssetsInAlbums(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	common.WriteBody(w, []album.AlbumPhoto{{
				ImageURL: "/asset/1.jpg",ExifTimestamp:  "2021/01/01", ImageHash: "575db2e474109f982ead09e7f8676680a679c9c0",
				ThumbnailURL: "/asset/preview/1.jpg", ThumbnailWidth: 75, ThumbnailHeight: 75}})
}