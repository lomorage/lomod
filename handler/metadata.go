package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/exif"
	"bitbucket.org/lomoware/lomo-backend/common/geo"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/scene"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const totalCountHeader = "X-Total-Count"

var regexMetaName = regexp.MustCompile(`^[\w-.]+$`)

type metadataReply struct {
	Categories     *[]string                       `json:"Categories,omitempty"`
	CategoryNames  *map[string][]string            `json:"CategoryNames,omitempty"`
	CategoryValues *map[string]map[string][]string `json:"CategoryValues,omitempty"`
}

func (h *Handler) listAssetMetadataCategories(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	var metas []string
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		metas, err = asset.ListMetadataCategories(ctx, tx, wl.Userid)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, metadataReply{Categories: &metas})
}

func (h *Handler) listAssetMetadataNames(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	var names []string
	category := mux.Vars(r)["category"]
	_, ok := types.AllMetadataCategories[category]
	if !ok {
		common.WriteError(w, common.ErrNotImplementedFormat)
		return
	}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		names, err = asset.ListMetadataNames(ctx, tx, wl.Userid, category)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, metadataReply{
		CategoryNames: &map[string][]string{category: names},
	})
}

func (h *Handler) listAssetMetadataValues(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	var values []string
	category := mux.Vars(r)["category"]
	_, ok := types.AllMetadataCategories[category]
	if !ok {
		common.WriteError(w, common.ErrNotImplementedFormat)
		return
	}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		values, err = asset.ListMetadataValues(ctx, tx, wl.Userid, category, mux.Vars(r)["name"])
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, metadataReply{
		CategoryValues: &map[string]map[string][]string{
			category: {mux.Vars(r)["name"]: values}},
	})
}

func (h *Handler) getAssetMetadatas(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)

	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	id := mux.Vars(r)["assetID"]
	typ := types.Index
	if len(id) == 40 {
		typ = types.Hash
	}

	var a *types.Asset
	var assetID int
	var masterFile string
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		// get asset info
		var err error
		assetID, _, err = asset.NormalizeAssetID(ctx, tx, wl.Userid, id, typ)
		if err != nil {
			return err
		}
		a, masterFile, err = getAssetWithGeo(ctx, tx, wl.Userid, assetID)
		return err
	}); err != nil {
		if common.IsErrNoRows(err) {
			common.WriteError(w, common.ErrNotExistAsset)
		} else {
			common.WriteError(w, err)
		}
		return
	}

	if masterFile != "" {
		h.probeAndUpdateGeo(wl.Userid, assetID, a, masterFile)
	}
	common.WriteBody(w, a)
}

// getAssetWithGeo returns the asset, plus its master file path if GPS still
// needs to be probed from EXIF (masterFile is "" when no probing is needed).
// Read-only and cheap -- safe to run inside the caller's transaction.
func getAssetWithGeo(ctx context.Context, tx *sql.Tx, userID, assetID int) (a *types.Asset, masterFile string, err error) {
	a, err = asset.GetAssetByID(ctx, tx, userID, assetID)
	if err != nil || a.Longitude != 0.0 {
		return a, "", err
	}
	masterFile, _, err = asset.GetAssetMasterPreviewPath(ctx, tx, userID, assetID, 0, 0, nil, nil, 0)
	return a, masterFile, err
}

// probeAndUpdateGeo reads GPS from EXIF and persists it, deliberately outside
// of any DB transaction: exiftool is an external subprocess that can be slow
// under load (more so now that it runs at reduced OS priority), and holding
// a SQLite transaction open across it blocks every other DB reader/writer
// for as long as it takes -- observed in production as multi-minute stalls
// on unrelated /asset/metadata requests during a large backlog.
func (h *Handler) probeAndUpdateGeo(userID, assetID int, a *types.Asset, masterFile string) {
	exifTags, err := exif.NewTags(masterFile, h.exiftool, "")
	if err != nil {
		logrus.Warnf("Unable to read %s's exif: %v", a.Name, err)
		return
	}
	a.Latitude = exifTags.GetLatitude()
	a.Longitude = exifTags.GetLongitude()
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		return asset.UpdateGeo(ctx, tx, userID, assetID, a.Latitude, a.Longitude)
	}); err != nil {
		logrus.Warnf("Unable to update %s's geo: %v", a.Name, err)
	}
}

