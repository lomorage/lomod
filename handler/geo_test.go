package handler

import (
	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	. "gopkg.in/check.v1"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
)

var (
	normalizedState = "California"
	testMetaCountryEn = types.Metadata{
		Category:     types.MetadataCategoryGeo,
		SourceDevice: types.SourceDeviceIos,
		AssetID:      1,
		Name:         "ios.geo.country.en_US",
		Value:        "United States",
		Model:        "tensorflow",
		Version:      1,
	}
	testMetaStateEn = types.Metadata{
		Category:     types.MetadataCategoryGeo,
		SourceDevice: types.SourceDeviceIos,
		AssetID:      1,
		Name:         "ios.geo.state.en_US",
		Value:        "CA",
		Model:        "tensorflow",
		Version:      1,
	}
	testMetaDistrictEn = types.Metadata{
		Category:     types.MetadataCategoryGeo,
		SourceDevice: types.SourceDeviceIos,
		AssetID:      1,
		Name:         "ios.geo.district.en_US",
		Value:        "Santa Clara County",
		Model:        "tensorflow",
		Version:      1,
	}
	testMetaCityEn = types.Metadata{
			Category:     types.MetadataCategoryGeo,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      1,
			Name:         "ios.geo.city.en_US",
			Value:        "San Jose",
			Model:        "tensorflow",
			Version:      1,
	}
	testMetaCountryZh, testMetaStateZh, testMetaDistrictZh, testMetaCityZh types.Metadata
	)

func init() {
	testMetaCountryZh = testMetaCountryEn
	testMetaCountryZh.Name = "ios.geo.country.zh"
	testMetaCountryZh.Value = "美国"
	testMetaStateZh = testMetaStateEn
	testMetaStateZh.Name = "ios.geo.state.zh"
	testMetaStateZh.Value = "加利福尼亚"
	testMetaDistrictZh = testMetaDistrictEn
	testMetaDistrictZh.Name = "ios.geo.district.zh"
	testMetaDistrictZh.Value = "圣巴巴拉"
	testMetaCityZh = testMetaCityEn
	testMetaCityZh.Name = "ios.geo.city.zh"
	testMetaCityZh.Value = "圣何塞"
}

func (ts *mainSuite) listMetadataPlaces(c *C) []types.GeoLocation {
	u := "/assets/metadata/places?token=" + ts.token
	req, err := http.NewRequest("GET", u, nil)
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, u, rr, http.StatusOK)

	res := rr.Result()
	defer res.Body.Close()

	places := []types.GeoLocation{}
	c.Assert(json.NewDecoder(res.Body).Decode(&places), IsNil)
	return places
}

func (ts *mainSuite) updateMetadataPlaces(c *C, place types.GeoLocation){
	u := "/assets/metadata/places/" + strconv.Itoa(place.ID) + "?token=" + ts.token
	content, err := json.Marshal(place)
	c.Assert(err, IsNil)
	req, err := http.NewRequest("PUT", u, bytes.NewBuffer(content))
	c.Assert(err, IsNil)

	rr := httptest.NewRecorder()
	ts.h.CreateRouter().ServeHTTP(rr, req)
	validateStatusCode(c, u, rr, http.StatusOK)

	res := rr.Result()
	defer res.Body.Close()
}

func (ts *mainSuite) TestMetadataGeoNoCountryState(c *C) {
	// update city, state with both English, and Chinese
	ts.initFakeAssets(c)
	metas := []types.Metadata{testMetaCityEn}

	c.Assert(ts.h.openMemDB(), IsNil)
	content := ts.testInsertMetadatas(c, metas, false, false)
	c.Assert(strings.Contains(string(content), common.ErrGeoNoCountry.Error()), Equals, true)

	metas = append(metas, testMetaCountryEn)
	content = ts.testInsertMetadatas(c, metas, false, false)
	c.Assert(strings.Contains(string(content), common.ErrGeoNoState.Error()), Equals, true)
}

