package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func (h *Handler) listAssetLabels(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	var labels []*types.AssetLabel
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		//return home dir if request is from localhost
		labels, err = asset.ListLabels(ctx, tx)
		return err
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, labels)
}

func (h *Handler) addAssetLabel(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	label := &types.AssetLabel{}
	if err := json.NewDecoder(r.Body).Decode(&label); err != nil {
		common.WriteError(w, err)
		return
	}

	var id int64
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		id, err = asset.CreateLabel(ctx, tx, label)
		return err
	})
	if err != nil {
		common.WriteError(w, err)
		return
	}

	common.WriteBody(w, id)
}

func (h *Handler) listAssetsInLabel(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	labelID, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		common.WriteError(w, err)
		return
	}

	var assets []*types.AssetNameConfidence
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assets, err = asset.ListAssetsInLabel(ctx, tx, wl.Userid, labelID)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, assets)
}

func (h *Handler) addAssetsInLabel(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	labelID, err := strconv.Atoi(mux.Vars(r)["id"])
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
		ids, notExistIDs, err = asset.ValidateAssetIDs(ctx, tx, wl.Userid, aids)
		if err != nil {
			return err
		}
		if len(notExistIDs) != 0 {
			return nil
		}

		return asset.AddAssetsInLabel(ctx, tx, labelID, ids)
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	if len(notExistIDs) != 0 {
		common.WriteError(w, errors.Errorf("assets %v not exist", notExistIDs))
	}
}

func (h *Handler) deleteAssetsFromLabel(w http.ResponseWriter, r *http.Request) {
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
		ids, notExistIDs, err := asset.ValidateAssetIDs(ctx, tx, wl.Userid, aids)
		if err != nil {
			return err
		}
		if len(notExistIDs) != 0 {
			logrus.Warnf("while deleting assets %v from album %d, found not exist IDs: %v", aids, id, notExistIDs)
			return common.ErrBadRequest
		}
		return asset.RemoveLabelForAssets(ctx, tx, id, ids)
	}); err != nil {
		common.WriteError(w, err)
		return
	}
}

func (h *Handler) getAssetLabels(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	id, err := strconv.Atoi(mux.Vars(r)["assetID"])
	if err != nil {
		common.WriteError(w, err)
		return
	}

	var labels []*types.AssetLabelConfidence
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		labels, err = asset.ListLabelsForAsset(ctx, tx, id)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, labels)
}
