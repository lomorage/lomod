package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	. "gopkg.in/check.v1"
)

func (ts *mainSuite) TestMetadataName(c *C) {
	ts.initFakeAssets(c)

	ts.h.conf.UseMemdb = true
	c.Assert(ts.h.openMemDB(), IsNil)

	reply := ts.testInsertMetadatas(c, []types.Metadata{
		{
			Category:     types.MetadataCategoryTag,
			SourceDevice: types.SourceDeviceAndroid,
			AssetID:      1,
			Name:         "my key",
			Value:        "123",
			Model:        "tensorflow",
			Version:      1,
		}}, false, false)
	c.Assert(string(reply), Equals, `{"id": "0", "text": "invalid metadata name my key"}
`)
	reply = ts.testInsertMetadatas(c, []types.Metadata{
		{
			Category:     types.MetadataCategoryTag,
			SourceDevice: types.SourceDeviceAndroid,
			AssetID:      1,
			Name:         "名字",
			Value:        "",
			Model:        "tensorflow",
			Version:      1,
		}}, false, false)
	c.Assert(string(reply), Equals, `{"id": "0", "text": "invalid metadata name 名字"}
`)
}

func (ts *mainSuite) TestMetadataBasic(c *C) {
	ts.initFakeAssets(c)

	ts.h.conf.UseMemdb = true
	c.Assert(ts.h.openMemDB(), IsNil)

	a := ts.getAssetWithMetadataByID(c, 1)
	c.Assert(a, NotNil)
	c.Assert(a.Name, Equals, "1.jpg")
	c.Assert(a.Metadatas, IsNil)

	// insert category, try different valid metadata name
	testName1 := "city.1"
	testValue1 := "san jose"
	testValue2 := "sfo"
	testName2 := "POI-2"
	poiValue1 := "museum"
	poiValue2 := "动物园"
	testName3 := "People_3"
	faceValue1 := "alice"

	// test asset metadata:
	// 1: geo: city - san jose ; android
	// 2: geo: city - sfo; poi - museum; ios
	// 3: geo: city - san jose; android, poi - zoo; ios
	// 4: face: people - alice; android
	// use even number asset ID because it is for alice's fake assets
	metas := []types.Metadata{
		{
			Category:     types.MetadataCategoryTag,
			SourceDevice: types.SourceDeviceAndroid,
			AssetID:      1,
			Name:         testName1,
			Value:        testValue1,
			Model:        "tensorflow",
			Version:      1,
		},
		{
			Category:     types.MetadataCategoryTag,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      3,
			Name:         testName1,
			Value:        testValue2,
			Model:        "tensorflow",
			Version:      1,
		},
		{
			Category:     types.MetadataCategoryTag,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      3,
			Name:         testName2,
			Value:        poiValue1,
			Model:        "tensorflow",
			Version:      2,
		},
		{
			Category:     types.MetadataCategoryTag,
			SourceDevice: types.SourceDeviceAndroid,
			AssetID:      5,
			Name:         testName1,
			Value:        testValue1,
			Model:        "tensorflow",
			Version:      1,
		},
		{
			Category:     types.MetadataCategoryTag,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      5,
			Name:         testName2,
			Value:        poiValue2,
			Model:        "tensorflow",
			Version:      1,
		},
		{
			Category:     types.MetadataCategoryFace,
			SourceDevice: types.SourceDeviceAndroid,
			AssetID:      7,
			Name:         testName3,
			Value:        faceValue1,
			Model:        "tensorflow",
			Version:      1,
		},
	}

	asset1Meta := []types.Metadata{metas[0]}
	asset3Meta := []types.Metadata{metas[2], metas[1]}
	asset5Meta := []types.Metadata{metas[4], metas[3]}
	asset7Meta := []types.Metadata{metas[5]}

	ts.testInsertMetadatas(c, metas, false, true)

	// no places because name is not qualified
	places := ts.listMetadataPlaces(c)
	c.Assert(len(places), Equals, 0)

	a = ts.getAssetWithMetadataByID(c, 1)
	c.Assert(a, NotNil)
	c.Assert(a.Name, Equals, "1.jpg")
	c.Assert(a.Metadatas, NotNil)
	c.Assert(len(*a.Metadatas), Equals, 1)
	c.Assert((*a.Metadatas)[0].Category.String(), Equals, types.MetadataCategoryTag)
	c.Assert((*a.Metadatas)[0].SourceDevice, Equals, types.SourceDeviceAndroid)
	c.Assert((*a.Metadatas)[0].Name, Equals, testName1)
	c.Assert((*a.Metadatas)[0].Value, Equals, testValue1)
	c.Assert((*a.Metadatas)[0].Model, Equals, "tensorflow")
	c.Assert((*a.Metadatas)[0].Version, Equals, 1)

	categories := ts.listMetadataCategorys(c)
	c.Assert(categories, DeepEquals, []string{types.MetadataCategoryFace,
		types.MetadataCategoryTag})

	names := ts.listMetadataName(c, types.MetadataCategoryTag)
	c.Assert(names, DeepEquals, []string{testName2, testName1})
	names = ts.listMetadataName(c, types.MetadataCategoryFace)
	c.Assert(names, DeepEquals, []string{testName3})

	values := ts.listMetadataValue(c, types.MetadataCategoryTag, testName1)
	c.Assert(values, DeepEquals, []string{testValue1, testValue2})
	values = ts.listMetadataValue(c, types.MetadataCategoryTag, testName2)
	c.Assert(values, DeepEquals, []string{poiValue1, poiValue2})
	values = ts.listMetadataValue(c, types.MetadataCategoryFace, testName3)
	c.Assert(values, DeepEquals, []string{faceValue1})

	// all assets without geo category
	assets, total := ts.getMetadataTasks(c, map[string][]string{
		asset.QueryMetadataCategoryMiss: {types.MetadataCategoryTag},
	})
	c.Assert(total, Equals, 5)
	c.Assert(assets, DeepEquals, []int{15, 13, 11, 9, 7})

	// all assets without face category
	assets, total = ts.getMetadataTasks(c, map[string][]string{
		asset.QueryMetadataCategoryMiss: {types.MetadataCategoryFace},
	})
	c.Assert(total, Equals, 7)
	c.Assert(assets, DeepEquals, []int{15, 13, 11, 9, 5, 3, 1})

	// all assets without geo category @ android
	assets, total = ts.getMetadataTasks(c, map[string][]string{
		asset.QueryMetadataCategoryMiss: {types.MetadataCategoryTag},
		asset.QueryMetadataSourceDevice: {string(types.SourceDeviceAndroid)},
	})
	c.Assert(total, Equals, 6)
	c.Assert(assets, DeepEquals, []int{15, 13, 11, 9, 7, 3})

	// all assets without geo category @ ios
	assets, total = ts.getMetadataTasks(c, map[string][]string{
		asset.QueryMetadataCategoryMiss: {types.MetadataCategoryTag},
		asset.QueryMetadataSourceDevice: {string(types.SourceDeviceIos)},
	})
	c.Assert(total, Equals, 6)
	c.Assert(assets, DeepEquals, []int{15, 13, 11, 9, 7, 1})

	// all assets without face category @ android
	assets, total = ts.getMetadataTasks(c, map[string][]string{
		asset.QueryMetadataCategoryMiss: {types.MetadataCategoryFace},
		asset.QueryMetadataSourceDevice: {string(types.SourceDeviceAndroid)},
	})
	c.Assert(total, Equals, 7)
	c.Assert(assets, DeepEquals, []int{15, 13, 11, 9, 5, 3, 1})

	// all assets without face category @ ios
	assets, total = ts.getMetadataTasks(c, map[string][]string{
		asset.QueryMetadataCategoryMiss: {types.MetadataCategoryFace},
		asset.QueryMetadataSourceDevice: {string(types.SourceDeviceIos)},
	})
	c.Assert(total, Equals, 8)
	c.Assert(assets, DeepEquals, []int{15, 13, 11, 9, 7, 5, 3, 1})

	// all assets metadata version < 2
	assets, total = ts.getMetadataTasks(c, map[string][]string{
		asset.QueryMetadataVersionLess: {"2"},
	})
	c.Assert(total, Equals, 4)
	// asset 2 has 2 metadata, one has less version
	c.Assert(assets, DeepEquals, []int{7, 5, 3, 1})

	// all assets metadata version < 2 and geo category and name = city @ android
	assets, total = ts.getMetadataTasks(c, map[string][]string{
		asset.QueryMetadataCategory:     {types.MetadataCategoryTag},
		asset.QueryMetadataSourceDevice: {string(types.SourceDeviceAndroid)},
		asset.QueryMetadataVersionLess:  {"2"},
		asset.QueryMetadataName:         {testName1},
	})
	c.Assert(total, Equals, 2)
	c.Assert(assets, DeepEquals, []int{5, 1})

	// all assets metadata version < 2 and geo category and name = city @ ios
	assets, total = ts.getMetadataTasks(c, map[string][]string{
		asset.QueryMetadataCategory:     {types.MetadataCategoryTag},
		asset.QueryMetadataSourceDevice: {string(types.SourceDeviceIos)},
		asset.QueryMetadataVersionLess:  {"2"},
		asset.QueryMetadataName:         {testName1},
	})
	c.Assert(total, Equals, 1)
	c.Assert(assets, DeepEquals, []int{3})

	// all assets metadata having geo category but without name = city @ android
	assets, total = ts.getMetadataTasks(c, map[string][]string{
		asset.QueryMetadataCategory:     {types.MetadataCategoryTag},
		asset.QueryMetadataSourceDevice: {string(types.SourceDeviceAndroid)},
		asset.QueryMetadataNameMiss:     {testName1},
	})
	c.Assert(total, Equals, 6)
	c.Assert(assets, DeepEquals, []int{15, 13, 11, 9, 7, 3})

	// all assets metadata having geo category but without name = city @ ios
	assets, total = ts.getMetadataTasks(c, map[string][]string{
		asset.QueryMetadataCategory:     {types.MetadataCategoryTag},
		asset.QueryMetadataSourceDevice: {string(types.SourceDeviceIos)},
		asset.QueryMetadataNameMiss:     {testName1},
	})
	c.Assert(total, Equals, 7)
	c.Assert(assets, DeepEquals, []int{15, 13, 11, 9, 7, 5, 1})

	// search assets
	assets, total = ts.getMetadataTasks(c, map[string][]string{
		asset.QueryMetadataCategory: {types.MetadataCategoryTag},
		asset.QueryMetadataNV:       {testName3 + "," + testValue1},
	})
	c.Assert(total, Equals, 0)
	c.Assert(len(assets), Equals, 0)

	assets, total = ts.getMetadataTasks(c, map[string][]string{
		asset.QueryMetadataCategory: {types.MetadataCategoryTag},
		asset.QueryMetadataNV:       {testName1 + "," + faceValue1},
	})
	c.Assert(total, Equals, 0)
	c.Assert(len(assets), Equals, 0)

	assets, total = ts.getMetadataTasks(c, map[string][]string{
		asset.QueryMetadataCategory:     {types.MetadataCategoryTag},
		asset.QueryMetadataSourceDevice: {string(types.SourceDeviceAndroid)},
		asset.QueryMetadataNV:           {testName1 + "," + testValue1},
	})
	c.Assert(total, Equals, 2)
	c.Assert(assets, DeepEquals, []int{5, 1})

	assets, total = ts.getMetadataTasks(c, map[string][]string{
		asset.QueryMetadataCategory:     {types.MetadataCategoryTag},
		asset.QueryMetadataSourceDevice: {string(types.SourceDeviceIos)},
		asset.QueryMetadataNV:           {testName1 + "," + testValue1},
	})
	c.Assert(total, Equals, 0)
	c.Assert(len(assets), Equals, 0)

	assets, total = ts.getMetadataTasks(c, map[string][]string{
		asset.QueryMetadataCategory:     {types.MetadataCategoryTag},
		asset.QueryMetadataSourceDevice: {string(types.SourceDeviceAndroid)},
		asset.QueryMetadataNV:           {testName1 + "," + testValue2},
	})
	c.Assert(total, Equals, 0)
	c.Assert(len(assets), Equals, 0)

	assets, total = ts.getMetadataTasks(c, map[string][]string{
		asset.QueryMetadataCategory:     {types.MetadataCategoryTag},
		asset.QueryMetadataSourceDevice: {string(types.SourceDeviceIos)},
		asset.QueryMetadataNV:           {testName1 + "," + testValue2},
	})
	c.Assert(total, Equals, 1)
	c.Assert(assets, DeepEquals, []int{3})

	// get all metadatas by ID, consider order by name case
	retMetas := ts.getAssetWithMetadataByIDs(c, []int{1, 2, 3, 4, 5, 6, 7})

	expectMetas := []*types.Asset{
		{Name: "1.jpg", Hash: "1", Device: "iphonex", Longitude: fakeGPS, Latitude: fakeGPS, Metadatas: &asset1Meta},
		{Name: "3.mp4", Hash: "3", Device: "iphonex", Longitude: fakeGPS, Latitude: fakeGPS, Metadatas: &asset3Meta},
		{Name: "5.jpg", Hash: "5", Device: "iphonex", Longitude: fakeGPS, Latitude: fakeGPS, Metadatas: &asset5Meta},
		{Name: "7.jpg", Hash: "7", Device: "iphonex", Longitude: fakeGPS, Latitude: fakeGPS, Metadatas: &asset7Meta},
	}
	(*expectMetas[0].Metadatas)[0].AssetID = 1
	(*expectMetas[1].Metadatas)[0].AssetID = 3
	(*expectMetas[1].Metadatas)[1].AssetID = 3
	(*expectMetas[2].Metadatas)[0].AssetID = 5
	(*expectMetas[2].Metadatas)[1].AssetID = 5
	(*expectMetas[3].Metadatas)[0].AssetID = 7

	// reset create time and last modified time for compare
	for id, a := range retMetas {
		metas := *a.Metadatas
		for i, meta := range metas {
			meta.CreateTime = types.LomoTime{}
			meta.LastModifiedTime = types.LomoTime{}
			metas[i] = meta
		}
		a.Date = types.LomoTime{}
		a.Metadatas = &metas
		retMetas[id] = a
	}
	c.Assert(retMetas, DeepEquals, expectMetas)
}

