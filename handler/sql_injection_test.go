package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	. "gopkg.in/check.v1"
)

// Integration tests: query parameters and asset IDs from requests must never
// be spliced into SQL. Each test gives alice and bob their own assets and
// checks a crafted request from alice can't reach bob's.

// sqlInjectionName closes the quoted name and ORs in a condition that's always
// true; no spaces, so it survives as a URL path segment unchanged.
const sqlInjectionName = "x'or'1'='1"

var (
	aliceHash = strings.Repeat("a", 40)
	bobHash   = strings.Repeat("b", 40)
)

func (ts *mainSuite) userIDByName(c *C, name string) int {
	id := 0
	c.Assert(dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "select id from user where user_name = ?", name).Scan(&id)
	}), IsNil)
	return id
}

// insertGeoAsset inserts an asset owned by uid with one geo metadata entry.
func (ts *mainSuite) insertGeoAsset(c *C, uid int, hash, name, value string) int {
	id := 0
	c.Assert(dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		aid, err := asset.InsertAsset(ctx, tx, uid, 0, ext.JPG, &types.Asset{Hash: hash}, 1, 0, time.Now())
		if err != nil {
			return err
		}
		id = int(aid)
		return asset.InsertOrUpdateMetadata(ctx, tx, types.Metadata{
			Category:     types.MetadataCategoryGeo,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      id,
			Name:         name,
			Value:        value,
			Version:      1,
		})
	}), IsNil)
	return id
}

func (ts *mainSuite) metadataSearchStatus(c *C, query url.Values) (int, []int) {
	req := httptest.NewRequest(http.MethodGet, "/assets/metadata?token="+ts.token+"&"+query.Encode(), nil)
	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	ids := []int{}
	if rr.Code == http.StatusOK {
		c.Assert(json.NewDecoder(rr.Body).Decode(&ids), IsNil)
	}
	return rr.Code, ids
}

func (ts *mainSuite) setUpTwoUsersGeo(c *C) (aliceAsset, bobAsset int) {
	aliceAsset = ts.insertGeoAsset(c, ts.userIDByName(c, "alice"), aliceHash, "city", "Paris")
	bobAsset = ts.insertGeoAsset(c, ts.userIDByName(c, "bob"), bobHash, "city", "Rome")
	return
}

func (ts *mainSuite) TestMetadataSearchByNameNotInjectable(c *C) {
	aliceAsset, _ := ts.setUpTwoUsersGeo(c)

	code, ids := ts.metadataSearchStatus(c, url.Values{asset.QueryMetadataName: {"city"}})
	c.Assert(code, Equals, http.StatusOK)
	c.Assert(ids, DeepEquals, []int{aliceAsset})

	code, ids = ts.metadataSearchStatus(c, url.Values{asset.QueryMetadataName: {sqlInjectionName}})
	c.Assert(code, Equals, http.StatusOK)
	c.Assert(ids, DeepEquals, []int{})

	code, ids = ts.metadataSearchStatus(c, url.Values{asset.QueryMetadataNameMiss: {sqlInjectionName}})
	c.Assert(code, Equals, http.StatusOK)
	c.Assert(ids, DeepEquals, []int{aliceAsset}) // alice's asset lacks that name; bob's stays out
}

func (ts *mainSuite) TestMetadataSearchByNameValueNotInjectable(c *C) {
	aliceAsset, _ := ts.setUpTwoUsersGeo(c)
	quoted := ts.insertGeoAsset(c, ts.userIDByName(c, "alice"), "alice-hash-2", "street", "O'Brien St")

	geo := string(types.MetadataCategoryGeo)
	code, ids := ts.metadataSearchStatus(c, url.Values{asset.QueryMetadataCategory: {geo},
		asset.QueryMetadataNV: {"city" + common.QueryDelimiterKV + "Par"}})
	c.Assert(code, Equals, http.StatusOK)
	c.Assert(ids, DeepEquals, []int{aliceAsset})

	// a value with a quote is an ordinary search, not broken SQL
	code, ids = ts.metadataSearchStatus(c, url.Values{asset.QueryMetadataCategory: {geo},
		asset.QueryMetadataNV: {"street" + common.QueryDelimiterKV + "O'Brien"}})
	c.Assert(code, Equals, http.StatusOK)
	c.Assert(ids, DeepEquals, []int{quoted})

	for _, nv := range []string{
		sqlInjectionName + common.QueryDelimiterKV + "x",
		"city" + common.QueryDelimiterKV + "x') or ('1'='1",
	} {
		code, ids = ts.metadataSearchStatus(c, url.Values{asset.QueryMetadataCategory: {geo}, asset.QueryMetadataNV: {nv}})
		c.Assert(code, Equals, http.StatusOK, Commentf("%s", nv))
		c.Assert(ids, DeepEquals, []int{}, Commentf("%s", nv))
	}
}