func (ts *mainSuite) TestMetadataGeoWrongCountryState(c *C) {
	// update city, state with both English, and Chinese
	ts.initFakeAssets(c)
	country := "United----State"
	state := "CAJ"
	city := "SanJose"
	metas := []types.Metadata{
		{
			Category:     types.MetadataCategoryGeo,
			SourceDevice: types.SourceDeviceAndroid,
			AssetID:      1,
			Name:         "ios.geo.country.en_US",
			Value:        country,
			Model:        "tensorflow",
			Version:      1,
		},
		{
			Category:     types.MetadataCategoryGeo,
			SourceDevice: types.SourceDeviceAndroid,
			AssetID:      1,
			Name:         "ios.geo.state.en_US",
			Value:        state,
			Model:        "tensorflow",
			Version:      1,
		},
		{
			Category:     types.MetadataCategoryGeo,
			SourceDevice: types.SourceDeviceAndroid,
			AssetID:      1,
			Name:         "ios.geo.city.en_US",
			Value:        city,
			Model:        "tensorflow",
			Version:      1,
		},
	}

	c.Assert(ts.h.openMemDB(), IsNil)
	ts.testInsertMetadatas(c, metas, false, true)
	places := ts.listMetadataPlaces(c)
	sort.Slice(places, func(i, j int) bool {return places[i].City < places[j].City})
	c.Assert(places, DeepEquals, []types.GeoLocation{
		{ID: 1, Country: country, Level: types.MetadataGeoLevelCountry},
		{ID: 2, Country: country, State: state, Level: types.MetadataGeoLevelState},
		{ID: 3, Country: country, State: state, City: city, Level: types.MetadataGeoLevelCity},
	})
}

// update city, state with both English and Chinese, and english is in front of Chinese
func (ts *mainSuite) TestMetadataGeoLangBasicEnZh(c *C) {
	ts.initFakeAssets(c)
	metas := []types.Metadata{testMetaCountryEn, testMetaStateEn, testMetaCityEn, testMetaStateZh, testMetaCityZh}

	c.Assert(ts.h.openMemDB(), IsNil)
	ts.testInsertMetadatas(c, metas, false, true)

	// country and state should be normalized value
	places := ts.listMetadataPlaces(c)
	sort.Slice(places, func(i, j int) bool {return places[i].City < places[j].City})
	c.Assert(places, DeepEquals, []types.GeoLocation{
		{ID: 3, Country: testMetaCountryEn.Value, State: normalizedState, City: testMetaCityEn.Value, Level: types.MetadataGeoLevelCity},
	})

	ts.validateGeoLang(c, []types.MetadataForeignLang{
		{Lang: "zh", NameEn: normalizedState, Name: testMetaStateZh.Value},
		{Lang: "zh", NameEn: testMetaCityEn.Value, Name: testMetaCityZh.Value},
	})
}

// update city, state with both English and Chinese, and english is after Chinese
func (ts *mainSuite) TestMetadataGeoLangBasicZhEn(c *C) {
	ts.initFakeAssets(c)
	metas := []types.Metadata{testMetaStateZh, testMetaCityZh, testMetaCountryEn, testMetaStateEn, testMetaCityEn}

	c.Assert(ts.h.openMemDB(), IsNil)
	ts.testInsertMetadatas(c, metas, false, true)

	// country and state should be normalized value
	places := ts.listMetadataPlaces(c)
	sort.Slice(places, func(i, j int) bool {return places[i].City < places[j].City})
	c.Assert(places, DeepEquals, []types.GeoLocation{
		{ID: 3, Country: testMetaCountryEn.Value, State: normalizedState, City: testMetaCityEn.Value, Level: types.MetadataGeoLevelCity},
	})

	ts.validateGeoLang(c, []types.MetadataForeignLang{
		{Lang: "zh", NameEn: normalizedState, Name: testMetaStateZh.Value},
		{Lang: "zh", NameEn: testMetaCityEn.Value, Name: testMetaCityZh.Value},
	})
}

// TODO: lomod should expose API to test instead of reading db
func (ts *mainSuite) validateGeoLang(c *C, expPlaces []types.MetadataForeignLang) {
	obtPlaces := []types.MetadataForeignLang{}
	err := dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "select lang, name, name_en from album_geo_lang order by name_en")
		if err != nil {
			return err
		}
		for rows.Next() {
			place := types.MetadataForeignLang{}
			err = rows.Scan(&place.Lang, &place.Name, &place.NameEn)
			if err != nil {
				return err
			}
			obtPlaces = append(obtPlaces, place)
		}
		return rows.Err()
	})
	c.Assert(err, IsNil)
	c.Assert(obtPlaces, DeepEquals, expPlaces)
}

