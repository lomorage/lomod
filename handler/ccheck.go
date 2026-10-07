package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/check"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/user"
)

// maxVerifyHashes bounds one /assets/verify request; each hash costs a DB lookup and a stat.
const maxVerifyHashes = 500

func (h *Handler) startCCheck(w http.ResponseWriter, r *http.Request) {
	h.ccheckCh <- struct{}{}
}

func (h *Handler) check(users []user.User) error {
	assetsDB := h.memdb.GetAssetsList()
	defer func() {
		assetsDB = nil
	}()

	uids := make(map[int]string)
	for _, user := range users {
		uids[user.ID] = user.Name
	}

	// Record the run before scanning: if lomod dies mid-check the run stays incomplete and
	// is never used as evidence by /assets/verify.
	var runID int64
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		runID, err = check.BeginRun(ctx, tx, time.Now(), check.MaxAssetID(assetsDB))
		return err
	}); err != nil {
		h.checkLogger.Warnf("while recording consistency check start, got %v", err)
		runID = 0
	}

	if err := h.ccheckRunner.Start(users, assetsDB); err != nil {
		h.checkLogger.Warnf("while scanning file system, got %v", err)
		return err
	}

	if runID > 0 {
		if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
			return check.FinishRun(ctx, tx, runID, time.Now(), h.ccheckRunner.Unverified)
		}); err != nil {
			h.checkLogger.Warnf("while recording consistency check result, got %v", err)
		}
	}

	h.ccheckRunner.Report(h.checkLogger, false)
	return nil
}

type verifyAssetsRequest struct {
	Hashes []string `json:"hashes"`
}

type verifyAssetsResponse struct {
	LastCheck *verifyAssetsRun    `json:"lastCheck"`
	Assets    []check.AssetVerify `json:"assets"`
}

type verifyAssetsRun struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// verifyAssets tells a client which of its uploaded hashes are safely stored, so it can decide
// whether deleting the local original is safe. Read only; see check.VerifyAssets for the rules.
func (h *Handler) verifyAssets(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	req := verifyAssetsRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, common.ErrBadRequest)
		return
	}
	if len(req.Hashes) > maxVerifyHashes {
		common.WriteError(w, common.ErrBadRequest)
		return
	}

	resp := verifyAssetsResponse{Assets: []check.AssetVerify{}}

	h.userMountLock.Lock()
	mi, ok := h.userMountStatus[wl.Username]
	h.userMountLock.Unlock()
	if ok && mi.homeErr != nil {
		for _, hash := range req.Hashes {
			resp.Assets = append(resp.Assets, check.AssetVerify{Hash: hash, Status: check.VerifyUnavailable})
		}
		common.WriteBody(w, resp)
		return
	}

	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		assets, run, err := check.VerifyAssets(ctx, tx, wl.Userid, req.Hashes)
		if err != nil {
			return err
		}
		resp.Assets = assets
		if run != nil {
			resp.LastCheck = &verifyAssetsRun{Start: run.StartTime, End: run.EndTime}
		}
		return nil
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, resp)
}

func (h *Handler) getCCheckResult(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	h.ccheckRunner.Report(w, r.URL.Query().Get(common.QueryKeyPlainOutput) == "1")
}