func (h *Handler) validateAssetMetadatas(userid int, metas []types.Metadata) error {
	// FIXME: is memdb and sqlite db 100% sync?
	h.memdb.Lock()
	defer h.memdb.Unlock()
	for _, meta := range metas {
		if h.memdb.ExistByID(userid, meta.AssetID) == nil {
			return errors.Errorf("%d not exist for the user", meta.AssetID)
		}
		if !regexMetaName.MatchString(meta.Name) {
			return errors.Errorf("invalid metadata name %s", meta.Name)
		}
		if !meta.Validate() {
			return errors.Errorf("%d has invalid category or source device", meta.AssetID)
		}
	}
	return nil
}

func (h *Handler) addAssetMetadatas(w http.ResponseWriter, r *http.Request) {
	h.CORS(w, r)
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	metas := []types.Metadata{}
	if err := json.NewDecoder(r.Body).Decode(&metas); err != nil {
		common.WriteError(w, err)
		return
	}
	if err := h.validateAssetMetadatas(wl.Userid, metas); err != nil {
		common.WriteError(w, err)
		return
	}
	place, placeLangs, err := h.parsePlaceFromMetadatas(metas)
	if err != nil {
		common.WriteError(w, err)
		return
	}
	force := r.URL.Query()[common.QueryKeyForce]
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		// TODO: one metadata update contains multiple assets
		assetID := 0
		for _, meta := range metas {
			if meta.Value == "" && force == nil {
				logrus.Infof("got empty metadata value: %v", meta)
				continue
			}
			if assetID == 0 {
				assetID = meta.AssetID
			}
			if err := asset.InsertOrUpdateMetadata(ctx, tx, meta); err != nil {
				return err
			}
		}
		if place != nil {
			if err := h.processMetadatasPlace(ctx, tx, assetID, place, placeLangs); err != nil {
				return err
			}
		}
		sceneLabel, sceneProb := h.parseSceneFromMetadatas(metas)
		if sceneLabel == "" || sceneProb == 0.0 {
			return nil
		}
		return h.processMetadatasScene(ctx, tx, assetID, sceneLabel, sceneProb)
	}); err != nil {
		common.WriteError(w, err)
	}
}

func (h *Handler) processMetadatasPlace(ctx context.Context, tx *sql.Tx, assetID int,
	place *types.GeoLocation, placeLangs map[string]types.MetadataForeignLang) error {
	for name, placeLang := range placeLangs {
		if placeLang.Lang == "" {
			continue
		}
		// if english name is not exist, try to load from DB, if still not found, return failure
		if placeLang.NameEn == "" {
			metas, err := asset.GetAssetMetadataByCategory(ctx, tx, assetID, types.MetadataCategoryGeo)
			if err != nil {
				return err
			}
			for _, meta := range metas {
				if !strings.Contains(meta.Name, name) {
					continue
				}
				_, _, _, lang, err := types.ParseMetadataName(meta.Name)
				if err != nil {
					return errors.Errorf("parse %d's metadata %s: %v", meta.AssetID, meta.Name, err)
				}
				if lang != types.MetadataLangEn {
					continue
				}
				placeLang.NameEn = meta.Value
				break
			}
		}
		if placeLang.NameEn == "" {
			return errors.Errorf("metadata %s - %s has not english version yet", name, placeLang.Name)
		}
		err := geo.InsertPlaceLangs(ctx, tx, placeLang)
		if err != nil {
			return err
		}
	}
	ids, err := geo.SelectOrInsertPlace(ctx, tx, *place)
	if err != nil {
		return err
	}
	existPlaceIDs, err := geo.GetPlacesByAssetID(ctx, tx, assetID)
	if err != nil {
		return err
	}
	// check if already in system
	idMap := map[int]struct{}{}
	filteredIDs := []int{}
	for _, id := range ids {
		// skip not exist place
		if id == 0 {
			continue
		}
		idMap[id] = struct{}{}
	}
	for _, id := range existPlaceIDs {
		delete(idMap, id)
		filteredIDs = append(filteredIDs, id)
	}
	if len(filteredIDs) != 0 {
		logrus.Warnf("asset %d has associated with places: %v", assetID, filteredIDs)
	}
	if len(idMap) == 0 {
		return nil
	}
	newIDs := []int{}
	for id := range idMap {
		newIDs = append(newIDs, id)
	}
	return geo.AssociateAssetWithPlaces(ctx, tx, assetID, newIDs)
}

