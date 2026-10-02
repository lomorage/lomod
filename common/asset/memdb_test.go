package asset

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	. "testing"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/migrator"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/migrations/sqls/lomod"
	. "gopkg.in/check.v1"
)

type memdbSuite struct {
}

var _ = Suite(&memdbSuite{})

func TestMemdbSuite(t *T) {
	TestingT(t)
}

func loadCategory(fname string) (types.Years, error) {
	years := types.Years{}
	contents, err := ioutil.ReadFile(fname)
	if err != nil {
		return years, err
	}

	return years, json.Unmarshal(contents, &years)
}

func insertAssets(mdb *Memdb, filename string) (types.Years, error) {
	category, err := loadCategory(filename)
	if err != nil {
		return category, err
	}

	for _, y := range category.Years {
		for _, m := range y.Months {
			for _, d := range m.Days {
				for _, a := range d.Assets {
					if err := mdb.Insert(a, 0, a.Date.Year(), int(a.Date.Month()), a.Date.Day()); err != nil {
						return category, err
					}
				}
			}
		}
	}

	return category, nil
}

func (hs *memdbSuite) SetUpTest(c *C) {
}

// dummy log trace callback function
func (hs *memdbSuite) LogCallback(string, string, string, bool) {
}

func (hs *memdbSuite) TearDownTest(c *C) {
}

func (hs *memdbSuite) TestBasic(c *C) {
	mdb := NewMemDB()
	hs.testBasic(c, mdb)
}

func (hs *memdbSuite) testBasic(c *C, mdb *Memdb) {
	category, err := insertAssets(mdb, "./assets_short.json")
	c.Assert(err, IsNil)

	// test day
	d := mdb.GetAssetsByDay(0, 2003, 1, 17)
	c.Assert(len(d.Assets), Equals, 1, Commentf("Got assets: %v", d.Assets))
	c.Assert(d.Assets[0].Hash, Equals, "17363532de7bc73e42823c1448bd52ffe45d4bfc", Commentf("Got assets: %v", d.Assets))
	c.Assert(d.Assets[0].Name, Equals, "4.jpg", Commentf("Got assets: %v", d.Assets))
	c.Assert(d.Hash, Equals, "fcf8499ca85333832c4302cd3363f328605f731d")
	c.Assert(d.Day, Equals, 17)

	d = mdb.GetAssetsByDay(1, 2003, 1, 17)
	c.Assert(len(d.Assets), Equals, 0)
	c.Assert(d.Hash, Equals, "")

	d = mdb.GetAssetsByDay(0, 2003, 1, 18)
	c.Assert(len(d.Assets), Equals, 0)
	c.Assert(d.Hash, Equals, "")

	d = mdb.GetAssetsByDay(0, 2003, 11, 1)
	c.Assert(len(d.Assets), Equals, 2)
	c.Assert(d.Day, Equals, 1)
	c.Assert(d.Assets[0].Hash, Equals, "575db2e474109f982ead09e7f8676680a679c9c0")
	c.Assert(d.Assets[0].Name, Equals, "3.jpg")
	c.Assert(d.Assets[1].Hash, Equals, "7426655f042e8605385cb413de639373b934b18d")
	c.Assert(d.Assets[1].Name, Equals, "2.jpg")

	// test month
	m := mdb.GetAssetsByMonth(0, 2003, 1)
	c.Assert(m.Month, Equals, 1)
	c.Assert(m.Hash, Equals, "a7204f74598f70940349623353cc80cecb28d1c6", Commentf("got month asset %v", m))
	c.Assert(len(m.Days), Equals, 1)
	c.Assert(m.Days[0].Day, Equals, 17)
	c.Assert(m.Days[0].Hash, Equals, "fcf8499ca85333832c4302cd3363f328605f731d")

	m = mdb.GetAssetsByMonth(1, 2003, 1)
	c.Assert(m.Month, Equals, 1)
	c.Assert(m.Hash, Equals, "")
	c.Assert(len(m.Days), Equals, 0)

	m = mdb.GetAssetsByMonth(0, 2003, 2)
	c.Assert(m.Month, Equals, 2)
	c.Assert(m.Hash, Equals, "")
	c.Assert(len(m.Days), Equals, 0)

	m = mdb.GetAssetsByMonth(0, 2003, 11)
	c.Assert(m.Month, Equals, 11)
	c.Assert(m.Hash, Equals, "e632b8eee3e9f6ea533022942d8768d4e1549888")
	c.Assert(len(m.Days), Equals, 2)
	c.Assert(m.Days[0].Day, Equals, 1)
	c.Assert(m.Days[0].Hash, Equals, "f1d1acc8ffdac4ebdb80d1ad203b512109dcd1fc")
	c.Assert(len(m.Days[0].Assets), Equals, 2)
	c.Assert(m.Days[0].Assets[0].Hash, Equals, "575db2e474109f982ead09e7f8676680a679c9c0")
	c.Assert(m.Days[0].Assets[0].Name, Equals, "3.jpg")
	c.Assert(m.Days[0].Assets[1].Hash, Equals, "7426655f042e8605385cb413de639373b934b18d")
	c.Assert(m.Days[0].Assets[1].Name, Equals, "2.jpg")
	c.Assert(m.Days[1].Day, Equals, 23)
	c.Assert(m.Days[1].Hash, Equals, "ee8e87bb216aa46de86501f4a4c5a27d00aff155")
	c.Assert(len(m.Days[1].Assets), Equals, 1)
	c.Assert(m.Days[1].Assets[0].Hash, Equals, "4ebf54db04f335ff66bfc1fd982be62bf23fc967")
	c.Assert(m.Days[1].Assets[0].Name, Equals, "1.jpg")

	// test year
	y := mdb.GetAssetsByYear(0, 2003, true)
	c.Assert(y.Year, Equals, 2003)
	c.Assert(y.Hash, Equals, "f8ac00b7a696970dd4af275deec103536cfe3e08")
	c.Assert(len(y.Months), Equals, 2)
	c.Assert(y.Months[0].Month, Equals, 1)
	c.Assert(y.Months[0].Hash, Equals, "a7204f74598f70940349623353cc80cecb28d1c6")
	c.Assert(len(y.Months[0].Days), Equals, 1)
	c.Assert(y.Months[0].Days[0].Day, Equals, 17)
	c.Assert(y.Months[0].Days[0].Hash, Equals, "fcf8499ca85333832c4302cd3363f328605f731d")

	// deep compare
	c.Assert(y, DeepEquals, category.Years[0])

	y = mdb.GetAssetsByYear(0, 2001, true)
	c.Assert(y.Year, Equals, 2001)
	c.Assert(y.Hash, Equals, "")
	c.Assert(len(y.Months), Equals, 0)

	y = mdb.GetAssetsByYear(1, 2003, true)
	c.Assert(y.Year, Equals, 2003)
	c.Assert(y.Hash, Equals, "")
	c.Assert(len(y.Months), Equals, 0)

	// test all years
	for i, y := range category.Years {
		for m := range y.Months {
			category.Years[i].Months[m].Days = []types.Day{}
		}
	}

	ys := mdb.GetAssetsByYears(0, false)
	c.Assert(ys, DeepEquals, category, Commentf("got %v", ys))
}