func (ts *mainSuite) testInsertMetadatas(c *C, metas []types.Metadata, force, ok bool) []byte {
	url := fmt.Sprintf("/assets/metadata?token=%s", ts.token)
	if force {
		url += "&&force=1"
	}
	contents, err := json.Marshal(metas)
	c.Assert(err, IsNil)
	buf := bytes.NewBuffer(contents)
	req, err := http.NewRequest(http.MethodPost, url, buf)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)

	res := rr.Result()
	defer res.Body.Close()
	if ok {
		validateStatusCode(c, url, rr, http.StatusOK)
		return nil
	}
	content, err := ioutil.ReadAll(res.Body)
	c.Assert(err, IsNil)
	return content
}

func (ts *mainSuite) listMetadataCategorys(c *C) []string {
	url := fmt.Sprintf("/assets/metadata/category?token=%s", ts.token)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, url, rr, http.StatusOK)

	res := rr.Result()
	defer res.Body.Close()

	reply := metadataReply{}
	c.Assert(json.NewDecoder(res.Body).Decode(&reply), IsNil)
	c.Assert(reply.Categories, NotNil)
	c.Assert(reply.CategoryNames, IsNil)
	c.Assert(reply.CategoryValues, IsNil)
	return *reply.Categories
}

func (ts *mainSuite) listMetadataName(c *C, category string) []string {
	url := fmt.Sprintf("/assets/metadata/%s/names?token=%s", category, ts.token)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, url, rr, http.StatusOK)

	res := rr.Result()
	defer res.Body.Close()

	reply := metadataReply{}
	c.Assert(json.NewDecoder(res.Body).Decode(&reply), IsNil)
	c.Assert(reply.Categories, IsNil)
	c.Assert(reply.CategoryNames, NotNil)
	c.Assert(reply.CategoryValues, IsNil)
	c.Assert(len(*reply.CategoryNames), Equals, 1)
	return (*reply.CategoryNames)[category]
}

