package prodtest

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	. "testing"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/migrator"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/migrations/sqls/lomod"
	. "gopkg.in/check.v1"
)

type prodSuite struct {
}

var _ = Suite(&prodSuite{})

func TestProdSuite(t *T) {
	TestingT(t)
}

func (hs *prodSuite) TestProdDB(c *C) {
	hs.testProdDB(c, "./prod-assets_jeromy.db")
	hs.testProdDB(c, "./prod-assets_leslie.db")
}

func (hs *prodSuite) testProdDB(c *C, filename string) {
	c.Assert(migrator.Start(filename, lomod.SchemaStatements, common.DefaultFolderPermission), IsNil)

	db, err := dbx.OpenDB(filename, &dbx.DBTrace{Log: &dbx.DummyLogger{}})
	c.Assert(err, IsNil)
	defer db.Close()

	assets := map[int]map[int]string{}
	hashMaps := map[string][]int{}
	c.Assert(dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "select id, user_id, hash, create_time from asset")
		if err != nil {
			return err
		}

		defer rows.Close()

		for rows.Next() {
			var (
				id, uid  int
				hash, dt string
			)
			err = rows.Scan(&id, &uid, &hash, &dt)
			if err != nil {
				return err
			}
			ua, ok := assets[uid]
			if !ok {
				ua = map[int]string{}
			}
			ua[id] = hash
			assets[uid] = ua

			key := hash + strconv.Itoa(uid)
			as, ok := hashMaps[key]
			if !ok {
				as = []int{}
			}
			hashMaps[key] = append(as, id)

			_, err = types.ParseDBTime(dt)
			if err != nil {
				fmt.Printf("%s: %d fail to parse time: %v", filename, id, err)
			}
		}
		return rows.Err()
	}), IsNil)

	c.Assert(dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		for uid, as := range assets {
			for id, hash := range as {
				a1, err := asset.GetAssetByHash(ctx, tx, uid, hash)
				if err != nil {
					return err
				}
				a2, err := asset.GetAssetByID(ctx, tx, uid, id)
				if err != nil {
					return err
				}
				c.Assert(a1, DeepEquals, a2)
			}
		}
		return nil
	}), IsNil)

	for hash, ids := range hashMaps {
		if len(ids) > 1 {
			fmt.Printf("---- %s: %s has %v\n", filename, hash, ids)
		}
	}
}
