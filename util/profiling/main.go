package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime/pprof"
	"time"

	"database/sql"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/release"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()

	app.Version = release.Version
	app.Usage = "profiling the application"
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
		cli.StringFlag{
			Name:   "profile",
			Usage:  "cpu profile file",
			Hidden: true,
		},
		cli.IntFlag{
			Name:   "profile-count",
			Usage:  "number of loops to run for profile",
			Value:  100,
			Hidden: true,
		},
		cli.StringFlag{
			Name:   "profile-token",
			Usage:  "token used for profile",
			Hidden: true,
		},
	}

	app.Action = profileApp

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
}

// Bench is result for each bench mark run
type Bench struct {
	ByYearMonth string
	ByMonth     string
}

// BenchResult is summary for all benchmarks
type BenchResult struct {
	Bench []Bench
}

func listAssetsYearMonth(db *sql.DB, token string) (*types.Years, error) {
	var years types.Years
	if err := dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		userid, _, _, _, err := user.ValidateTokenAll(ctx, tx, token)
		if err != nil {
			return err
		}
		years, err = asset.GetAssetsByYears(ctx, tx, userid, false)
		return err
	}); err != nil {
		return nil, err
	}
	return &years, nil
}

func listAssetsByMonth(db *sql.DB, token string, y, m int) error {
	return dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		userid, _, _, _, err := user.ValidateTokenAll(ctx, tx, token)
		if err != nil {
			return err
		}
		_, err = asset.GetAssetsByMonth(ctx, tx, userid, y, m)
		return err
	})
}

// Profile run many benchmark tests for db
func profileApp(ctx cli.Context) error {
	profile := ctx.GlobalString("profile")
	token := ctx.GlobalString("profile-token")
	count := ctx.GlobalInt("profile-count")
	dbFile, err := dbx.GetDBFile(ctx.GlobalString("base"), "assets.db", common.DefaultFolderPermission)
	if err != nil {
		return err
	}
	db, err := sql.Open("", dbFile)
	if err != nil {
		return err
	}

	f, err := os.Create(profile)
	if err != nil {
		return err
	}
	defer f.Close()

	pprof.StartCPUProfile(f)
	defer pprof.StopCPUProfile()

	result := BenchResult{Bench: []Bench{}}
	for i := 0; i < count; i++ {
		b := Bench{}
		start := time.Now()
		years, err := listAssetsYearMonth(db, token)
		if err != nil {
			return err
		}
		b.ByYearMonth = time.Since(start).String()

		start = time.Now()
		for _, y := range years.Years {
			for _, m := range y.Months {
				if err := listAssetsByMonth(db, token, y.Year, m.Month); err != nil {
					return err
				}
			}
		}
		b.ByMonth = time.Since(start).String()
		result.Bench = append(result.Bench, b)
	}
	content, err := json.MarshalIndent(result, "  ", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(content))
	return nil
}