// TODO: lomod should expose API to test instead of reading db
func (ts *mainSuite) validateGeoAlbum(c *C, expAssetPlacess map[int][]types.GeoLocation) {
	obtAssetPlacess := map[int][]types.GeoLocation{}
	err := dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			"select asset_id, level, country, state, district, city, locality, neighborhood, street, substreet, poi " +
			"from asset_album_geo as aa inner join album_geo as ag on aa.album_id = ag.id " +
			"order by asset_id, level, country, state, district, city, locality, neighborhood, street, substreet, poi")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				assetID int
				place types.GeoLocation
			)
			err = rows.Scan(&assetID, &place.Level, &place.Country, &place.State, &place.District, &place.City, &place.Locality,
				&place.Neighborhood, &place.Street, &place.SubStreet, &place.POI)
			if err != nil {
				return err
			}
			places, ok := obtAssetPlacess[assetID]
			if !ok {
				places = []types.GeoLocation{}
			}
			places = append(places, place)
			obtAssetPlacess[assetID] = places
		}
		return rows.Err()
	})
	c.Assert(err, IsNil)
	c.Assert(obtAssetPlacess, DeepEquals, expAssetPlacess)
}

func (ts *mainSuite) TestMetadataGeoLangUpdate(c *C) {
	// 1st update city, district with English, 2nd update with Chinese
	ts.initFakeAssets(c)
	metas := []types.Metadata{testMetaCountryEn, testMetaStateEn, testMetaDistrictEn, testMetaCityEn}

	c.Assert(ts.h.openMemDB(), IsNil)
	ts.testInsertMetadatas(c, metas, false, true)

	// no places because name is not qualified
	places := ts.listMetadataPlaces(c)
	sort.Slice(places, func(i, j int) bool {return places[i].City < places[j].City})
	c.Assert(places, DeepEquals, []types.GeoLocation{
		{ID: 3, Country: testMetaCountryEn.Value, State: normalizedState, District: testMetaDistrictEn.Value,
			Level: types.MetadataGeoLevelDistrict},
		{ID: 4, Country: testMetaCountryEn.Value, State: normalizedState, District: testMetaDistrictEn.Value, City: testMetaCityEn.Value,
			Level: types.MetadataGeoLevelCity},
	})
	ts.validateGeoLang(c, []types.MetadataForeignLang{})

	metas = []types.Metadata{testMetaCountryZh, testMetaStateZh, testMetaDistrictZh, testMetaCityZh}

	c.Assert(ts.h.openMemDB(), IsNil)
	ts.testInsertMetadatas(c, metas, false, true)

	places = ts.listMetadataPlaces(c)
	sort.Slice(places, func(i, j int) bool {return places[i].City < places[j].City})
	c.Assert(places, DeepEquals, []types.GeoLocation{
		{ID: 3, Country: testMetaCountryEn.Value, State: normalizedState, District: testMetaDistrictEn.Value, Level: types.MetadataGeoLevelDistrict},
		{ID: 4, Country: testMetaCountryEn.Value, State: normalizedState, District: testMetaDistrictEn.Value, City: testMetaCityEn.Value,
			Level: types.MetadataGeoLevelCity},
	})

	ts.validateGeoLang(c, []types.MetadataForeignLang{
		{Lang: "zh", NameEn: normalizedState, Name: testMetaStateZh.Value},
		{Lang: "zh", NameEn: testMetaCityEn.Value, Name: testMetaCityZh.Value},
		{Lang: "zh", NameEn: testMetaDistrictEn.Value, Name: testMetaDistrictZh.Value},
		{Lang: "zh", NameEn: testMetaCountryEn.Value, Name: testMetaCountryZh.Value},
	})
}

func (ts *mainSuite) TestMetadataGeoMix(c *C) {
	// 1st update city with both English, and Chinese, and district in English only
	// 2nd update district with Chinese
	ts.initFakeAssets(c)
	metas := []types.Metadata{testMetaCountryEn, testMetaStateEn, testMetaCityEn, testMetaCityZh}

	c.Assert(ts.h.openMemDB(), IsNil)
	ts.testInsertMetadatas(c, metas, false, true)

	places := ts.listMetadataPlaces(c)
	sort.Slice(places, func(i, j int) bool {return places[i].City < places[j].City})
	c.Assert(places, DeepEquals, []types.GeoLocation{
		{ID: 3, Country: testMetaCountryEn.Value, State: normalizedState, City: testMetaCityEn.Value, Level: types.MetadataGeoLevelCity},
	})
	ts.validateGeoLang(c, []types.MetadataForeignLang{
		{Lang: "zh", NameEn: testMetaCityEn.Value, Name: testMetaCityZh.Value},
	})

	metas = []types.Metadata{testMetaCountryZh, testMetaStateZh}

	c.Assert(ts.h.openMemDB(), IsNil)
	ts.testInsertMetadatas(c, metas, false, true)

	places = ts.listMetadataPlaces(c)
	sort.Slice(places, func(i, j int) bool {return places[i].City < places[j].City})
	c.Assert(places, DeepEquals, []types.GeoLocation{
		{ID: 3, Country: testMetaCountryEn.Value, State: normalizedState, City: testMetaCityEn.Value, Level: types.MetadataGeoLevelCity},
	})

	ts.validateGeoLang(c, []types.MetadataForeignLang{
		{Lang: "zh", NameEn: normalizedState, Name: testMetaStateZh.Value},
		{Lang: "zh", NameEn: testMetaCityEn.Value, Name: testMetaCityZh.Value},
		{Lang: "zh", NameEn: testMetaCountryEn.Value, Name: testMetaCountryZh.Value},
	})
}