func (h *Handler) parsePlaceFromMetadatas(metas []types.Metadata) (*types.GeoLocation, map[string]types.MetadataForeignLang, error) {
	var place *types.GeoLocation
	placeLangs := map[string]types.MetadataForeignLang{} // 0: en, 1: second lang
	assetID := 0
	for _, meta := range metas {
		if assetID == 0 {
			assetID = meta.AssetID
		}
		if meta.Category != types.MetadataCategoryGeo || meta.Value == "" {
			continue
		}
		if place == nil {
			place = &types.GeoLocation{}
		}
		_, _, name, lang, err := types.ParseMetadataName(meta.Name)
		if err != nil {
			return nil, nil, errors.Errorf("parse %d's metadata %s: %v", meta.AssetID, meta.Name, err)
		}
		placeLang, ok := placeLangs[name]
		if !ok {
			placeLang = types.MetadataForeignLang{}
		}
		if lang != types.MetadataLangEn {
			placeLang.Lang = lang
			placeLang.Name = meta.Value
			placeLangs[name] = placeLang
			continue
		}
		placeLang.NameEn = meta.Value
		placeLangs[name] = placeLang

		if err := place.Set(name, meta.Value); err != nil {
			logrus.Warnf("set asset %d's geo metadata %s: %v", meta.AssetID, meta.Name, err)
			continue
		}
	}

	if place == nil {
		return nil, nil, nil
	}

	// find country, state, city and refill in case it is update request
	if place.Country == "" {
		err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
			var err error
			place.Country, err = geo.FindCountryByAssetID(ctx, tx, assetID)
			return err
		})
		if err != nil {
			if common.IsErrNoRows(err) {
				return place, placeLangs, common.ErrGeoNoCountry
			}
			return nil, nil, err
		}
	}
	if place.State == "" {
		err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
			var err error
			place.State, err = geo.FindStateByAssetID(ctx, tx, assetID)
			return err
		})
		if err != nil {
			if common.IsErrNoRows(err) {
				return place, placeLangs, common.ErrGeoNoState
			}
			return nil, nil, err
		}
	}
	// normalize country and state based on pre-seed database, if not found, insert as is
	csc := geo.CountryState{Country: place.Country, State: place.State}
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		return geo.NormalizeCountryState(ctx, tx, &csc)
	})
	if err != nil && !common.IsErrNoRows(err) {
		return nil, nil, errors.Wrapf(err, "%v", metas)
	}

	err = dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := geo.SelectOrInsertCountryState(ctx, tx, csc)
		return err
	})
	if err != nil {
		return nil, nil, err
	}

	place.Country = csc.Country
	place.State = csc.State

	if len(placeLangs) == 0 {
		return place, nil, nil
	}

	country, ok := placeLangs[types.MetadataGeoCountry]
	if !ok {
		return nil, nil, errors.Errorf("no country in metadata: %v", metas)
	}
	state, ok := placeLangs[types.MetadataGeoState]
	if !ok {
		return nil, nil, errors.Errorf("no state in metadata: %v", metas)
	}

	country.NameEn = csc.Country
	placeLangs[types.MetadataGeoCountry] = country

	state.NameEn = csc.State
	placeLangs[types.MetadataGeoState] = state

	return place, placeLangs, nil
}

