package asset

import (
	"context"
	"database/sql"
	"io/ioutil"
	"os"
	. "testing"
	"time"

	"strconv"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/migrator"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/migrations/sqls/lomod"
	. "gopkg.in/check.v1"
)

// limit is the page size the tests query with.
const limit = 100

var testAssetCount = 2 * limit

// getAssetIDsByMetadata runs GetAssetsByMetadata for one page of asset IDs.
func getAssetIDsByMetadata(ctx context.Context, tx *sql.Tx, uid int, queries map[string][]string, page int) ([]int, int, error) {
	res, total, err := GetAssetsByMetadata(ctx, tx, uid, queries, page, limit)
	if err != nil {
		return nil, 0, err
	}
	return res.([]int), total, nil
}

type metadataSuite struct {
	db       *sql.DB
	dbfile   string
	assetIDs []int
}

var _ = Suite(&metadataSuite{})

func TestMetadataSuite(t *T) {
	TestingT(t)
}

func (hs *metadataSuite) SetUpTest(c *C) {
	f, err := ioutil.TempFile("", "")
	c.Assert(err, IsNil)
	c.Assert(f.Close(), IsNil)

	hs.dbfile = f.Name()

	c.Assert(migrator.StartLomod(hs.dbfile, "", lomod.SchemaStatements, common.DefaultFolderPermission), IsNil)

	// insert 200 asset IDs for testing
	hs.db, err = sql.Open("sqlite3", hs.dbfile)
	c.Assert(err, IsNil)

	// #1-2 is for user 0
	// #3-200 is for user 1
	hs.assetIDs = make([]int, testAssetCount)
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		for i := 0; i < 2; i++ {
			id, err := InsertAsset(ctx, tx, 0, 0, 0, &types.Asset{Hash: strconv.Itoa(i)}, 0, 0, time.Now())
			if err != nil {
				return err
			}
			hs.assetIDs[i] = int(id)
		}
		for i := 2; i < testAssetCount; i++ {
			id, err := InsertAsset(ctx, tx, 1, 0, 0, &types.Asset{Hash: strconv.Itoa(i)}, 0, 0, time.Now())
			if err != nil {
				return err
			}
			hs.assetIDs[i] = int(id)
		}
		return nil
	}), IsNil)
}

func (hs *metadataSuite) TearDownTest(c *C) {
	c.Assert(hs.db.Close(), IsNil)
	c.Assert(os.Remove(hs.dbfile), IsNil)
}