func (ts *mainSuite) TestMetadataGeoLangDoubleUpdate(c *C) {
	// update city twice and data should remain same
	ts.initFakeAssets(c)
	metas := []types.Metadata{testMetaCountryEn, testMetaStateEn, testMetaCityEn, testMetaCityZh}

	c.Assert(ts.h.openMemDB(), IsNil)
	ts.testInsertMetadatas(c, metas, false, true)

	// no places because name is not qualified
	places := ts.listMetadataPlaces(c)
	sort.Slice(places, func(i, j int) bool {return places[i].City < places[j].City})
	c.Assert(places, DeepEquals, []types.GeoLocation{
		{ID: 3, Country: testMetaCountryEn.Value, State: normalizedState, City: testMetaCityEn.Value, Level: types.MetadataGeoLevelCity},
	})

	ts.validateGeoLang(c, []types.MetadataForeignLang{
		{Lang: "zh", NameEn: testMetaCityEn.Value, Name: testMetaCityZh.Value},
	})

	// update again and data should not change
	ts.testInsertMetadatas(c, metas, false, true)

	// no places because name is not qualified
	places = ts.listMetadataPlaces(c)
	sort.Slice(places, func(i, j int) bool {return places[i].City < places[j].City})
	c.Assert(places, DeepEquals, []types.GeoLocation{
		{ID: 3, Country: testMetaCountryEn.Value, State: normalizedState, City: testMetaCityEn.Value, Level: types.MetadataGeoLevelCity},
	})

	ts.validateGeoLang(c, []types.MetadataForeignLang{
		{Lang: "zh", NameEn: testMetaCityEn.Value, Name: testMetaCityZh.Value},
	})
}

func (ts *mainSuite) TestMetadataGeoLangNoEng(c *C) {
	// update city without english version should fail
	ts.initFakeAssets(c)
	metas := []types.Metadata{testMetaCountryEn, testMetaStateEn, testMetaCityZh}

	c.Assert(ts.h.openMemDB(), IsNil)
	content := ts.testInsertMetadatas(c, metas, false, false)
	c.Assert(strings.Contains(string(content), "metadata city - 圣何塞 has not english version yet"), Equals, true)

	// no places because name is not qualified
	places := ts.listMetadataPlaces(c)
	c.Assert(places, DeepEquals, []types.GeoLocation{})

	ts.validateGeoLang(c, []types.MetadataForeignLang{})
}

func (ts *mainSuite) TestMetadataGeoAlbum(c *C) {
	ts.initFakeAssets(c)
	// insert both chinese and english metadata, and album should only has english geo location
	metas := []types.Metadata{testMetaCountryEn, testMetaStateEn, testMetaDistrictEn, testMetaCityEn,
		testMetaCountryZh, testMetaStateZh, testMetaDistrictZh, testMetaCityZh}

	c.Assert(ts.h.openMemDB(), IsNil)
	ts.testInsertMetadatas(c, metas, false, false)
	ts.validateGeoAlbum(c, map[int][]types.GeoLocation{
		1: {
			{Level: 1, Country: testMetaCountryEn.Value},
			{Level: 2, Country: testMetaCountryEn.Value, State: normalizedState},
			{Level: 3, Country: testMetaCountryEn.Value, State: normalizedState, District: testMetaDistrictEn.Value},
			{Level: 4, Country: testMetaCountryEn.Value, State: normalizedState, District: testMetaDistrictEn.Value,
				City: testMetaCityEn.Value},
		},
	})
}
