package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"github.com/sirupsen/logrus"
)

// maxOrigNamesPerRequest bounds one request's work on a small NAS; clients send more in turns.
const maxOrigNamesPerRequest = 500

// AssetOrigName is one asset's original file name, by the asset's hash.
type AssetOrigName struct {
	Hash string
	Name string
}

// setAssetOrigNames records the names files had on the uploading device, for assets uploaded
// before clients sent the name with the upload. A name already stored is kept. The reply lists
// the name each of the given assets now has (possibly one recorded earlier); assets that don't
// exist are left out.
func (h *Handler) setAssetOrigNames(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	req := []AssetOrigName{}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		logrus.Warnf("invalid original names request: %v", err)
		common.WriteError(w, common.ErrBadRequest, "expected a JSON list of {Hash, Name}")
		return
	}
	if len(req) > maxOrigNamesPerRequest {
		common.WriteError(w, common.ErrBadRequest, "too many names in one request")
		return
	}

	names := make(map[string]string, len(req))
	for _, n := range req {
		hash := strings.ToLower(strings.TrimSpace(n.Hash))
		if hash == "" {
			continue
		}
		names[hash] = asset.CleanOrigName(n.Name)
	}

	var stored map[string]string
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		stored, err = asset.SetOrigNames(ctx, tx, wl.Userid, names)
		return err
	}); err != nil {
		common.WriteError(w, err, "set original names")
		return
	}

	reply := make([]AssetOrigName, 0, len(stored))
	if h.conf.UseMemdb {
		h.memdb.Lock()
	}
	for hash, name := range stored {
		if h.conf.UseMemdb && !h.memdb.SetOrigName(wl.Userid, hash, name) {
			logrus.Warnf("asset %s not in memdb while setting its original name", hash)
		}
		reply = append(reply, AssetOrigName{Hash: hash, Name: name})
	}
	if h.conf.UseMemdb {
		h.memdb.Unlock()
	}
	common.WriteBody(w, reply)
}