func (ts *mainSuite) listMetadataValue(c *C, category string, name string) []string {
	url := fmt.Sprintf("/assets/metadata/%s/%s/values?token=%s", category, name, ts.token)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, url, rr, http.StatusOK)

	res := rr.Result()
	defer res.Body.Close()

	reply := metadataReply{}
	c.Assert(json.NewDecoder(res.Body).Decode(&reply), IsNil)
	c.Assert(reply.Categories, IsNil)
	c.Assert(reply.CategoryNames, IsNil)
	c.Assert(reply.CategoryValues, NotNil)
	c.Assert(len(*reply.CategoryValues), Equals, 1)
	return (*reply.CategoryValues)[category][name]
}

func (ts *mainSuite) searchAssets(c *C, prefix, token string, queries map[string][]string, reply interface{}) int {
	q := []string{}
	for k, v := range queries {
		for _, a := range v {
			q = append(q, k+"="+a)
		}
	}
	u := prefix + "?token=" + token + "&" + strings.Join(q, "&")
	req, err := http.NewRequest(http.MethodGet, u, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, u, rr, http.StatusOK)

	res := rr.Result()
	defer res.Body.Close()

	c.Assert(len(res.Header[totalCountHeader]), Equals, 1)
	total, err := strconv.Atoi(res.Header[totalCountHeader][0])
	c.Assert(err, IsNil)

	c.Assert(json.NewDecoder(res.Body).Decode(&reply), IsNil)
	return total
}

