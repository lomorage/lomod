package lomod

import (
	"context"
	"database/sql"
	"io/ioutil"
	"strconv"
	. "testing"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	. "gopkg.in/check.v1"
)

type sqlSuite struct {
}

var _ = Suite(&sqlSuite{})

func TestSqlSuite(t *T) {
	TestingT(t)
}

func (ss *sqlSuite) TestMetadata(c *C) {
	testDB, err := ioutil.TempFile("", "assets-db")
	c.Assert(err, IsNil)
	c.Assert(testDB.Close(), IsNil)
	//defer os.Remove(testDB.Name())

	db, err := sql.Open("sqlite3", testDB.Name())
	c.Assert(err, IsNil)
	defer db.Close()
	err = dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		statements := []string{sql3, sql7}
		for _, s := range statements {
			_, err := tx.ExecContext(ctx, s)
			if err != nil {
				return err
			}
		}

		name := "test_name"
		value := "hi"
		model := "test"
		version := 1
		t := time.Now().Truncate(time.Second)
		s := "insert into metadata(category, source_device, asset_id, name, value, model, version, create_time, last_modified_time) values(?, ?, ?, ?, ?, ?, ?, ?, ?)"
		// from types.metadataCategoryIDGeo to metadataCategoryIDEncrypt
		for i := 0; i <= 7; i++ {
			for j := 0; j < 100; j++ {
				device := "ios"
				if j%2 == 1 {
					device = "android"
				}
				_, err := tx.ExecContext(ctx, s, i, device, j, name+strconv.Itoa(j), value+strconv.Itoa(j),
					model, version, t, t)
				if err != nil {
					return err
				}
			}
		}

		// start migrate to new metadata tables
		_, err = tx.ExecContext(ctx, sql11)
		if err != nil {
			return err
		}

		// read again should be same
		for _, tbl := range []string{
			types.MetadataCategoryGeo,
			types.MetadataCategoryScene,
			types.MetadataCategoryFace,
			types.MetadataCategoryText,
			types.MetadataCategoryHuman,
			types.MetadataCategorySimilarity,
			types.MetadataCategoryTag,
			types.MetadataCategoryEncrypt,
		} {
			s := "select * from metadata_" + tbl + " order by asset_id"
			rows, err := tx.QueryContext(ctx, s)
			if err != nil {
				return err
			}
			j := 0
			for rows.Next() {
				var (
					obtAssetID, obtVersion                                int
					obtDevice, obtName, obtValue, obtModel, obtCT, obtLMT string
				)
				err = rows.Scan(&obtDevice, &obtAssetID, &obtName, &obtValue, &obtModel, &obtVersion, &obtCT, &obtLMT)
				if err != nil {
					return err
				}
				if j%2 == 1 {
					c.Assert(obtDevice, Equals, "android")
				} else {
					c.Assert(obtDevice, Equals, "ios")
				}
				c.Assert(obtAssetID, Equals, j)
				c.Assert(obtName, Equals, name+strconv.Itoa(j))
				c.Assert(obtValue, Equals, value+strconv.Itoa(j))
				c.Assert(obtModel, Equals, model)
				c.Assert(obtVersion, Equals, version)
				c.Assert(obtCT, Equals, t.Format("2006-01-02 15:04:05+00:00"))
				c.Assert(obtLMT, Equals, t.Format("2006-01-02 15:04:05+00:00"))
				j++
			}
		}

		return nil
	})
	c.Assert(err, IsNil)
}
