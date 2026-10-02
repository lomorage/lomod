package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"

	. "gopkg.in/check.v1"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/album"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/types"
)

func (ts *mainSuite) createAlbum(c *C, a *album.Album) {
	contents, err := json.Marshal(a)
	c.Assert(err, IsNil)
	buf := bytes.NewBuffer(contents)
	res := &album.Album{}
	ts.requestWithMethodBody(c, "/album?token="+ts.token, http.MethodPost, http.StatusOK, buf, res, nil, nil)
	c.Assert(res.ID, Not(Equals), 0)
	a.ID = res.ID
}

func (ts *mainSuite) deleteAlbum(c *C, aid, expectStatus int, token string) {
	ts.requestWithMethod(c, fmt.Sprintf("/album/%d?token=%s", aid, token), http.MethodDelete, expectStatus, nil, nil, nil)
}

func (ts *mainSuite) deleteAlbums(c *C, ids []int, expectStatus int, token string) {
	content, err := json.Marshal(ids)
	c.Assert(err, IsNil)
	ts.requestWithMethodBody(c, fmt.Sprintf("/album?token=%s", token), http.MethodDelete, expectStatus,
		bytes.NewBuffer(content), nil, nil, nil)
}

func (ts *mainSuite) updateAlbum(c *C, a *album.Album) {
	contents, err := json.Marshal(a)
	c.Assert(err, IsNil)
	buf := bytes.NewBuffer(contents)
	ts.requestWithMethodBody(c, "/album?token="+ts.token, http.MethodPut, http.StatusOK, buf, nil, nil, nil)
}

func (ts *mainSuite) listAlbums(c *C) *album.Albums {
	as := &album.Albums{}
	ts.requestGet(c, "/album?token="+ts.token, &as, nil)
	return as
}

func (ts *mainSuite) TestAlbumBasic(c *C) {
	a1 := &album.Album{Title: "favorite", Description: "test album", Author: "test"}
	ts.createAlbum(c, a1)
	a2 := &album.Album{Title: "face", Description: "face album", Author: "face"}
	ts.createAlbum(c, a2)

	list := ts.listAlbums(c)
	c.Assert(len(list.Albums), Equals, 2)
	c.Assert(list.Albums[0].ID, Equals, a2.ID)
	c.Assert(list.Albums[0].Title, Equals, a2.Title)
	c.Assert(list.Albums[0].Description, Equals, a2.Description)
	c.Assert(list.Albums[0].Author, Equals, a2.Author)
	c.Assert(list.Albums[1].ID, Equals, a1.ID)
	c.Assert(list.Albums[1].Title, Equals, a1.Title)
	c.Assert(list.Albums[1].Description, Equals, a1.Description)
	c.Assert(list.Albums[1].Author, Equals, a1.Author)

	// test update
	a1.Title = "my favorite"
	a1.Author = "new"
	ts.updateAlbum(c, a1)

	list = ts.listAlbums(c)
	c.Assert(len(list.Albums), Equals, 2)
	c.Assert(list.Albums[0].ID, Equals, a2.ID)
	c.Assert(list.Albums[0].Title, Equals, a2.Title)
	c.Assert(list.Albums[0].Description, Equals, a2.Description)
	c.Assert(list.Albums[0].Author, Equals, a2.Author)
	c.Assert(list.Albums[1].ID, Equals, a1.ID)
	c.Assert(list.Albums[1].Title, Equals, a1.Title)
	c.Assert(list.Albums[1].Description, Equals, a1.Description)
	c.Assert(list.Albums[1].Author, Equals, a1.Author)

	// bob can not delete alice album
	ts.deleteAlbum(c, a1.ID, http.StatusBadRequest, ts.tokenBob)
	list = ts.listAlbums(c)
	c.Assert(len(list.Albums), Equals, 2)
	c.Assert(list.Albums[0].ID, Equals, a2.ID)
	c.Assert(list.Albums[1].ID, Equals, a1.ID)

	ts.deleteAlbum(c, a1.ID, http.StatusOK, ts.token)
	list = ts.listAlbums(c)
	c.Assert(len(list.Albums), Equals, 1)
	c.Assert(list.Albums[0].ID, Equals, a2.ID)
}

