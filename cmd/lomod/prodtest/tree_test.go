package prodtest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/pkg/errors"

	. "gopkg.in/check.v1"
)

func readFromFile(filename string, data interface{}) error {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(content, data); err != nil {
		return errors.Wrapf(err, "while parsing: "+filename)
	}
	return nil
}

func (hs *prodSuite) TestMerkleTree(c *C) {
	hs.testMerkleTree(c, "./prod-assets_jeromy.db", "prod-tree_jeromy")
	hs.testMerkleTree(c, "./prod-assets_leslie.db", "prod-tree_leslie")
}

func (hs *prodSuite) testMerkleTree(c *C, filename, rootDir string) {
	dbtrace := &dbx.DBTrace{Log: &dbx.DummyLogger{}}
	db, err := dbx.OpenDB(filename, dbtrace)
	c.Assert(err, IsNil)

	memdb := asset.NewMemDB()
	c.Assert(memdb.Build(db), IsNil)

	users := map[int]string{}
	c.Assert(dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		//return home dir if request is from localhost
		us, err := user.ListUsers(ctx, tx)
		if err != nil {
			return err
		}
		for _, u := range us.Users {
			if u.IsBotUser() {
				continue
			}
			users[u.ID] = filepath.Join(rootDir, u.Name)
		}
		return nil
	}), IsNil)

	for id, name := range users {
		expectYears := types.Years{}
		c.Assert(readFromFile(filepath.Join(name, "summary.json"), &expectYears), IsNil)

		years := memdb.GetAssetsByYears(id)
		c.Assert(years, DeepEquals, expectYears)
		for _, y := range years.Years {
			ydir := filepath.Join(name, strconv.Itoa(y.Year))
			expectYear := types.Year{}
			c.Assert(readFromFile(filepath.Join(ydir, "summary.json"), &expectYear), IsNil)

			year := memdb.GetAssetsByYear(id, y.Year, true)
			for i, m := range year.Months {
				mdir := filepath.Join(ydir, strconv.Itoa(m.Month))
				expectMonth := types.Month{}
				c.Assert(readFromFile(filepath.Join(mdir, "summary.json"), &expectMonth), IsNil)

				month := memdb.GetAssetsByMonth(id, y.Year, m.Month)
				for j, d := range month.Days {
					ddir := filepath.Join(mdir, strconv.Itoa(d.Day))
					expectDay := types.Day{}
					c.Assert(readFromFile(filepath.Join(ddir, "summary.json"), &expectDay), IsNil)

					day := memdb.GetAssetsByDay(id, y.Year, m.Month, d.Day)
					for k := 0; k < len(day.Assets); k++ {
						day.Assets[k].Date = types.LomoTime{Time: time.Time{}}
						month.Days[j].Assets[k].Date = types.LomoTime{Time: time.Time{}}
						year.Months[i].Days[j].Assets[k].Date = types.LomoTime{Time: time.Time{}}
					}
					c.Assert(day, DeepEquals, expectDay)
				}
				c.Assert(month, DeepEquals, expectMonth)
			}
			c.Assert(year, DeepEquals, expectYear)
		}
	}
}

// this is just for simple verification purpose.
func (hs *prodSuite) TestVerify(c *C) {
	dbtrace := &dbx.DBTrace{Log: &dbx.DummyLogger{}}
	db, err := dbx.OpenDB("./prod-assets_jeromy.db", dbtrace)
	c.Assert(err, IsNil)

	memdb := asset.NewMemDB()
	c.Assert(memdb.Build(db), IsNil)

	day := memdb.GetAssetsByDay(2, 2017, 8, 13)
	content, err := json.MarshalIndent(&day, "  ", "  ")
	c.Assert(err, IsNil)
	fmt.Println(string(content))

	var (
		id int
		t  string
	)
	c.Assert(dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "select id, create_time from asset where id=1326").Scan(&id, &t)
	}), IsNil)
	fmt.Printf("---- %d: %s\n", id, t)
	d, err := time.Parse(common.TimeFormatDBs[0], t)
	c.Assert(err, IsNil)
	fmt.Println(d)
	fmt.Println(d.UTC().Format(common.TimeFormatLomod))
	fmt.Println(d.Format(common.TimeFormatLomod))
}