func (hs *metadataSuite) TestMetadataInsertTask(c *C) {
	testName := "name1"
	metas := []types.Metadata{
		{
			Category:     types.MetadataCategoryGeo,
			SourceDevice: types.SourceDeviceAndroid,
			AssetID:      hs.assetIDs[2],
			Name:         testName,
			Value:        "value1",
			Model:        "tensorflow",
			Version:      1,
		},
		{
			Category:     types.MetadataCategoryGeo,
			SourceDevice: types.SourceDeviceAndroid,
			AssetID:      hs.assetIDs[3],
			Name:         testName,
			Value:        "value2",
			Model:        "tensorflow",
			Version:      1,
		},
	}
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		for _, meta := range metas {
			if err := InsertOrUpdateMetadata(ctx, tx, meta); err != nil {
				return err
			}
		}
		return nil
	}), IsNil)

	var (
		total    int
		assetIDs []int
	)

	// search by missing category only
	err := dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 0, map[string][]string{
			QueryMetadataCategoryMiss: {string(types.MetadataCategoryGeo)}}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, 2)
	c.Assert(len(assetIDs), Equals, 2)

	for i := 0; i < 2; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[1-i])
	}

	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategoryMiss: {string(types.MetadataCategoryGeo)}}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, testAssetCount-4)
	c.Assert(len(assetIDs), Equals, limit)

	for i := 0; i < limit; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[testAssetCount-1-i])
	}

	// search by missing category + device
	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategoryMiss: {string(types.MetadataCategoryGeo)},
			QueryMetadataSourceDevice: {string(types.SourceDeviceAndroid)},
		}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, testAssetCount-4)
	c.Assert(len(assetIDs), Equals, limit)

	for i := 0; i < limit; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[testAssetCount-1-i])
	}

	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategoryMiss: {string(types.MetadataCategoryGeo)},
			QueryMetadataSourceDevice: {string(types.SourceDeviceIos)},
		}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, testAssetCount-2)
	c.Assert(len(assetIDs), Equals, limit)

	for i := 0; i < limit; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[testAssetCount-1-i])
	}

	// search by version only
	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataVersionLess: {"2"}}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, 2)
	c.Assert(len(assetIDs), Equals, 2)

	for i := 0; i < 2; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[3-i])
	}

	// search by device
	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataSourceDevice: {string(types.SourceDeviceAndroid)}}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, 2)
	c.Assert(len(assetIDs), Equals, 2)

	for i := 0; i < 2; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[3-i])
	}

	// search by miss name
	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataNameMiss: {testName}}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, testAssetCount-4)
	c.Assert(len(assetIDs), Equals, limit)

	for i := 0; i < limit; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[testAssetCount-1-i])
	}

	// search by version + device
	// 2 for android, 0 for ios
	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataVersionLess: {"2"}, QueryMetadataSourceDevice: {string(types.SourceDeviceAndroid)}}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, 2)
	c.Assert(len(assetIDs), Equals, 2)

	for i := 0; i < 2; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[3-i])
	}

	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataVersionLess: {"2"}, QueryMetadataSourceDevice: {string(types.SourceDeviceIos)}}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, 0)
	c.Assert(len(assetIDs), Equals, 0)

	// search by version + name
	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataVersionLess: {"2"}, QueryMetadataName: {testName}}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, 2)
	c.Assert(len(assetIDs), Equals, 2)

	for i := 0; i < 2; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[3-i])
	}

	// search by name + source device
	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataNameMiss:     {testName},
			QueryMetadataSourceDevice: {string(types.SourceDeviceAndroid)}}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, testAssetCount-4)
	c.Assert(len(assetIDs), Equals, limit)

	for i := 0; i < limit; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[testAssetCount-1-i])
	}

	// update metadata for these assets
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		for _, id := range assetIDs {
			newName := strconv.Itoa(id) + "-name1"
			newValue := "value1"
			if err := InsertOrUpdateMetadata(ctx, tx, types.Metadata{
				Category:     types.MetadataCategoryGeo,
				SourceDevice: types.SourceDeviceIos,
				AssetID:      id,
				Name:         newName,
				Value:        newValue,
				Model:        "tensorflow",
				Version:      2,
			}); err != nil {
				return err
			}
		}
		return nil
	}), IsNil)

	// get another task, which should return 96 for geo category, and 0 for face
	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategoryMiss: {string(types.MetadataCategoryGeo)}}, 0)
		return err
	})
	c.Assert(err, IsNil)
	left := limit - 4
	c.Assert(total, Equals, left)
	c.Assert(len(assetIDs), Equals, left)

	for i := 0; i < left-1; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[limit-1-i])
	}

	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategoryMiss: {string(types.MetadataCategoryFace)}}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, testAssetCount-2)
	c.Assert(len(assetIDs), Equals, limit)

	for i := 0; i < limit; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[testAssetCount-1-i])
	}

	// update metadata with face metadata, then search geo, it should still have 96 left
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		for _, id := range hs.assetIDs {
			if err := InsertOrUpdateMetadata(ctx, tx, types.Metadata{
				Category:     types.MetadataCategoryFace,
				SourceDevice: types.SourceDeviceAndroid,
				AssetID:      id,
				Name:         testName,
				Value:        "value1",
				Model:        "tensorflow",
				Version:      1,
			}); err != nil {
				return err
			}
		}
		return nil
	}), IsNil)

	// android should return 0, ios should still return all
	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategoryMiss: {string(types.MetadataCategoryFace)}}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, 0)
	c.Assert(len(assetIDs), Equals, 0)

	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategoryMiss: {string(types.MetadataCategoryFace)},
			QueryMetadataSourceDevice: {string(types.SourceDeviceAndroid)}}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, 0)
	c.Assert(len(assetIDs), Equals, 0)

	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategoryMiss: {string(types.MetadataCategoryFace)},
			QueryMetadataSourceDevice: {string(types.SourceDeviceIos)}}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, testAssetCount-2)
	c.Assert(len(assetIDs), Equals, limit)
	for i := 0; i < limit-1; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[testAssetCount-1-i])
	}

	// get geo task again
	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategoryMiss: {string(types.MetadataCategoryGeo)}}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, left)
	c.Assert(len(assetIDs), Equals, left)

	for i := 0; i < left-1; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[limit-1-i])
	}

	// search by version
	// should be the first 2 + 96
	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory:    {string(types.MetadataCategoryGeo)},
			QueryMetadataVersionLess: {"2"}}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, 2)
	c.Assert(len(assetIDs), Equals, 2)

	for i := 0; i < 2; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[3-i])
	}

	// search by version + device
	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory:     {string(types.MetadataCategoryGeo)},
			QueryMetadataVersionLess:  {"2"},
			QueryMetadataSourceDevice: {string(types.SourceDeviceIos)}}, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(total, Equals, 0)
	c.Assert(len(assetIDs), Equals, 0)
}

