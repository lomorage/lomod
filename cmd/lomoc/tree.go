package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/urfave/cli"
)

func writeToFile(filename string, data interface{}) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("  ", "  ")
	return enc.Encode(data)
}

func dumpTree(ctx *cli.Context) error {
	db, err := sql.Open("sqlite3", ctx.String("db"))
	if err != nil {
		return err
	}
	defer db.Close()

	memdb := asset.NewMemDB()
	if err := memdb.Build(db); err != nil {
		return err
	}

	users := map[int]string{}
	if err := dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		//return home dir if request is from localhost
		us, err := user.ListUsers(ctx, tx)
		if err != nil {
			return err
		}
		for _, u := range us.Users {
			if u.IsBotUser() {
				continue
			}
			users[u.ID] = u.Name
		}
		return nil
	}); err != nil {
		return err
	}

	for id, name := range users {
		if err := os.MkdirAll(name, common.DefaultFolderPermission); err != nil {
			return err
		}
		years := memdb.GetAssetsByYears(id, false)
		if err := writeToFile(filepath.Join(name, "summary.json"), years); err != nil {
			return err
		}
		for _, y := range years.Years {
			ydir := filepath.Join(name, strconv.Itoa(y.Year))
			if err := os.MkdirAll(ydir, common.DefaultFolderPermission); err != nil {
				return err
			}
			year := memdb.GetAssetsByYear(id, y.Year, true)
			if err := writeToFile(filepath.Join(ydir, "summary.json"), year); err != nil {
				return err
			}
			for _, m := range year.Months {
				mdir := filepath.Join(ydir, strconv.Itoa(m.Month))
				if err := os.MkdirAll(mdir, common.DefaultFolderPermission); err != nil {
					return err
				}
				month := memdb.GetAssetsByMonth(id, y.Year, m.Month)
				if err := writeToFile(filepath.Join(mdir, "summary.json"), month); err != nil {
					return err
				}
				for _, d := range month.Days {
					ddir := filepath.Join(mdir, strconv.Itoa(d.Day))
					if err := os.MkdirAll(ddir, common.DefaultFolderPermission); err != nil {
						return err
					}
					day := memdb.GetAssetsByDay(id, y.Year, m.Month, d.Day)
					if err := writeToFile(filepath.Join(ddir, "summary.json"), day); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}
