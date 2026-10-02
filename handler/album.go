package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/album"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func (h *Handler) listAlbums(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	var albums *album.Albums
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		//return home dir if request is from localhost
		albums, err = album.ListAlbums(ctx, tx, wl.Userid)
		return err
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, albums)
}

func (h *Handler) createAlbum(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	a := album.Album{CreateTime: time.Now().Format(common.TimeFormatLomod)}
	a.LastModifiedTime = a.CreateTime
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		common.WriteError(w, err)
		return
	}
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		id, err := album.CreateAlbum(ctx, tx, wl.Userid, a)
		if err != nil {
			return err
		}
		a.ID = int(id)
		return err
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, a)
}

func (h *Handler) deleteAlbums(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	ids := []int{}
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		common.WriteError(w, err)
		return
	}

	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		for _, id := range ids {
			err := album.DeleteAlbum(ctx, tx, wl.Userid, id)
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		common.WriteError(w, err)
		return
	}
}

func (h *Handler) deleteAlbum(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		common.WriteError(w, err)
		return
	}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		return album.DeleteAlbum(ctx, tx, wl.Userid, id)
	}); err != nil {
		common.WriteError(w, err)
		return
	}
}

func (h *Handler) updateAlbum(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	a := album.Album{}
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		common.WriteError(w, err)
		return
	}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		return album.UpdateAlbum(ctx, tx, wl.Userid, a)
	}); err != nil {
		common.WriteError(w, err)
		return
	}
}

type mergeAlbumRequest struct {
	Title    string `json:"Title"`
	AlbumIDs []int  `json:"AlbumIDs"`
}

func (h *Handler) mergeAlbum(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	req := &mergeAlbumRequest{}
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		common.WriteError(w, err)
		return
	}

	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		return album.MergeAlbum(ctx, tx, req.Title, wl.Userid, req.AlbumIDs)
	}); err != nil {
		common.WriteError(w, err)
		return
	}
}

func (h *Handler) listAlbumAssetsSummary(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		common.WriteError(w, err)
		return
	}
	total := 0
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		total, err = album.GetTotalAssets(ctx, tx, id)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	w.Header().Add(totalCountHeader, strconv.Itoa(total))
}

func (h *Handler) listAlbumAssets(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		common.WriteError(w, err)
		return
	}
	retHash := r.URL.Query()[common.QueryKeyRetHash] != nil
	page := 0
	if len(r.URL.Query()["page"]) > 0 && len(r.URL.Query()["page"][0]) > 0 {
		var err error
		page, err = strconv.Atoi(r.URL.Query()["page"][0])
		if err != nil {
			common.WriteError(w, err)
			return
		}
	}

	var aids interface{}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		if retHash {
			aids, err = album.ListAssets(ctx, tx, id, page, defaultLimit)
		} else {
			aids, err = album.ListAssetIDs(ctx, tx, id, page, defaultLimit)
		}
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, aids)
}

func (h *Handler) addAssetsInAlbum(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	albumID, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		common.WriteError(w, err)
		return
	}

	aids := []string{}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&aids); err != nil {
		common.WriteError(w, err)
		return
	}

	var (
		ids         []int
		notExistIDs []string
	)
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		if err := album.ValidateAlbumUser(ctx, tx, wl.Userid, albumID); err != nil {
			return err
		}
		ids, notExistIDs, err = asset.ValidateAssetIDs(ctx, tx, wl.Userid, aids)
		if err != nil {
			return err
		}
		if len(notExistIDs) != 0 {
			return nil
		}
		idFilenameMap := map[int]string{}
		for _, id := range ids {
			idFilenameMap[id] = ""
		}
		return album.AddAssets(ctx, tx, albumID, idFilenameMap)
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	if len(notExistIDs) != 0 {
		common.WriteError(w, errors.Errorf("assets %v not exist", notExistIDs))
	}
}

func (h *Handler) deleteAssetsFromAlbum(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		common.WriteError(w, err)
		return
	}

	aids := []string{}
	if err := json.NewDecoder(r.Body).Decode(&aids); err != nil {
		common.WriteError(w, err)
		return
	}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		if err := album.ValidateAlbumUser(ctx, tx, wl.Userid, id); err != nil {
			return err
		}
		ids, notExistIDs, err := asset.ValidateAssetIDs(ctx, tx, wl.Userid, aids)
		if err != nil {
			return err
		}
		if len(notExistIDs) != 0 {
			logrus.Warnf("while deleting assets %v from album %d, found not exist IDs: %v", aids, id, notExistIDs)
			return common.ErrBadRequest
		}
		return album.DeleteAssets(ctx, tx, id, ids)
	}); err != nil {
		common.WriteError(w, err)
		return
	}
}

func (h *Handler) getAssetAlbums(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	a := album.Albums{}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		a.Albums, err = album.ListAlbumsByAssetID(ctx, tx, mux.Vars(r)["assetID"])
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, a)
}
