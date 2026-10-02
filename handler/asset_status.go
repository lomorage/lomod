package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/sirupsen/logrus"
)

func (h *Handler) assetUpdateStatus(uid int, flag types.AssetStatus, assetIDs []string,
	isSet bool) ([]*types.Asset, error) {
	var newStatuses map[string]asset.SetAssetStatusReply
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		newStatuses, err = asset.UpdateAssetsStatus(ctx, tx, uid, flag, assetIDs, isSet)
		return err
	}); err != nil {
		return nil, err
	}

	assets := []*types.Asset{}
	h.memdb.Lock()
	for aid, newStatus := range newStatuses {
		a, err := h.memdb.UpdateStatus(uid, newStatus.AssetCreateDate.Year(), int(newStatus.AssetCreateDate.Month()),
			newStatus.AssetCreateDate.Day(), aid, newStatus.Status)
		if err != nil {
			logrus.Warnf("failed to update asset %s status: %v", aid, err)
			continue
		}
		assets = append(assets, a)
	}
	h.memdb.Unlock()
	return assets, nil
}

func (h *Handler) replyAssets(w http.ResponseWriter, assets []*types.Asset) {
	reply := []types.AssetName{}
	for _, a := range assets {
		reply = append(reply, types.AssetName{Name: a.Name, Hash: a.Hash})
	}
	common.WriteBody(w, reply)
}

func (h *Handler) hideAssets(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	ids := []string{}
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		common.WriteError(w, err)
		return
	}

	assets, err := h.assetUpdateStatus(wl.Userid, types.AssetStatusHidden, ids, true)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	h.replyAssets(w, assets)
}

func (h *Handler) unhideAssets(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	ids := []string{}
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		common.WriteError(w, err)
		return
	}

	assets, err := h.assetUpdateStatus(wl.Userid, types.AssetStatusHidden, ids, false)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	h.replyAssets(w, assets)
}

func (h *Handler) setAssetsFavorite(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	ids := []string{}
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		common.WriteError(w, err)
		return
	}

	assets, err := h.assetUpdateStatus(wl.Userid, types.AssetStatusFavorite, ids, true)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	h.replyAssets(w, assets)
}

func (h *Handler) unsetAssetsFavorite(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	ids := []string{}
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		common.WriteError(w, err)
		return
	}

	assets, err := h.assetUpdateStatus(wl.Userid, types.AssetStatusFavorite, ids, false)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	h.replyAssets(w, assets)
}