func (ts *mainSuite) getAlbumSummary(c *C, aid int) int {
	u := fmt.Sprintf("/album/%d/assets?token=%s", aid, ts.token)
	req, err := http.NewRequest(http.MethodHead, u, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, u, rr, http.StatusOK)

	res := rr.Result()
	defer res.Body.Close()

	c.Assert(len(res.Header[totalCountHeader]), Equals, 1)
	total, err := strconv.Atoi(res.Header[totalCountHeader][0])
	c.Assert(err, IsNil)

	return total
}

func (ts *mainSuite) listAlbumAssetIDs(c *C, aid, page int) []string {
	as := []string{}
	ts.requestGet(c, fmt.Sprintf("/album/%d/assets?token=%s&page=%d", aid, ts.token, page), &as, nil)
	return as
}

func (ts *mainSuite) listAlbumsByAssetID(c *C, aid string) album.Albums {
	as := album.Albums{}
	ts.requestGet(c, fmt.Sprintf("/asset/album/%s?token=%s", aid, ts.token), &as, nil)
	return as
}

func (ts *mainSuite) listAlbumAssets(c *C, aid, page int) []types.AssetName {
	as := []types.AssetName{}
	ts.requestGet(c, fmt.Sprintf("/album/%d/assets?token=%s&%s=1&page=%d", aid, ts.token, common.QueryKeyRetHash,
		page), &as, nil)
	return as
}

func (ts *mainSuite) addAssetsInAlbum(c *C, expectStatus, aid int, ids []string) {
	contents, err := json.Marshal(ids)
	c.Assert(err, IsNil)
	buf := bytes.NewBuffer(contents)
	ts.requestWithMethodBody(c, fmt.Sprintf("/album/%d/assets?token=%s", aid, ts.token), http.MethodPost,
		expectStatus, buf, nil, nil, nil)
}

func (ts *mainSuite) removeAssetsInAlbum(c *C, expectStatus, aid int, ids []string) {
	contents, err := json.Marshal(ids)
	c.Assert(err, IsNil)
	buf := bytes.NewBuffer(contents)
	ts.requestWithMethodBody(c, fmt.Sprintf("/album/%d/assets?token=%s", aid, ts.token), http.MethodDelete,
		expectStatus, buf, nil, nil, nil)
}

func (ts *mainSuite) validateAlbumAssets(c *C, aid int, assetsInAlbum []types.AssetName) {
	count := ts.getAlbumSummary(c, aid)
	c.Assert(count, Equals, len(assetsInAlbum))
	assets := ts.listAlbumAssets(c, aid, 0)
	if len(assetsInAlbum) == 0 {
		c.Assert(len(assets), Equals, 0)
	} else {
		c.Assert(assets, DeepEquals, assetsInAlbum)
	}
}

func (ts *mainSuite) validateAlbumAssetIDs(c *C, aid int, assetsInAlbum []string) {
	count := ts.getAlbumSummary(c, aid)
	c.Assert(count, Equals, len(assetsInAlbum))
	assets := ts.listAlbumAssetIDs(c, aid, 0)
	if len(assetsInAlbum) == 0 {
		c.Assert(len(assets), Equals, 0)
	} else {
		c.Assert(assets, DeepEquals, assetsInAlbum)
	}
}

