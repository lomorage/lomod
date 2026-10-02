package handler

import (
	"context"
	"database/sql"
	"path"
	"sort"
	"strconv"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common/cmd"

	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	. "gopkg.in/check.v1"
)

var fakeGPS = 0.11

func (ts *mainSuite) initFakeAssets(c *C) {
	c.Assert(dbx.InQuery(ts.h.db, func(ctx context.Context, tx *sql.Tx) error {
		for i := 1; i <= 16; i++ {
			uid := ts.alice.ID
			eid := ext.JPEG
			if i%2 == 0 {
				uid = ts.bob.ID
			}
			if i%3 == 0 {
				eid = ext.MP4
			}
			_, err := asset.InsertAsset(ctx, tx, uid, 1, eid, &types.Asset{
				Hash:      strconv.Itoa(i),
				Latitude:  fakeGPS,
				Longitude: fakeGPS,
			}, 100, 0, time.Now())
			if err != nil {
				return err
			}
		}
		return nil
	}), IsNil)
}

func (ts *mainSuite) TestMigrationAssetSummary(c *C) {
	ts.initFakeAssets(c)
	ts.h.migrateAssetSize()

	result := ts.listSystemInfo(c, "/system?token="+ts.token)
	c.Assert(result.UserStatus, NotNil)
	alice, ok := result.UserStatus[ts.alice.Name]
	c.Assert(ok, Equals, true)
	c.Assert(alice.AssetSummary, DeepEquals, map[string]types.AssetSummary{
		ext.JPGString: {Count: 5, Size: 500},
		ext.MP4String: {Count: 3, Size: 300},
	})
	bob, ok := result.UserStatus[ts.bob.Name]
	c.Assert(ok, Equals, true)
	c.Assert(bob.AssetSummary, DeepEquals, map[string]types.AssetSummary{
		ext.JPGString: {Count: 6, Size: 600},
		ext.MP4String: {Count: 2, Size: 200},
	})
}

func (ts *mainSuite) TestMigrationMetadataPlaces(c *C) {
	ts.initFakeAssets(c)
	// insert gps and metadata
	c.Assert(cmd.Exec("bash", "../cmd/lomod/scripts/set_gps.sh", path.Join(ts.userdir, "assets.db")), IsNil)

	ts.h.migratePlaces()

	// CA need be normalized to California, and OR need be normalized to Oregon
	expectList := []types.GeoLocation{
		{City: "Suzhou", Level: types.MetadataGeoLevelCity, Country: "China", State: "Jiangsu"},
		{City: "Xian", Level: types.MetadataGeoLevelCity, Country: "China", State: "Shaanxi"},
		{City: "Belvedere Tiburon", Level: types.MetadataGeoLevelCity, Country: "United States", State: "California"},
		{City: "Macdoel", Level: types.MetadataGeoLevelCity, Country: "United States", State: "California"},
		{City: "Pacifica", Level: types.MetadataGeoLevelCity, Country: "United States", State: "California"},
		{City: "San Jose", Level: types.MetadataGeoLevelCity, Country: "United States", State: "California"},
		{City: "Tiburon", Level: types.MetadataGeoLevelCity, Country: "United States", State: "California"},
		{City: "Chiloquin", Level: types.MetadataGeoLevelCity, Country: "United States", State: "Oregon"},
		{City: "Fort Klamath", Level: types.MetadataGeoLevelCity, Country: "United States", State: "Oregon"},
		{Street: "梧桐街", Level: types.MetadataGeoLevelStreet, Country: "China", State: "Jiangsu", City: "Suzhou"},
		{Street: "Gaoxin Road", Level: types.MetadataGeoLevelStreet, Country: "China", State: "Shaanxi", City: "Xian"},
		{Street: "Forest Service Road 10", Level: types.MetadataGeoLevelStreet,
			Country: "United States", State: "California", City: "Macdoel"},
		{Street: "US-97", Level: types.MetadataGeoLevelStreet,
			Country: "United States", State: "California", City: "Macdoel"},
		{Street: "South Ridge Trail", Level: types.MetadataGeoLevelStreet,
			Country: "United States", State: "California", City: "Pacifica"},
		{Street: "Cobbert Dr", Level: types.MetadataGeoLevelStreet,
			Country: "United States", State: "California", City: "San Jose"},
		{Street: "Technology Dr", Level: types.MetadataGeoLevelStreet,
			Country: "United States", State: "California", City: "San Jose"},
		{Street: "Main St", Level: types.MetadataGeoLevelStreet,
			Country: "United States", State: "California", City: "Tiburon"},
		{Street: "Rim Dr", Level: types.MetadataGeoLevelStreet,
			Country: "United States", State: "Oregon", City: "Chiloquin"},
		{Street: "Rim Dr", Level: types.MetadataGeoLevelStreet,
			Country: "United States", State: "Oregon", City: "Fort Klamath"},
		{POI: "Old St. Hilary Open Space", Level: types.MetadataGeoLevelPOI,
			Country: "United States", State: "California", City: "Belvedere Tiburon"},
		{POI: "Lava Beds National Monument", Level: types.MetadataGeoLevelPOI,
			Country: "United States", State: "California", City: "Macdoel", Street: "Forest Service Road 10"},
		{POI: "Golden Gate National Recreation Area", Level: types.MetadataGeoLevelPOI,
			Country: "United States", State: "California", City: "Pacifica", Street: "South Ridge Trail"},
		{POI: "Crater Lake National Park", Level: types.MetadataGeoLevelPOI,
			Country: "United States", State: "Oregon", City: "Chiloquin", Street: "Rim Dr"},
		{POI: "Crater Lake National Park", Level: types.MetadataGeoLevelPOI,
			Country: "United States", State: "Oregon", City: "Fort Klamath", Street: "Rim Dr"},
	}

	result := ts.listMetadataPlaces(c)
	sort.Sort(types.ByLevel(result))

	updatePlace := "US-97"
	updatePlaceID := 1
	updatePlaceIndex := 0
	for i := 0; i < len(result); i++ {
		if result[i].Street == updatePlace {
			updatePlaceID = result[i].ID
			updatePlaceIndex = i
		}
		result[i].ID = 0
	}
	c.Assert(result, DeepEquals, expectList)

	// update zero longitude, or latitude wont' take effect
	for _, p := range []types.GeoLocation{
		{ID: updatePlaceID},
		{ID: updatePlaceID, Longitude: 1.0},
		{ID: updatePlaceID, Latitude: 2.0},
	} {
		ts.updateMetadataPlaces(c, p)

		result = ts.listMetadataPlaces(c)
		sort.Sort(types.ByLevel(result))
		for i := 0; i < len(result); i++ {
			if i == updatePlaceIndex {
				c.Assert(result[i].Longitude, Equals, p.Longitude)
				c.Assert(result[i].Latitude, Equals, p.Latitude)
			}
			result[i].ID = 0
			result[i].Longitude = 0
			result[i].Latitude = 0
		}
		c.Assert(result, DeepEquals, expectList, Commentf("update %v", p))
	}

	ts.updateMetadataPlaces(c, types.GeoLocation{ID: updatePlaceID, Longitude: 1.0, Latitude: 2.0})

	expectList = append(expectList[:updatePlaceIndex], expectList[updatePlaceIndex+1:]...)

	result = ts.listMetadataPlaces(c)
	sort.Sort(types.ByLevel(result))
	for i := 0; i < len(result); i++ {
		result[i].ID = 0
	}
	c.Assert(result, DeepEquals, expectList)
}