func (hs *memdbSuite) TestDelete(c *C) {
	mdb := NewMemDB()
	_, err := insertAssets(mdb, "./assets_short.json")
	c.Assert(err, IsNil)

	c.Assert(mdb.Remove(0, "1.jpg", types.Index), IsNil)

	delete1, err := loadCategory("./assets_short_delete1.json")
	c.Assert(err, IsNil)
	c.Assert(len(delete1.Years), Equals, 3)
	year := mdb.GetAssetsByYear(0, 2003, true)
	c.Assert(year, DeepEquals, delete1.Years[0])
	year = mdb.GetAssetsByYear(0, 2004, true)
	c.Assert(year, DeepEquals, delete1.Years[1])
	year = mdb.GetAssetsByYear(0, 2013, true)
	c.Assert(year, DeepEquals, delete1.Years[2])

	for i, y := range delete1.Years {
		for m := range y.Months {
			delete1.Years[i].Months[m].Days = []types.Day{}
		}
	}
	years := mdb.GetAssetsByYears(0, false)
	c.Assert(years, DeepEquals, delete1)

	// remove 1 phone at 2004
	c.Assert(mdb.Remove(0, "d4d8773112f68162949b9578f6e476c7f6c8af1f", types.Hash), IsNil)

	delete2, err := loadCategory("./assets_short_delete2.json")
	c.Assert(err, IsNil)
	c.Assert(len(delete2.Years), Equals, 2)
	year = mdb.GetAssetsByYear(0, 2003, true)
	c.Assert(year, DeepEquals, delete2.Years[0])
	year = mdb.GetAssetsByYear(0, 2013, true)
	c.Assert(year, DeepEquals, delete2.Years[1])

	hs.validateByYears(c, mdb, delete2)
}