func (h *Handler) processMetadatasScene(ctx context.Context, tx *sql.Tx, assetID int, labels string, prob float32) error {
	for _, l := range strings.Split(labels, ",") {
		err := scene.AssociateAssetWithSceneLabel(ctx, tx, assetID, strings.TrimSpace(l), prob)
		if err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) parseSceneFromMetadatas(metas []types.Metadata) (label string, prob float32) {
	for _, m := range metas {
		if m.Category != types.MetadataCategoryScene {
			continue
		}
		if m.Value == "" {
			continue
		}

		if strings.Contains(m.Name, types.MetadataSceneLabel) {
			label = m.Value
			continue
		}
		if strings.Contains(m.Name, types.MetadataSceneProbability) {
			// skip error if parsing failure because probability will be 0
			p, _ := strconv.ParseFloat(m.Value, 32)
			prob = float32(p)
		}
	}
	return
}

func (h *Handler) getAssetsForMetadata(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	var (
		total    int
		assetIDs interface{}
	)
	limit := 100
	if len(r.URL.Query()["limit"]) > 0 && len(r.URL.Query()["limit"][0]) > 0 {
		var err error
		limit, err = strconv.Atoi(r.URL.Query()["limit"][0])
		if err != nil {
			common.WriteError(w, err)
			return
		}
	}

	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = asset.GetAssetsByMetadata(ctx, tx, wl.Userid, r.URL.Query(), 0, limit)
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	w.Header().Add(totalCountHeader, strconv.Itoa(total))
	common.WriteBody(w, assetIDs)
}

func (h *Handler) searchAssets(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}
	page := 0
	if len(r.URL.Query()["page"]) > 0 && len(r.URL.Query()["page"][0]) > 0 {
		var err error
		page, err = strconv.Atoi(r.URL.Query()["page"][0])
		if err != nil {
			common.WriteError(w, err)
			return
		}
	}
	limit := 100
	if len(r.URL.Query()["limit"]) > 0 && len(r.URL.Query()["limit"][0]) > 0 {
		var err error
		limit, err = strconv.Atoi(r.URL.Query()["limit"][0])
		if err != nil {
			common.WriteError(w, err)
			return
		}
	}

	var (
		total    int
		assetIDs interface{}
	)
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		_, ok := r.URL.Query()[asset.QueryAssetsNoAlbum]
		if ok {
			assetIDs, total, err = asset.GetAssetsNotInAlbums(ctx, tx, wl.Userid, page, limit)
		} else {
			assetIDs, total, err = asset.GetAssetsByMetadata(ctx, tx, wl.Userid, r.URL.Query(), page, limit)
		}
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	w.Header().Add(totalCountHeader, strconv.Itoa(total))
	common.WriteBody(w, assetIDs)
}

func (h *Handler) getMetadataPlaces(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	var places []types.GeoLocation
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		places, err = geo.GetGeoLocations(ctx, tx, r.URL.Query().Get(common.QueryKeyAll) == "1")
		return err
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, places)
}

func (h *Handler) updateMetadataPlace(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	var place types.GeoLocation
	if err := json.NewDecoder(r.Body).Decode(&place); err != nil {
		common.WriteError(w, err)
		return
	}

	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		return geo.UpdatePlaces(ctx, tx, place)
	}); err != nil {
		common.WriteError(w, err)
		return
	}
}

func (h *Handler) getMetadataByIDs(w http.ResponseWriter, r *http.Request) {
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

	if len(ids) > 100 {
		logrus.Errorf("Requested %d asset IDs which is beyond limit 100", len(ids))
		common.WriteError(w, common.ErrBadRequest)
		return
	}

	// Deliberately does NOT probe/update GPS the way the single-asset
	// getAssetMetadatas does: probeAndUpdateGeo shells out to exiftool per
	// asset, and doing that sequentially for up to 100 assets in one request
	// turned every call to this endpoint into a multi-minute stall that
	// starved unrelated requests (previews included) on this NAS's single
	// lomod process. Geo backfill already has its own dedicated, throttled
	// path -- see lomo-mobile's SyncService.syncRemoteGPS, which calls
	// getAssetMetadatas (the single-asset endpoint) a few at a time
	// specifically to stay within this hardware's concurrency budget.
	metadatas := []*types.Asset{}
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		for _, id := range ids {
			a, err := asset.GetAssetByID(ctx, tx, wl.Userid, id)
			if err != nil {
				if common.IsErrNoRows(err) {
					logrus.Errorf("Requested asset %d not found", id)
					continue
				}
				return err
			}
			metadatas = append(metadatas, a)
		}
		return nil
	}); err != nil {
		common.WriteError(w, err)
		return
	}
	common.WriteBody(w, metadatas)
}