func (ts *mainSuite) getMetadataTasks(c *C, queries map[string][]string) ([]int, int) {
	tasks := []int{}
	return tasks, ts.searchAssets(c, "/assets/metadata", ts.token, queries, &tasks)
}

func (ts *mainSuite) getAssetWithMetadataByID(c *C, id int) *types.Asset {
	return ts.getAssetWithMetadata(c, fmt.Sprintf("/asset/metadata/%d?token=%s", id, ts.token))
}

func (ts *mainSuite) getAssetWithMetadataByIDs(c *C, ids []int) []*types.Asset {
	contents, err := json.Marshal(ids)
	c.Assert(err, IsNil)
	buf := bytes.NewBuffer(contents)
	var metas []*types.Asset
	ts.requestWithMethodBody(c, "/assets/metadata/byid?token="+ts.token, http.MethodPost, http.StatusOK, buf, &metas, nil, nil)

	return metas
}

func (ts *mainSuite) getAssetWithMetadataByHash(c *C, hash string) *types.Asset {
	return ts.getAssetWithMetadata(c, fmt.Sprintf("/asset/metadata/%s?token=%s", hash, ts.token))
}

func (ts *mainSuite) getAssetWithMetadata(c *C, u string) *types.Asset {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, u, rr, http.StatusOK)
	res := rr.Result()
	defer res.Body.Close()

	reply := &types.Asset{}
	c.Assert(json.NewDecoder(res.Body).Decode(reply), IsNil)
	return reply
}

