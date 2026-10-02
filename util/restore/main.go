package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/device"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/release"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

type restoreAsset struct {
	id    int
	uid   int
	year  int
	month int
	day   int
	ext   int
	hash  string
	date  time.Time
}

func main() {
	app := cli.NewApp()

	app.Version = release.Version
	app.Usage = "restore assets"
	app.Email = "support@lomorage.com"

	dir, err := os.Getwd()
	if err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "base, b",
			Usage: "base directory to store db file",
			Value: dir,
		},
		cli.BoolFlag{
			Name:  "run",
			Usage: "run restore action. Without it, it just scan and report",
		},
	}

	app.Action = restore

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
}

func restore(ctx *cli.Context) error {
	run := ctx.GlobalBool("run")
	dbFile, err := dbx.GetDBFile(ctx.GlobalString("base"), "assets.db", common.DefaultFolderPermission)
	if err != nil {
		return err
	}
	db, err := sql.Open("", dbFile)
	if err != nil {
		return err
	}

	var users *user.Users
	if err := dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		users, err = user.ListUsers(ctx, tx)
		return err
	}); err != nil {
		return err
	}

	allAssets := map[*user.User][]restoreAsset{}
	for _, u := range users.Users {
		assets, err := scanAssets(u)
		if err != nil {
			return err
		}
		allAssets[u] = assets
	}

	id := func(a1, a2 *restoreAsset) bool {
		return a1.id < a2.id
	}
	for k, v := range allAssets {
		logrus.Infof("dry-run inserting for users %s", k.Name)
		By(id).Sort(v)
		for _, a := range v {
			e, _ := ext.GetExtString(a.ext)
			logrus.Infof("dry-run inserting user %d's asset %d.%s - %d-%d-%d %s @ %s", a.uid, a.id, e, a.year, a.month, a.day, a.hash, a.date)
		}
		if !run {
			continue
		}
		logrus.Infof("start inserting for users %s", k.Name)
		if err := restoreAssets(db, k, &v); err != nil {
			logrus.Warnf("inserting for users %s got %v", k.Name, err)
		}
	}
	return nil
}

func scanAssets(u *user.User) ([]restoreAsset, error) {
	logrus.Infof("start scanning users %s", u.Name)
	d, _ := common.GetUserPhotoDir(u.HomeDir)
	d = d + string(filepath.Separator)
	assets := []restoreAsset{}
	err := filepath.Walk(d, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			logrus.Warnf("while scanning %s: %v", p, err)
			return nil
		}
		if info.IsDir() {
			logrus.Infof("scanning dir %s", p)
			return nil
		}
		if strings.HasSuffix(info.Name(), "_image.jpg") {
			logrus.Infof("scanning live photo's extracted image file %s", p)
			return nil
		}

		a, err := restoreAssetDate(d, p, info)
		if err != nil {
			logrus.Infof("while calculating asset %s, got %v", info.Name(), err)
			return nil
		}
		a.uid = u.ID
		if err := restoreAssetInfo(&a, p, info.Name()); err != nil {
			return err
		}
		assets = append(assets, a)
		return nil
	})

	return assets, err
}

func restoreAssets(db *sql.DB, u *user.User, assets *[]restoreAsset) error {
	return dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		ids, err := device.GetDeviceIDs(ctx, tx, u.ID)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return errors.New("empty device for the user")
		}
		for _, a := range *assets {
			if _, err := tx.ExecContext(ctx, "insert into asset(id, user_id, hash, year, month, day, ext_id, device_id, create_time, upload_time) values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", a.id, a.uid, a.hash, a.year, a.month, a.day, a.ext, ids[0], a.date, a.date); err != nil {
				logrus.Warnf("insert asset %v got %v", a, err)
			}
		}

		return nil
	})
}

// By is the type of a "less" function that defines the ordering of restore assets.
type By func(a1, a2 *restoreAsset) bool

// Sort is a method on the function type, By, that sorts the argument slice according to the function.
func (by By) Sort(assets []restoreAsset) {
	as := &assetSorter{
		assets: assets,
		by:     by, // The Sort method's receiver is the function (closure) that defines the sort order.
	}
	sort.Sort(as)
}

type assetSorter struct {
	assets []restoreAsset
	by     func(p1, p2 *restoreAsset) bool // Closure used in the Less method.
}

// Len is part of sort.Interface.
func (s *assetSorter) Len() int {
	return len(s.assets)
}

// Swap is part of sort.Interface.
func (s *assetSorter) Swap(i, j int) {
	s.assets[i], s.assets[j] = s.assets[j], s.assets[i]
}

// Less is part of sort.Interface. It is implemented by calling the "by" closure in the sorter.
func (s *assetSorter) Less(i, j int) bool {
	return s.by(&s.assets[i], &s.assets[j])
}

func restoreAssetDate(d, p string, info os.FileInfo) (restoreAsset, error) {
	var err error
	a := restoreAsset{date: info.ModTime()}
	name := strings.TrimPrefix(p, d)
	parts := strings.Split(name, string(filepath.Separator))
	a.year, err = strconv.Atoi(parts[0])
	if err != nil {
		errors.Wrapf(err, "while converting year %s", name)
		return a, err
	}
	a.month, err = strconv.Atoi(parts[1])
	if err != nil {
		errors.Wrapf(err, "while converting month %s", name)
		return a, err
	}
	a.day, err = strconv.Atoi(parts[2])
	if err != nil {
		errors.Wrapf(err, "while converting day %s", name)
		return a, err
	}

	return a, nil
}

func restoreAssetInfo(a *restoreAsset, p, name string) error {
	var err error
	a.hash, err = common.GetFileSHA(p)
	if err != nil {
		errors.Wrapf(err, "while calculating SHA for %s", name)
		return err
	}

	parts := strings.Split(name, ".")
	a.id, err = strconv.Atoi(parts[0])
	if err != nil {
		errors.Wrapf(err, "while converting id %s", name)
		return err
	}
	a.ext, err = ext.GetExtID(parts[1])
	if err != nil {
		errors.Wrapf(err, "while calculate extension %s", name)
		return err
	}

	return nil
}