func (ts *mainSuite) TestMetadataSearchVersionMustBeNumber(c *C) {
	aliceAsset, _ := ts.setUpTwoUsersGeo(c)

	code, ids := ts.metadataSearchStatus(c, url.Values{
		asset.QueryMetadataName: {"city"}, asset.QueryMetadataVersionLess: {"2"}})
	c.Assert(code, Equals, http.StatusOK)
	c.Assert(ids, DeepEquals, []int{aliceAsset})

	code, _ = ts.metadataSearchStatus(c, url.Values{
		asset.QueryMetadataName: {"city"}, asset.QueryMetadataVersionLess: {"2 or 1=1"}})
	c.Assert(code, Not(Equals), http.StatusOK)
}

func (ts *mainSuite) TestMetadataValuesNotInjectable(c *C) {
	ts.setUpTwoUsersGeo(c)
	c.Assert(ts.listMetadataValue(c, string(types.MetadataCategoryGeo), "city"), DeepEquals, []string{"Paris"})
	c.Assert(ts.listMetadataValue(c, string(types.MetadataCategoryGeo), sqlInjectionName), HasLen, 0)
}

func (ts *mainSuite) assetStatus(c *C, id int) int {
	status := 0
	c.Assert(dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "select status from asset where id = ?", id).Scan(&status)
	}), IsNil)
	return status
}

func (ts *mainSuite) TestHideAssetsIDsNotInjectable(c *C) {
	aliceAsset, bobAsset := ts.setUpTwoUsersGeo(c)
	ts.h.conf.UseMemdb = true
	c.Assert(ts.h.openMemDB(), IsNil)

	hide := func(ids []string) int {
		body, err := json.Marshal(ids)
		c.Assert(err, IsNil)
		req := httptest.NewRequest(http.MethodPost, "/assets/hide?token="+ts.token, bytes.NewReader(body))
		rr := httptest.NewRecorder()
		ts.h.CreateRouter().ServeHTTP(rr, req)
		return rr.Code
	}

	hide([]string{"0) or (1=1"})
	c.Assert(ts.assetStatus(c, aliceAsset), Equals, 0)
	c.Assert(ts.assetStatus(c, bobAsset), Equals, 0)

	// bob's asset by its real ID is still out of alice's reach
	hide([]string{strconv.Itoa(bobAsset) + ".jpg"})
	c.Assert(ts.assetStatus(c, bobAsset), Equals, 0)

	c.Assert(hide([]string{strconv.Itoa(aliceAsset) + ".jpg"}), Equals, http.StatusOK)
	c.Assert(types.IsAssetStatus(ts.assetStatus(c, aliceAsset), types.AssetStatusHidden), Equals, true)
}

func (ts *mainSuite) TestValidateAssetIDsNotInjectable(c *C) {
	aliceAsset, bobAsset := ts.setUpTwoUsersGeo(c)
	aliceID := ts.userIDByName(c, "alice")

	// IsHash only checks the length, so pad the injection to 40 characters
	fakeHash := "') or (1=1) or hash in ('"
	fakeHash += strings.Repeat("a", 40-len(fakeHash))
	c.Assert(asset.IsHash(fakeHash), Equals, true)

	validate := func(ids []string) ([]int, []string, error) {
		var (
			notExist []string
			found    []int
		)
		err := dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
			var err error
			found, notExist, err = asset.ValidateAssetIDs(ctx, tx, aliceID, ids)
			return err
		})
		return found, notExist, err
	}

	found, notExist, err := validate([]string{fakeHash})
	c.Assert(err, IsNil)
	c.Assert(found, HasLen, 0)
	c.Assert(notExist, DeepEquals, []string{fakeHash})

	_, _, err = validate([]string{"0) or (1=1"})
	c.Assert(err, Equals, common.ErrBadRequest)

	found, notExist, err = validate([]string{strconv.Itoa(aliceAsset) + ".jpg", aliceHash, bobHash, strconv.Itoa(bobAsset)})
	c.Assert(err, IsNil)
	c.Assert(found, DeepEquals, []int{aliceAsset})
	sort.Strings(notExist)
	c.Assert(notExist, DeepEquals, []string{strconv.Itoa(bobAsset), bobHash})
}

func (ts *mainSuite) TestMetadataSearchByNameOnly(c *C) {
	aliceAsset, _ := ts.setUpTwoUsersGeo(c)
	// "name" with no ",value" part used to produce unbalanced SQL
	code, ids := ts.metadataSearchStatus(c, url.Values{asset.QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
		asset.QueryMetadataNV: {"city"}})
	c.Assert(code, Equals, http.StatusOK)
	c.Assert(ids, DeepEquals, []int{aliceAsset})
}