func (ts *mainSuite) getAssetMetadata(c *C, aid int) *types.Asset {
	u := fmt.Sprintf("/asset/metadata/%d?token=%s", aid, ts.token)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, u, rr, http.StatusOK)
	res := rr.Result()
	defer res.Body.Close()

	reply := &types.Asset{}
	c.Assert(json.NewDecoder(res.Body).Decode(reply), IsNil)
	return reply
}

func (ts *mainSuite) TestMetadataGeoJIT(c *C) {
	ts.testMetadataGeoJIT(c, "../cmd/lomod/test/img/5_2003_11_23.jpg", 39.9155555555556, 116.390833333333)
	ts.testMetadataGeoJIT(c, "../cmd/lomod/test/img/14_2017_09_13.heic", -23.5398944444444, -46.65655)
}

func (ts *mainSuite) testMetadataGeoJIT(c *C, f string, lat, lon float64) {
	// insert one asset into DB, then get asset metadata which should scan and save geo data
	var assetID int
	t := time.Now()
	c.Assert(dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		e := ext.JPG
		if filepath.Ext(f) == ".heic" {
			e = ext.HEIC
		}
		id, err := asset.InsertAsset(ctx, tx, 1, 1, e, &types.Asset{Hash: f}, 0, 0, t)
		assetID = int(id)
		return err
	}), IsNil)

	// copy asset to
	m := strconv.Itoa(int(t.Month()))
	if int(t.Month()) <= 9 {
		m = "0" + m
	}
	d := strconv.Itoa(t.Day())
	if t.Day() <= 9 {
		d = "0" + d
	}
	dir := fmt.Sprintf("%s/alice/Photos/master/%d/%s/%s", photodir, t.Year(), m, d)
	c.Assert(os.MkdirAll(dir, 0755), IsNil)
	c.Assert(cmd.Exec("cp", f,
		fmt.Sprintf("%s/%d%s%s_%d%s", dir, t.Year(), m, d, assetID, filepath.Ext(f))), IsNil)

	a := ts.getAssetWithMetadataByID(c, assetID)
	c.Assert(a, NotNil)
	c.Assert(a.Latitude, Equals, lat)
	c.Assert(a.Longitude, Equals, lon)
	c.Assert(a.Metadatas, IsNil)

	// read from DB again
	c.Assert(dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		a, err = asset.GetAssetByID(ctx, tx, 1, assetID)
		return err
	}), IsNil)
	c.Assert(a.Latitude, Equals, lat)
	c.Assert(a.Longitude, Equals, lon)
}