func (hs *metadataSuite) TestMetadataCategoryList(c *C) {
	var categories []string
	for i := 0; i < 2; i++ {
		err := dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
			var err error
			categories, err = ListMetadataCategories(ctx, tx, i)
			return err
		})
		c.Assert(err, IsNil)
		c.Assert(len(categories), Equals, 0)
	}

	uniqueGeoNames := map[string]struct{}{"name1": {}}
	uniqueGeoValues := map[string]map[string]struct{}{"name1": {"value1": {}, "value2": {}}}
	metas := []types.Metadata{
		{
			Category:     types.MetadataCategoryGeo,
			SourceDevice: types.SourceDeviceAndroid,
			AssetID:      hs.assetIDs[2],
			Name:         "name1",
			Value:        "value1",
			Model:        "tensorflow",
			Version:      1,
		},
		{
			Category:     types.MetadataCategoryFace,
			SourceDevice: types.SourceDeviceAndroid,
			AssetID:      hs.assetIDs[2],
			Name:         "name1",
			Value:        "value1",
			Model:        "tensorflow",
			Version:      1,
		},
		{
			Category:     types.MetadataCategoryGeo,
			SourceDevice: types.SourceDeviceAndroid,
			AssetID:      hs.assetIDs[3],
			Name:         "name1",
			Value:        "value2",
			Model:        "tensorflow",
			Version:      1,
		},
	}
	for i := 4; i < 104; i++ {
		newName := strconv.Itoa(i) + "-name1"
		newValue := "value1"
		uniqueGeoNames[newName] = struct{}{}
		uniqueGeoValues[newName] = map[string]struct{}{newValue: {}}
		metas = append(metas, types.Metadata{
			Category:     types.MetadataCategoryGeo,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      hs.assetIDs[i],
			Name:         newName,
			Value:        newValue,
			Model:        "tensorflow",
			Version:      2,
		})
	}
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		for _, meta := range metas {
			if err := InsertOrUpdateMetadata(ctx, tx, meta); err != nil {
				return err
			}
		}
		return nil
	}), IsNil)

	// user 0 still not category
	err := dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		categories, err = ListMetadataCategories(ctx, tx, 0)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(len(categories), Equals, 0)

	var names []string
	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		names, err = ListMetadataNames(ctx, tx, 0, types.MetadataCategoryGeo)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(len(names), Equals, 0)

	for n := range uniqueGeoNames {
		err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
			names, err = ListMetadataValues(ctx, tx, 0, types.MetadataCategoryGeo, n)
			return err
		})
		c.Assert(err, IsNil)
		c.Assert(len(names), Equals, 0)
	}

	// user 1 should have more category
	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		categories, err = ListMetadataCategories(ctx, tx, 1)
		return err
	})
	c.Assert(err, IsNil)
	categoryMap := map[string]struct{}{
		types.MetadataCategoryGeo:  {},
		types.MetadataCategoryFace: {},
	}
	c.Assert(len(categories), Equals, len(categoryMap))
	for _, cat := range categories {
		_, ok := categoryMap[cat]
		c.Assert(ok, Equals, true)
	}

	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		names, err = ListMetadataNames(ctx, tx, 1, types.MetadataCategoryGeo)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(len(names), Equals, len(uniqueGeoNames))
	for _, n := range names {
		_, ok := uniqueGeoNames[n]
		c.Assert(ok, Equals, true)
	}
	// search and compare all values now
	for n := range uniqueGeoNames {
		err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
			names, err = ListMetadataValues(ctx, tx, 1, types.MetadataCategoryGeo, n)
			return err
		})
		c.Assert(err, IsNil)
		c.Assert(len(names), Equals, len(uniqueGeoValues[n]))
		for _, v := range names {
			_, ok := uniqueGeoValues[n][v]
			c.Assert(ok, Equals, true)
		}
	}

	// all face category should have only 1 name and value
	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		names, err = ListMetadataNames(ctx, tx, 1, types.MetadataCategoryFace)
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(len(names), Equals, 1)
	c.Assert(names[0], Equals, "name1")

	// all face category should have only 1 name and value
	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		names, err = ListMetadataValues(ctx, tx, 1, types.MetadataCategoryFace, "name1")
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(len(names), Equals, 1)
	c.Assert(names[0], Equals, "value1")
}