func (hs *memdbSuite) validateByYears(c *C, mdb *Memdb, category types.Years) {
	for i, y := range category.Years {
		for m := range y.Months {
			category.Years[i].Months[m].Days = []types.Day{}
		}
	}
	years := mdb.GetAssetsByYears(0, false)
	c.Assert(years, DeepEquals, category)
}

func (hs *memdbSuite) TestCrossDay(c *C) {
	tdir, err := ioutil.TempDir("", "")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tdir)

	testDB, err := os.Create(path.Join(tdir, "assets.db"))
	c.Assert(err, IsNil)
	c.Assert(testDB.Close(), IsNil)
	c.Assert(migrator.StartLomod(testDB.Name(), "", lomod.SchemaStatements, common.DefaultFolderPermission), IsNil)

	db, err := dbx.OpenDB(testDB.Name(), &dbx.DBTrace{Log: hs})
	c.Assert(err, IsNil)

	d1, err := time.Parse(common.TimeFormatDBs[0], "2019-01-12 21:59:44-08:00")
	c.Assert(err, IsNil)
	d2, err := time.Parse(common.TimeFormatDBs[0], "2019-01-12 22:00:02-08:00")
	c.Assert(err, IsNil)
	d3, err := time.Parse(common.TimeFormatDBs[0], "2019-01-13 12:00:16-08:00")
	c.Assert(err, IsNil)

	c.Assert(dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		for i, d := range []time.Time{d1, d2, d3} {
			_, err := tx.ExecContext(ctx, `insert into asset(user_id, hash, year, month, day , device_id, ext_id, create_time, upload_time)
	values(?, ?, ?, ?, ?, ?, ?, ?, ?)`, 1, strconv.Itoa(i), 2019, 1, 13, 1, 1, d, time.Now())
			if err != nil {
				return err
			}
		}
		return nil
	}), IsNil)

	mdb := NewMemDB()
	c.Assert(mdb.Build(db), IsNil)

	month := mdb.GetAssetsByMonth(1, 2019, 1)
	fmt.Printf("---- %+v\n", month)
	c.Assert(len(month.Days), Equals, 1)
}

func (hs *memdbSuite) TestDeleteRepeat(c *C) {
	category, err := loadCategory("./assets_short.json")
	c.Assert(err, IsNil)

	mdb := NewMemDB()
	for _, y := range category.Years {
		for _, m := range y.Months {
			for _, d := range m.Days {
				for _, a := range d.Assets {
					c.Assert(mdb.Insert(a, 0, a.Date.Year(), int(a.Date.Month()), a.Date.Day()), IsNil)
				}
			}
		}
	}

	// delete all assets
	for _, y := range category.Years {
		for _, m := range y.Months {
			for _, d := range m.Days {
				for _, a := range d.Assets {
					c.Assert(mdb.Remove(0, a.Name, types.Index), IsNil)
				}
			}
		}
	}

	hs.validateDeleteAll(c, mdb)

	// now insert again
	hs.testBasic(c, mdb)

	// now delete again and validate years
	for _, y := range category.Years {
		for _, m := range y.Months {
			for _, d := range m.Days {
				for _, a := range d.Assets {
					c.Assert(mdb.Remove(0, a.Name, types.Index), IsNil)
				}
			}
		}
	}
	hs.validateDeleteAll(c, mdb)
}

func (hs *memdbSuite) validateDeleteAll(c *C, mdb *Memdb) {
	years := mdb.GetAssetsByYears(0, false)
	c.Assert(len(years.Years), Equals, 0)
	c.Assert(years.Hash, Equals, "")

	y := mdb.GetAssetsByYear(0, 2003, true)
	c.Assert(y.Hash, Equals, "")
	c.Assert(len(y.Months), Equals, 0)

	y = mdb.GetAssetsByYear(0, 2004, true)
	c.Assert(y.Hash, Equals, "")
	c.Assert(len(y.Months), Equals, 0)

	y = mdb.GetAssetsByYear(0, 2013, true)
	c.Assert(y.Hash, Equals, "")
	c.Assert(len(y.Months), Equals, 0)

	d := mdb.GetAssetsByDay(1, 2003, 1, 17)
	c.Assert(len(d.Assets), Equals, 0)
	d = mdb.GetAssetsByDay(0, 2003, 1, 18)
	c.Assert(len(d.Assets), Equals, 0)
	d = mdb.GetAssetsByDay(0, 2003, 11, 1)
	c.Assert(len(d.Assets), Equals, 0)
}

func (hs *memdbSuite) TestAllAssets(c *C) {
	mdb := NewMemDB()
	category, err := insertAssets(mdb, "../../api/test/lomod/testdata/assets_all.json")
	c.Assert(err, IsNil)

	hs.validateByYears(c, mdb, category)
}