func (ts *mainSuite) TestMetadataGetByHash(c *C) {
	ts.h.conf.UseMemdb = true
	c.Assert(ts.h.openMemDB(), IsNil)

	category, err := loadCategory()
	c.Assert(err, IsNil)

	ts.createAssets(c, &category, ts.token)

	// test hash with 1.jpg
	a := ts.getAssetWithMetadataByHash(c, "4ebf54db04f335ff66bfc1fd982be62bf23fc967")
	c.Assert(a, NotNil)
	c.Assert(a.Name, Equals, "1.jpg")
	c.Assert(a.Metadatas, IsNil)

	metas := []types.Metadata{testMetaCountryEn, testMetaStateEn, testMetaCityEn}

	ts.testInsertMetadatas(c, metas, false, true)

	a = ts.getAssetWithMetadataByHash(c, "4ebf54db04f335ff66bfc1fd982be62bf23fc967")
	c.Assert(a, NotNil)
	c.Assert(a.Name, Equals, "1.jpg")
	c.Assert(a.Metadatas, NotNil)
	c.Assert(len(*a.Metadatas), Equals, 3)
	c.Assert((*a.Metadatas)[0].Category.String(), Equals, types.MetadataCategoryGeo)
	c.Assert((*a.Metadatas)[0].SourceDevice, Equals, types.SourceDeviceIos)
	c.Assert((*a.Metadatas)[0].Name, Equals, testMetaCityEn.Name)
	c.Assert((*a.Metadatas)[0].Value, Equals, testMetaCityEn.Value)
	c.Assert((*a.Metadatas)[0].Model, Equals, "tensorflow")
	c.Assert((*a.Metadatas)[0].Version, Equals, 1)

	places := ts.listMetadataPlaces(c)
	content, _ := json.Marshal(places)
	fmt.Println(string(content))
	c.Assert(places, DeepEquals, []types.GeoLocation{
		{ID: 3, Country: testMetaCountryEn.Value, State: normalizedState, City: testMetaCityEn.Value, Level: types.MetadataGeoLevelCity},
	})
}