func (hs *metadataSuite) TestMetadataAssetSearch(c *C) {
	// assume all assets have metadata key, value -> city, sanjose
	testName := "city"
	testValue := "sanjose"
	metas := make([]types.Metadata, testAssetCount)
	for i := 0; i < testAssetCount; i++ {
		metas[i] = types.Metadata{
			Category:     types.MetadataCategoryGeo,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      hs.assetIDs[i],
			Name:         testName,
			Value:        testValue,
			Model:        "tensorflow",
			Version:      2,
		}
	}
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		for _, meta := range metas {
			if err := InsertOrUpdateMetadata(ctx, tx, meta); err != nil {
				return err
			}
		}
		return nil
	}), IsNil)

	var (
		total    int
		assetIDs []int
	)

	// user 0
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 0, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV:       {testName + common.QueryDelimiterKV + testValue}}, 0)
		return err
	}), IsNil)
	c.Assert(total, Equals, 2)
	c.Assert(len(assetIDs), Equals, 2)
	for i := 0; i < 2; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[1-i])
	}

	// user 1
	// page 1
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV:       {testName + common.QueryDelimiterKV + testValue}}, 0)
		return err
	}), IsNil)
	c.Assert(total, Equals, testAssetCount-2)
	c.Assert(len(assetIDs), Equals, limit)
	for i := 0; i < limit; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[testAssetCount-1-i])
	}

	// page 2
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV:       {testName + common.QueryDelimiterKV + testValue}}, 1)
		return err
	}), IsNil)
	c.Assert(total, Equals, testAssetCount-2)
	c.Assert(len(assetIDs), Equals, limit-2)
	for i := 0; i < limit-2; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[testAssetCount-limit-1-i])
	}

	// test AND operator @ discovery museum
	poiName := "poi"
	poiValue1 := "discovery-museum"
	testPOICount := 50
	metas = make([]types.Metadata, testPOICount)
	for i := 0; i < testPOICount; i++ {
		metas[i] = types.Metadata{
			Category:     types.MetadataCategoryGeo,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      hs.assetIDs[i],
			Name:         poiName,
			Value:        poiValue1,
			Model:        "tensorflow",
			Version:      2,
		}
	}
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		for _, meta := range metas {
			if err := InsertOrUpdateMetadata(ctx, tx, meta); err != nil {
				return err
			}
		}
		return nil
	}), IsNil)

	// user 0
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 0, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV: {
				testName + common.QueryDelimiterKV + testValue,
				poiName + common.QueryDelimiterKV + poiValue1}}, 0)
		return err
	}), IsNil)
	c.Assert(total, Equals, 2)
	c.Assert(len(assetIDs), Equals, 2)
	for i := 0; i < 2; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[1-i])
	}

	// user 1
	// query both poi and city
	// page 1
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV: {
				testName + common.QueryDelimiterKV + testValue,
				poiName + common.QueryDelimiterKV + poiValue1}}, 0)
		return err
	}), IsNil)
	c.Assert(total, Equals, testAssetCount-2)
	c.Assert(len(assetIDs), Equals, limit)
	for i := 0; i < limit; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[testAssetCount-1-i])
	}

	// page 2
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV: {
				testName + common.QueryDelimiterKV + testValue,
				poiName + common.QueryDelimiterKV + poiValue1}}, 1)
		return err
	}), IsNil)
	c.Assert(total, Equals, testAssetCount-2)
	c.Assert(len(assetIDs), Equals, limit-2)
	for i := 0; i < limit-2; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[testAssetCount-limit-1-i])
	}

	// query poi only
	// user 0
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 0, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV:       {poiName + common.QueryDelimiterKV + poiValue1}}, 0)
		return err
	}), IsNil)
	c.Assert(total, Equals, 2)
	c.Assert(len(assetIDs), Equals, 2)
	for i := 0; i < 2; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[1-i])
	}

	// user 1
	// page 1
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV:       {poiName + common.QueryDelimiterKV + poiValue1}}, 0)
		return err
	}), IsNil)
	c.Assert(total, Equals, testPOICount-2)
	c.Assert(len(assetIDs), Equals, testPOICount-2)
	for i := 0; i < testPOICount-2; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[testPOICount-1-i])
	}

	// page 2
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV:       {poiName + common.QueryDelimiterKV + poiValue1}}, 1)
		return err
	}), IsNil)
	c.Assert(total, Equals, testPOICount-2)
	c.Assert(len(assetIDs), Equals, 0)

	// insert another POI only for asset 50-149
	poiValue2 := "shark-ice"
	testPOICount = 100
	metas = make([]types.Metadata, testPOICount)
	for i := 0; i < testPOICount; i++ {
		metas[i] = types.Metadata{
			Category:     types.MetadataCategoryGeo,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      hs.assetIDs[50+i],
			Name:         poiName,
			Value:        poiValue2,
			Model:        "tensorflow",
			Version:      2,
		}
	}
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		for _, meta := range metas {
			if err := InsertOrUpdateMetadata(ctx, tx, meta); err != nil {
				return err
			}
		}
		return nil
	}), IsNil)

	// query both poi and city
	// user 0
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 0, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV: {
				testName + common.QueryDelimiterKV + testValue,
				poiName + common.QueryDelimiterKV + poiValue2}}, 0)
		return err
	}), IsNil)
	c.Assert(total, Equals, 2)
	c.Assert(len(assetIDs), Equals, 2)
	for i := 0; i < 2; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[1-i])
	}

	// user 1
	// page 1
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV: {
				testName + common.QueryDelimiterKV + testValue,
				poiName + common.QueryDelimiterKV + poiValue2}}, 0)
		return err
	}), IsNil)
	c.Assert(total, Equals, testAssetCount-2)
	c.Assert(len(assetIDs), Equals, limit)
	for i := 0; i < limit; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[testAssetCount-1-i])
	}

	// page 2
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV: {
				testName + common.QueryDelimiterKV + testValue,
				poiName + common.QueryDelimiterKV + poiValue2}}, 1)
		return err
	}), IsNil)
	c.Assert(total, Equals, testAssetCount-2)
	c.Assert(len(assetIDs), Equals, limit-2)
	for i := 0; i < limit-2; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[testAssetCount-limit-1-i])
	}

	// query poi with 2nd place only
	// user 0
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 0, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV:       {poiName + common.QueryDelimiterKV + poiValue2}}, 0)
		return err
	}), IsNil)
	c.Assert(total, Equals, 0)
	c.Assert(len(assetIDs), Equals, 0)

	// user 1
	// page 1
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV:       {poiName + common.QueryDelimiterKV + poiValue2}}, 0)
		return err
	}), IsNil)
	c.Assert(total, Equals, testPOICount)
	c.Assert(len(assetIDs), Equals, testPOICount)
	for i := 0; i < testPOICount; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[testAssetCount-51-i])
	}

	// page 2
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV:       {poiName + common.QueryDelimiterKV + poiValue2}}, 1)
		return err
	}), IsNil)
	c.Assert(total, Equals, testPOICount)
	c.Assert(len(assetIDs), Equals, 0)

	// query 2 poi which are not overlapped
	// page 1
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV: {
				poiName + common.QueryDelimiterKV + poiValue1,
				poiName + common.QueryDelimiterKV + poiValue2}}, 0)
		return err
	}), IsNil)
	c.Assert(total, Equals, 148)
	c.Assert(len(assetIDs), Equals, limit)
	for i := 0; i < limit; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[149-i])
	}

	// page 2
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV: {
				poiName + common.QueryDelimiterKV + poiValue1,
				poiName + common.QueryDelimiterKV + poiValue2}}, 1)
		return err
	}), IsNil)
	c.Assert(total, Equals, 148)
	c.Assert(len(assetIDs), Equals, 48)
	for i := 0; i < 48; i++ {
		c.Assert(assetIDs[i], Equals, hs.assetIDs[49-i])
	}
}

