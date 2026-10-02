package album

import (
	"context"
	"database/sql"
	"io/ioutil"
	"os"
	"strconv"
	. "testing"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/migrator"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/migrations/sqls/lomod"
	. "gopkg.in/check.v1"
)

const testAssetCount = 10

type albumSuite struct {
	dbfile   string
	assetIDs map[int]string
}

var _ = Suite(&albumSuite{})

func TestAlbumSuite(t *T) {
	TestingT(t)
}

func (as *albumSuite) SetUpTest(c *C) {
	f, err := ioutil.TempFile("", "")
	c.Assert(err, IsNil)
	c.Assert(f.Close(), IsNil)

	as.dbfile = f.Name()

	c.Assert(migrator.StartLomod(as.dbfile, ".", lomod.SchemaStatements, common.DefaultFolderPermission), IsNil)

	// insert 200 asset IDs for testing
	db, err := sql.Open("sqlite3", as.dbfile)
	c.Assert(err, IsNil)
	defer db.Close()

	// #1-2 is for user 0
	// #3-200 is for user 1
	as.assetIDs = make(map[int]string, testAssetCount)
	c.Assert(dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		for i := 0; i < 2; i++ {
			id, err := asset.InsertAsset(ctx, tx, 0, 0, 0, &types.Asset{Hash: strconv.Itoa(i)}, 0, 0, time.Now())
			if err != nil {
				return err
			}
			as.assetIDs[int(id)] = ""
		}
		for i := 2; i < testAssetCount; i++ {
			id, err := asset.InsertAsset(ctx, tx, 1, 0, 0, &types.Asset{Hash: strconv.Itoa(i)}, 0, 0, time.Now())
			if err != nil {
				return err
			}
			as.assetIDs[int(id)] = ""
		}
		return nil
	}), IsNil)
}

func (as *albumSuite) TearDownTest(c *C) {
	c.Assert(os.Remove(as.dbfile), IsNil)
}

func (as *albumSuite) TestAlbumBasic(c *C) {
	// 1. create 10 album
	// 2. list all albums
	// 3. add all asset into one album
	// 4. list all assets in the album
	// 5. delete the 10th album
	// 6. list all albums
	// 7. delete the asset, and it should be removed from album too

	db, err := sql.Open("sqlite3", as.dbfile)
	c.Assert(err, IsNil)
	defer db.Close()

	count := 10
	albums := make([]Album, count)
	for i := 0; i < count; i++ {
		albums[i] = Album{Title: "title: " + strconv.Itoa(i)}
	}
	ids := map[int64]Album{}
	for i := 0; i < count; i++ {
		c.Assert(dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
			id, err := CreateAlbum(ctx, tx, 0, albums[i])
			albums[i].ID = int(id)
			ids[id] = albums[i]
			return err
		}), IsNil)
	}
	c.Assert(len(ids), Equals, count)

	var albumsReply *Albums
	c.Assert(dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		albumsReply, err = ListAlbums(ctx, tx, 0)
		if err != nil {
			return err
		}
		return nil
	}), IsNil)
	c.Assert(albumsReply, NotNil)
	c.Assert(len(albumsReply.Albums), Equals, count)

	c.Assert(dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		return AddAssets(ctx, tx, albums[0].ID, as.assetIDs)
	}), IsNil)

	var aids []types.AssetName
	c.Assert(dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		aids, err = ListAssets(ctx, tx, albums[0].ID, 0, 50)
		return err
	}), IsNil)

	c.Assert(len(aids), Equals, len(as.assetIDs))
	for _, a := range aids {
		id, err := ext.GetAssetIDByName(a.Name)
		c.Assert(err, IsNil)
		_, ok := as.assetIDs[id]
		c.Assert(ok, Equals, true)
	}

	deleteID := 0
	for id := range as.assetIDs {
		deleteID = id
		delete(as.assetIDs, id)
		break
	}
	c.Assert(dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		return DeleteAssets(ctx, tx, albums[0].ID, []int{deleteID})
	}), IsNil)

	c.Assert(dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		aids, err = ListAssets(ctx, tx, albums[0].ID, 0, 50)
		return err
	}), IsNil)

	c.Assert(len(aids), Equals, len(as.assetIDs))
	for _, a := range aids {
		id, err := ext.GetAssetIDByName(a.Name)
		c.Assert(err, IsNil)
		_, ok := as.assetIDs[id]
		c.Assert(ok, Equals, true)
	}
}