func (ts *mainSuite) TestMetadataGetNonExistIDOrHash(c *C) {
	ts.h.conf.UseMemdb = true
	c.Assert(ts.h.openMemDB(), IsNil)

	category, err := loadCategory()
	c.Assert(err, IsNil)

	ts.createAssets(c, &category, ts.token)

	// test not exist hash
	u := fmt.Sprintf("/asset/metadata/4ebf54db04f335ff66bfc1fd982be62bf2300000?token=%s", ts.token)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, u, rr, http.StatusNotFound)

	res := rr.Result()
	defer res.Body.Close()

	// test not exist ID
	u = fmt.Sprintf("/asset/metadata/1000?token=%s", ts.token)
	req, err = http.NewRequest(http.MethodGet, u, nil)
	c.Assert(err, IsNil)

	rr = httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, u, rr, http.StatusNotFound)
}

func (ts *mainSuite) TestMetadataDelete(c *C) {
	ts.h.conf.UseMemdb = true
	c.Assert(ts.h.openMemDB(), IsNil)

	ts.h.previewDims = []types.Dimension{}

	category, err := loadCategory()
	c.Assert(err, IsNil)

	ts.createAssets(c, &category, ts.token)

	// bob should not delete alice's asset
	url := fmt.Sprintf("/asset/1?token=%s", ts.tokenBob)
	req, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, url, rr, http.StatusNotFound)

	result := &common.ErrResponse{}

	res := rr.Result()
	defer res.Body.Close()
	json.NewDecoder(res.Body).Decode(result)
	c.Assert(result.Text, Equals, common.ErrAssetNotExistForUser.Error())

	// insert metadata for the asset
	metas := []types.Metadata{
		{
			Category:     types.MetadataCategoryTag,
			SourceDevice: types.SourceDeviceAndroid,
			AssetID:      1,
			Name:         "name",
			Value:        "value",
			Model:        "tensorflow",
			Version:      1,
		},
	}

	ts.testInsertMetadatas(c, metas, false, true)

	// delete alice's asset
	ts.requestDelete(c, ts.token, &types.DeleteAssetItems{
		List: []types.DeleteAssetItem{{ID: "1"}},
	})

	// validate by reading from metadata table
	c.Assert(dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		metas, err = asset.GetMetadatas(ctx, tx, 1)
		return err
	}), IsNil)
	c.Assert(len(metas), Equals, 0)
}

func (ts *mainSuite) TestMetadataEmptyValue(c *C) {
	ts.initFakeAssets(c)

	ts.h.conf.UseMemdb = true
	c.Assert(ts.h.openMemDB(), IsNil)

	// without force, insert success, but no real data
	meta := types.Metadata{
		Category:     types.MetadataCategoryTag,
		SourceDevice: types.SourceDeviceAndroid,
		AssetID:      1,
		Name:         "mykey",
		Value:        "",
		Model:        "tensorflow",
		Version:      1,
	}
	ts.testInsertMetadatas(c, []types.Metadata{meta}, false, true)

	a := ts.getAssetMetadata(c, 1)
	c.Assert(a.Metadatas, IsNil)
	ts.testInsertMetadatas(c, []types.Metadata{meta}, true, true)

	a = ts.getAssetMetadata(c, 1)
	c.Assert(a.Metadatas, NotNil)
	c.Assert(len(*a.Metadatas), Equals, 1)
	(*a.Metadatas)[0].AssetID = 1
	(*a.Metadatas)[0].CreateTime = meta.CreateTime
	(*a.Metadatas)[0].LastModifiedTime = meta.LastModifiedTime
	c.Assert((*a.Metadatas)[0], DeepEquals, meta)
}