func (ts *mainSuite) TestMigrationMetadataPlaceAlbums(c *C) {
	ts.initFakeAssets(c)
	// insert gps and metadata
	c.Assert(cmd.Exec("bash", "../cmd/lomod/scripts/set_gps.sh", path.Join(ts.userdir, "assets.db")), IsNil)

	ts.h.migratePlaces()

	// validate geo album
	ts.validateGeoAlbum(c, map[int][]types.GeoLocation{
		1: {
			{Level: 1, Country: "United States"},
			{Level: 2, Country: "United States", State: "Oregon"},
			{Level: 4, Country: "United States", State: "Oregon", City: "Chiloquin"},
			{Level: 7, Country: "United States", State: "Oregon", City: "Chiloquin", Street: "Rim Dr"},
			{Level: 9, Country: "United States", State: "Oregon", City: "Chiloquin", Street: "Rim Dr", POI: "Crater Lake National Park"},
		},
		2: {
			{Level: 1, Country: "United States"},
			{Level: 2, Country: "United States", State: "California"},
			{Level: 4, Country: "United States", State: "California", City: "Macdoel"},
			{Level: 7, Country: "United States", State: "California", City: "Macdoel", Street: "US-97"},
		},
		3: {
			{Level: 1, Country: "United States"},
			{Level: 2, Country: "United States", State: "California"},
			{Level: 4, Country: "United States", State: "California", City: "Macdoel"},
			{Level: 7, Country: "United States", State: "California", City: "Macdoel", Street: "Forest Service Road 10"},
			{Level: 9, Country: "United States", State: "California", City: "Macdoel", Street: "Forest Service Road 10", POI: "Lava Beds National Monument"},
		},
		4: {
			{Level: 1, Country: "United States"},
			{Level: 2, Country: "United States", State: "Oregon"},
			{Level: 4, Country: "United States", State: "Oregon", City: "Fort Klamath"},
			{Level: 7, Country: "United States", State: "Oregon", City: "Fort Klamath", Street: "Rim Dr"},
			{Level: 9, Country: "United States", State: "Oregon", City: "Fort Klamath", Street: "Rim Dr", POI: "Crater Lake National Park"},
		},
		5: {
			{Level: 1, Country: "United States"},
			{Level: 2, Country: "United States", State: "Oregon"},
			{Level: 4, Country: "United States", State: "Oregon", City: "Chiloquin"},
			{Level: 7, Country: "United States", State: "Oregon", City: "Chiloquin", Street: "Rim Dr"},
			{Level: 9, Country: "United States", State: "Oregon", City: "Chiloquin", Street: "Rim Dr", POI: "Crater Lake National Park"},
		},
		6: {
			{Level: 1, Country: "China"},
			{Level: 2, Country: "China", State: "Shaanxi"},
			{Level: 4, Country: "China", State: "Shaanxi", City: "Xian"},
			{Level: 7, Country: "China", State: "Shaanxi", City: "Xian", Street: "Gaoxin Road"},
		},
		7: {
			{Level: 1, Country: "United States"},
			{Level: 2, Country: "United States", State: "California"},
			{Level: 4, Country: "United States", State: "California", City: "Macdoel"},
			{Level: 7, Country: "United States", State: "California", City: "Macdoel", Street: "Forest Service Road 10"},
			{Level: 9, Country: "United States", State: "California", City: "Macdoel", Street: "Forest Service Road 10", POI: "Lava Beds National Monument"},
		},
		8: {
			{Level: 1, Country: "United States"},
			{Level: 2, Country: "United States", State: "California"},
			{Level: 4, Country: "United States", State: "California", City: "San Jose"},
			{Level: 7, Country: "United States", State: "California", City: "San Jose", Street: "Technology Dr"},
		},
		9: {
			{Level: 1, Country: "United States"},
			{Level: 2, Country: "United States", State: "California"},
			{Level: 4, Country: "United States", State: "California", City: "San Jose"},
			{Level: 7, Country: "United States", State: "California", City: "San Jose", Street: "Cobbert Dr"},
		},
		10: {
			{Level: 1, Country: "China"},
			{Level: 2, Country: "China", State: "Jiangsu"},
			{Level: 4, Country: "China", State: "Jiangsu", City: "Suzhou"},
			{Level: 7, Country: "China", State: "Jiangsu", City: "Suzhou", Street: "梧桐街"},
		},
		11: {
			{Level: 1, Country: "United States"},
			{Level: 2, Country: "United States", State: "California"},
			{Level: 4, Country: "United States", State: "California", City: "Belvedere Tiburon"},
			{Level: 9, Country: "United States", State: "California", City: "Belvedere Tiburon", POI: "Old St. Hilary Open Space"},
		},
		12: {
			{Level: 1, Country: "United States"},
			{Level: 2, Country: "United States", State: "California"},
			{Level: 4, Country: "United States", State: "California", City: "Macdoel"},
			{Level: 7, Country: "United States", State: "California", City: "Macdoel", Street: "US-97"},
		},
		13: {
			{Level: 1, Country: "United States"},
			{Level: 2, Country: "United States", State: "California"},
			{Level: 4, Country: "United States", State: "California", City: "Pacifica"},
			{Level: 7, Country: "United States", State: "California", City: "Pacifica", Street: "South Ridge Trail"},
			{Level: 9, Country: "United States", State: "California", City: "Pacifica", Street: "South Ridge Trail", POI: "Golden Gate National Recreation Area"},
		},
		14: {
			{Level: 1, Country: "United States"},
			{Level: 2, Country: "United States", State: "California"},
			{Level: 4, Country: "United States", State: "California", City: "Tiburon"},
			{Level: 7, Country: "United States", State: "California", City: "Tiburon", Street: "Main St"},
		},
		15: {
			{Level: 1, Country: "United States"},
			{Level: 2, Country: "United States", State: "California"},
			{Level: 4, Country: "United States", State: "California", City: "Tiburon"},
			{Level: 7, Country: "United States", State: "California", City: "Tiburon", Street: "Main St"},
		},
		16: {
			{Level: 1, Country: "United States"},
			{Level: 2, Country: "United States", State: "California"},
			{Level: 4, Country: "United States", State: "California", City: "Tiburon"},
			{Level: 7, Country: "United States", State: "California", City: "Tiburon", Street: "Main St"},
		},
	})
}

func (ts *mainSuite) TestMigrationMetadataThingAlbums(c *C) {
	ts.initFakeAssets(c)
	// insert gps and metadata
	c.Assert(cmd.Exec("bash", "../cmd/lomod/scripts/set_scene.sh", path.Join(ts.userdir, "assets.db")), IsNil)

	ts.h.migrateScenes()

	labels := ts.listLabels(c)
	c.Assert(len(labels), Equals, 399)
}
