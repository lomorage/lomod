package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
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

// origNamesPage bounds one reply of listAssetOrigNames.
const origNamesPage = 1000

// AssetOrigNamesSince is a page of names recorded after the upload.
type AssetOrigNamesSince struct {
	// Rev is what to pass as since for the next page: the last one listed, or since itself.
	Rev   int64
	Names []AssetOrigName
	// More is true when there may be names after this page.
	More bool
}

// listAssetOrigNames lists names recorded after the upload (POST /assets/names) with a revision
// above ?since=, oldest first. Those don't change the tree hashes, so a device that already has
// a day's listing learns of them only here.
func (h *Handler) listAssetOrigNames(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	var since int64
	if s := r.URL.Query().Get("since"); s != "" {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil || v < 0 {
			common.WriteError(w, common.ErrBadRequest, "since must be a revision number")
			return
		}
		since = v
	}

	var changes []asset.OrigNameChange
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		changes, err = asset.ListOrigNamesSince(ctx, tx, wl.Userid, since, origNamesPage)
		return err
	}); err != nil {
		common.WriteError(w, err, "list original names")
		return
	}

	reply := AssetOrigNamesSince{Rev: since, Names: make([]AssetOrigName, 0, len(changes)),
		More: len(changes) == origNamesPage}
	for _, c := range changes {
		reply.Names = append(reply.Names, AssetOrigName{Hash: c.Hash, Name: c.Name})
		reply.Rev = c.Rev
	}
	common.WriteBody(w, reply)
}