func (ts *mainSuite) TestAlbumAssets(c *C) {
	// non-exist album should have count 0
	ts.validateAlbumAssets(c, 1, nil)

	a1 := &album.Album{Title: "favorite", Description: "test album", Author: "test"}
	ts.createAlbum(c, a1)
	ts.validateAlbumAssets(c, a1.ID, nil)
	ts.validateAlbumAssetIDs(c, a1.ID, nil)

	// add non-exist asset ID and hash, it should fail
	ts.addAssetsInAlbum(c, http.StatusInternalServerError, a1.ID, []string{"1"})
	ts.addAssetsInAlbum(c, http.StatusInternalServerError, a1.ID, []string{"0c03dcd19f804a559bd0ae32db2998dad4a4936a"})

	// add exist ID and non-exist hash, or vice versa, it should fail
	ts.insertAssetsWithGPS(c)
	defer ts.waitPreviewAssetsWithGPS(context.Background(), c)

	ts.addAssetsInAlbum(c, http.StatusInternalServerError, a1.ID, []string{"1", "0c03dcd19f804a559bd0ae32db2998dad4a4936a"})
	ts.addAssetsInAlbum(c, http.StatusInternalServerError, a1.ID, []string{"100", gpsAssets[0].hash})

	// add good ones to non-exist album
	ts.addAssetsInAlbum(c, http.StatusInternalServerError, 0, []string{"1", gpsAssets[0].hash})

	// add duplicate id and hash to correct album, then list to compare. It should have only 1 asset
	ts.addAssetsInAlbum(c, http.StatusOK, a1.ID, []string{"1", gpsAssets[0].hash})

	ts.validateAlbumAssets(c, a1.ID, []types.AssetName{{Name: "1.jpg", Hash: gpsAssets[0].hash}})
	ts.validateAlbumAssetIDs(c, a1.ID, []string{"1.jpg"})

	// delete non-exist asset and compare list
	ts.removeAssetsInAlbum(c, http.StatusBadRequest, a1.ID, []string{"100"})
	ts.removeAssetsInAlbum(c, http.StatusBadRequest, a1.ID, []string{"0c03dcd19f804a559bd0ae32db2998dad4a4936a"})
	ts.validateAlbumAssets(c, a1.ID, []types.AssetName{{Name: "1.jpg", Hash: gpsAssets[0].hash}})
	ts.validateAlbumAssetIDs(c, a1.ID, []string{"1.jpg"})

	// delete correct asset and compare again
	ts.removeAssetsInAlbum(c, http.StatusOK, a1.ID, []string{"1"})
	ts.removeAssetsInAlbum(c, http.StatusOK, a1.ID, []string{gpsAssets[0].hash})

	ts.validateAlbumAssets(c, a1.ID, nil)
	ts.validateAlbumAssetIDs(c, a1.ID, nil)

	// add two assets again
	ts.addAssetsInAlbum(c, http.StatusOK, a1.ID, []string{"1", gpsAssets[1].hash})

	ts.validateAlbumAssets(c, a1.ID, []types.AssetName{
		{Name: "2.heic", Hash: gpsAssets[1].hash},
		{Name: "1.jpg", Hash: gpsAssets[0].hash},
	})
	ts.validateAlbumAssetIDs(c, a1.ID, []string{"2.heic", "1.jpg"})

	// get 2nd page should have nothing
	assets := ts.listAlbumAssets(c, a1.ID, 1)
	c.Assert(len(assets), Equals, 0)

	// add 2nd album and adds 2 assets into the 2nd album
	a2 := &album.Album{Title: "face", Description: "face album", Author: "face"}
	ts.createAlbum(c, a2)
	ts.addAssetsInAlbum(c, http.StatusOK, a2.ID, []string{"1", gpsAssets[1].hash})

	ts.validateAlbumAssets(c, a2.ID, []types.AssetName{
		{Name: "2.heic", Hash: gpsAssets[1].hash},
		{Name: "1.jpg", Hash: gpsAssets[0].hash},
	})
	ts.validateAlbumAssetIDs(c, a1.ID, []string{"2.heic", "1.jpg"})

	// delete from the 1st album again, 2nd album should not be impacted
	ts.removeAssetsInAlbum(c, http.StatusOK, a1.ID, []string{"1", gpsAssets[1].hash})

	ts.validateAlbumAssets(c, a1.ID, nil)
	ts.validateAlbumAssets(c, a2.ID, []types.AssetName{
		{Name: "2.heic", Hash: gpsAssets[1].hash},
		{Name: "1.jpg", Hash: gpsAssets[0].hash},
	})
	ts.validateAlbumAssetIDs(c, a1.ID, nil)
	ts.validateAlbumAssetIDs(c, a2.ID, []string{"2.heic", "1.jpg"})
}

