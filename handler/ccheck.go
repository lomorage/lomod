package handler

import (
	"net/http"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/user"
)

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

	if err := h.ccheckRunner.Start(users, assetsDB); err != nil {
		h.checkLogger.Warnf("while scanning file system, got %v", err)
		return err
	}

	h.ccheckRunner.Report(h.checkLogger, false)
	return nil
}

func (h *Handler) getCCheckResult(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	h.ccheckRunner.Report(w, r.URL.Query().Get(common.QueryKeyPlainOutput) == "1")
}
