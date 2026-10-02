package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common/exif"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"github.com/leslie-wang/times"
	"github.com/pkg/errors"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/check"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const gpsFile = "gps.json"

func checkConsistencyDB(ctx *cli.Context) error {
	logrus.SetLevel(logrus.TraceLevel)

	dir, err := getDefaultLomoDir()
	if err != nil {
		return err
	}

	db, err := sql.Open("sqlite3", ctx.Args()[0])
	if err != nil {
		return err
	}
	defer db.Close()

	var users []user.User
	uids := make(map[int]string)

	if err := dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		//return home dir if request is from localhost
		us, err := user.ListUsers(ctx, tx)
		if err != nil {
			return err
		}
		for _, user := range us.Users {
			uids[user.ID] = user.Name
			users = append(users, *user)
		}
		return nil
	}); err != nil {
		return err
	}

	logrus.Info("start building memdb")
	memdb := asset.NewMemDB()
	if err := memdb.Build(db); err != nil {
		return err
	}

	runner := check.NewRunner("exiftool", logger.NewCheckLogger(logrus.StandardLogger()))
	if err := runner.Start(users, memdb.GetAssetsList()); err != nil {
		return err
	}
	logrus.Info("finish consistency check")

	content, err := json.MarshalIndent(runner.BadAssets, "", "  ")
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(filepath.Join(dir, "consistency.json"), content, filePermission); err != nil {
		return err
	}

	runner.Report(os.Stdout, false)
	return nil
}

func checkGPS(ctx *cli.Context) error {
	assets, err := check.ScanGPSInfo(ctx.Args()[0])
	if err != nil {
		return err
	}
	root, err := getDefaultLomoDir()
	if err != nil {
		return err
	}
	file := filepath.Join(root, gpsFile)
	content, err := json.Marshal(assets)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(file, content, common.DefaultFilePermission); err != nil {
		return err
	}
	fmt.Println("Done. Refer " + file)
	return nil
}

func checkConsistencyTime(ctx *cli.Context) error {
	if len(ctx.Args()) == 0 {
		return errors.New("please supply one directory at least")
	}

	writer := tabwriter.NewWriter(os.Stdout, 10, 2, 2, ' ', 0)
	defer writer.Flush()

	writer.Write([]byte("File Name\tCreate Time (FS)\tCreate Time (EXIF)\n"))

	for _, dir := range ctx.Args() {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				log.Printf("scan %s\n", path)
				return nil
			}
			if !ext.IsMediaFile(info.Name()) {
				return nil
			}
			timeFS, err := getCreateTimeByFS(path)
			if err != nil {
				log.Printf("get %s create tme from file system: %s", path, err)
			}

			timeEXIF, err := getCreateTimeByEXIF(path)
			if err != nil {
				log.Printf("%v\n", err)
				writer.Write([]byte(fmt.Sprintf("%s\t%d-%02d-%02d\t\n", path,
					timeFS.Year(), timeFS.Month(), timeFS.Day())))
				return nil
			}
			if timeFS.Year() == timeEXIF.Year() &&
				timeFS.Month() == timeEXIF.Month() &&
				timeFS.Day() == timeEXIF.Day() {
				return nil
			}
			writer.Write([]byte(fmt.Sprintf("%s\t%d-%02d-%02d\t%d-%02d-%02d\n", path,
				timeFS.Year(), timeFS.Month(), timeFS.Day(),
				timeEXIF.Year(), timeEXIF.Month(), timeEXIF.Day(),
			)))
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func getCreateTimeByFS(filename string) (time.Time, error) {
	var t time.Time
	spec, err := times.Stat(filename)
	if err != nil {
		return t, err
	}
	if spec.HasBirthTime() {
		t = spec.BirthTime()
	} else {
		t = spec.ModTime()
	}

	return t, nil
}

func getCreateTimeByEXIF(filename string) (time.Time, error) {
	var t time.Time
	tags, err := exif.NewTags(filename, "exiftool", "")
	if err != nil {
		return t, err
	}
	return tags.GetCreateTime()
}