func (hs *metadataSuite) TestMetadataAssetSearchLike(c *C) {
	// assume all assets have metadata key, value -> city, sanjose
	testName1 := "city"
	testName2 := "poi"
	meta := types.Metadata{
		Category:     types.MetadataCategoryGeo,
		SourceDevice: types.SourceDeviceIos,
		Model:        "tensorflow",
		Version:      2,
	}
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		meta.AssetID = 3
		meta.Name = testName1
		meta.Value = "san jose"
		if err := InsertOrUpdateMetadata(ctx, tx, meta); err != nil {
			return err
		}
		meta.AssetID = 4
		meta.Name = testName2
		meta.Value = "sanfrancisco "
		if err := InsertOrUpdateMetadata(ctx, tx, meta); err != nil {
			return err
		}
		meta.AssetID = 5
		meta.Name = testName2
		meta.Value = "my-san-carlos"
		if err := InsertOrUpdateMetadata(ctx, tx, meta); err != nil {
			return err
		}
		meta.AssetID = 5
		meta.Name = testName1
		meta.Value = "fake"
		return InsertOrUpdateMetadata(ctx, tx, meta)
	}), IsNil)

	var (
		total    int
		assetIDs []int
	)

	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV:       {testName1 + common.QueryDelimiterKV + "test"}}, 0)
		return err
	}), IsNil)
	c.Assert(total, Equals, 0)
	c.Assert(len(assetIDs), Equals, 0)

	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV:       {testName1 + common.QueryDelimiterKV + "san"}}, 0)
		return err
	}), IsNil)
	c.Assert(total, Equals, 1)
	c.Assert(len(assetIDs), Equals, 1)
	c.Assert(assetIDs[0], Equals, 3)

	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV:       {testName2 + common.QueryDelimiterKV + "san"}}, 0)
		return err
	}), IsNil)
	c.Assert(total, Equals, 2)
	c.Assert(len(assetIDs), Equals, 2)
	c.Assert(assetIDs[0], Equals, 5)
	c.Assert(assetIDs[1], Equals, 4)

	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assetIDs, total, err = getAssetIDsByMetadata(ctx, tx, 1, map[string][]string{
			QueryMetadataCategory: {string(types.MetadataCategoryGeo)},
			QueryMetadataNV:       {common.QueryDelimiterKV + "san"}}, 0)
		return err
	}), IsNil)
	c.Assert(total, Equals, 3)
	c.Assert(len(assetIDs), Equals, 3)
	c.Assert(assetIDs[0], Equals, 5)
	c.Assert(assetIDs[1], Equals, 4)
	c.Assert(assetIDs[2], Equals, 3)
}