func (ts *mainSuite) TestAlbumDeleteAsset(c *C) {
	ts.insertAssetsWithGPS(c)

	// when one asset is deleted, it should be removed from album too
	a1 := &album.Album{Title: "favorite", Description: "test album", Author: "test"}
	ts.createAlbum(c, a1)
	ts.addAssetsInAlbum(c, http.StatusOK, a1.ID, []string{"1", "2"})

	assets := ts.listAlbumAssets(c, a1.ID, 0)
	c.Assert(assets, DeepEquals, []types.AssetName{
		{Name: "2.heic", Hash: gpsAssets[1].hash},
		{Name: "1.jpg", Hash: gpsAssets[0].hash},
	})

	ts.requestDelete(c, ts.token, &types.DeleteAssetItems{
		List: []types.DeleteAssetItem{{ID: "1"}},
	})

	assets = ts.listAlbumAssets(c, a1.ID, 0)
	c.Assert(assets, DeepEquals, []types.AssetName{
		{Name: "2.heic", Hash: gpsAssets[1].hash},
	})

	// read from DB directly to make sure it is removed indeed
	count := 0
	err := dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "select count(*) from asset_album where album_id = ?", a1.ID).Scan(&count)
	})
	c.Assert(err, IsNil)
	c.Assert(count, Equals, 1)

	ts.requestDelete(c, ts.token, &types.DeleteAssetItems{
		List: []types.DeleteAssetItem{{ID: "2"}},
	})

	assets = ts.listAlbumAssets(c, a1.ID, 0)
	c.Assert(len(assets), Equals, 0)

	// query DB directly
	err = dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "select count(*) from asset_album where album_id = ?", a1.ID).Scan(&count)
	})
	c.Assert(err, IsNil)
	c.Assert(count, Equals, 0)
}

func (ts *mainSuite) TestAlbumDeleteMultiples(c *C) {
	a1 := &album.Album{Title: "favorite", Description: "test album", Author: "test"}
	ts.createAlbum(c, a1)
	a2 := &album.Album{Title: "face", Description: "face album", Author: "face"}
	ts.createAlbum(c, a2)

	ts.deleteAlbums(c, []int{1, 2}, http.StatusOK, ts.token)

	list := ts.listAlbums(c)
	c.Assert(len(list.Albums), Equals, 0)
}

func (ts *mainSuite) TestAssetNotInAlbum(c *C) {
	ts.insertAssetsWithGPS(c)
	q := map[string][]string{asset.QueryAssetsNoAlbum: {"1"}}
	ids := []types.AssetName{}
	count := ts.searchAssets(c, "/assets", ts.token, q, &ids)
	c.Assert(count, Equals, 2)
	c.Assert(ids, DeepEquals, []types.AssetName{{"2.heic", gpsAssets[1].hash}, {"1.jpg", gpsAssets[0].hash}})

	// bob should not get any assets
	count = ts.searchAssets(c, "/assets", ts.tokenBob, q, &ids)
	c.Assert(count, Equals, 0)
	c.Assert(len(ids), Equals, 0)

	a1 := &album.Album{Title: "favorite", Description: "test album", Author: "test"}
	ts.createAlbum(c, a1)

	ts.addAssetsInAlbum(c, http.StatusOK, a1.ID, []string{"1"})

	count = ts.searchAssets(c, "/assets", ts.token, q, &ids)
	c.Assert(count, Equals, 1)
	c.Assert(ids, DeepEquals, []types.AssetName{{"2.heic", gpsAssets[1].hash}})

	ts.addAssetsInAlbum(c, http.StatusOK, a1.ID, []string{"2"})

	count = ts.searchAssets(c, "/assets", ts.token, q, &ids)
	c.Assert(count, Equals, 0)
	c.Assert(len(ids), Equals, 0)
}

func (ts *mainSuite) TestAlbumsByAssetID(c *C) {
	ts.insertAssetsWithGPS(c)

	a1 := &album.Album{Title: "favorite", Description: "test album", Author: "test"}
	ts.createAlbum(c, a1)
	a2 := &album.Album{Title: "face", Description: "face album", Author: "face"}
	ts.createAlbum(c, a2)

	ts.addAssetsInAlbum(c, http.StatusOK, a1.ID, []string{"1", "2"})
	ts.addAssetsInAlbum(c, http.StatusOK, a2.ID, []string{"1", "2"})

	as := ts.listAlbumsByAssetID(c, "1")
	c.Assert(as.Albums, DeepEquals, []album.Album{*a2, *a1})

	as = ts.listAlbumsByAssetID(c, "2")
	c.Assert(as.Albums, DeepEquals, []album.Album{*a2, *a1})

	as = ts.listAlbumsByAssetID(c, gpsAssets[0].hash)
	c.Assert(as.Albums, DeepEquals, []album.Album{*a2, *a1})

	as = ts.listAlbumsByAssetID(c, gpsAssets[1].hash)
	c.Assert(as.Albums, DeepEquals, []album.Album{*a2, *a1})
}