func (hs *metadataSuite) TestMetadataDelete(c *C) {
	testName := "name1"
	metas := []types.Metadata{
		{
			Category:     types.MetadataCategoryGeo,
			SourceDevice: types.SourceDeviceIos,
			AssetID:      hs.assetIDs[2],
			Name:         testName,
			Value:        "value1",
			Model:        "tensorflow",
			Version:      1,
		},
		{
			Category:     types.MetadataCategoryGeo,
			SourceDevice: types.SourceDeviceAndroid,
			AssetID:      hs.assetIDs[2],
			Name:         testName,
			Value:        "value1",
			Model:        "tensorflow",
			Version:      1,
		},
		{
			Category:     types.MetadataCategoryGeo,
			SourceDevice: types.SourceDeviceAndroid,
			AssetID:      hs.assetIDs[3],
			Name:         testName,
			Value:        "value2",
			Model:        "tensorflow",
			Version:      1,
		},
	}
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		for _, meta := range metas {
			if err := InsertOrUpdateMetadata(ctx, tx, meta); err != nil {
				return err
			}
		}
		return nil
	}), IsNil)
	c.Assert(dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		return DeleteMetadatas(ctx, tx, hs.assetIDs[2])
	}), IsNil)
	err := dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		metas, err = GetMetadatas(ctx, tx, hs.assetIDs[2])
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(len(metas), Equals, 0)

	// asset 4 should still have metadata
	err = dbx.InQuery(hs.db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		metas, err = GetMetadatas(ctx, tx, hs.assetIDs[3])
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(len(metas), Equals, 1)
}
