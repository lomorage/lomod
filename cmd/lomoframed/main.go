package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/release"
	"bitbucket.org/lomoware/lomo-backend/handler/lomoframe"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func main() {
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("%s\n", c.App.Version)
	}

	app := cli.NewApp()

	app.Version = release.Version
	app.Usage = "lomo frame daemon application to login lomod and received shared photo"
	app.Email = "support@lomorage.com"

	dir, err := os.Getwd()
	if err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "base, b",
			Usage: "base directory",
			Value: dir,
		},
		cli.StringFlag{
			Name:  "log-dir",
			Usage: "logfile directory",
		},
		cli.UintFlag{
			Name:  "port, p",
			Value: common.LomoFramePort,
		},
		cli.DurationFlag{
			Name:  "ping-interval, pi",
			Usage: "keep alive interval",
			Value: time.Minute,
		},
	}

	sort.Sort(cli.FlagsByName(app.Flags))

	app.Action = bootService

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}

func initDir(name string, perm os.FileMode) error {
	_, err := os.Stat(name)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return os.MkdirAll(name, perm)
	}

	return nil
}

func initDirs(ctx *cli.Context) (lomoframe.Config, error) {
	config := lomoframe.Config{
		BaseDir: ctx.GlobalString("base"),
		MntDir:  filepath.Join(ctx.GlobalString("base"), "mnt"),
		ConfDir: common.GetConfDir(ctx.GlobalString("base")),
	}
	config.LogDir = ctx.GlobalString("log-dir")
	if config.LogDir == "" {
		config.LogDir = common.GetLogDir(config.BaseDir)
	}

	for _, dir := range []string{config.BaseDir, config.MntDir, config.LogDir, config.ConfDir} {
		if dir == config.MntDir {
			if err := initDir(dir, 0777); err != nil {
				return config, err
			}
		} else {
			if err := initDir(dir, common.DefaultFolderPermission); err != nil {
				return config, err
			}
		}
	}
	return config, nil
}

func bootService(ctx *cli.Context) error {
	config, err := initDirs(ctx)
	if err != nil {
		return err
	}
	config.Port = ctx.GlobalUint("port")
	config.Interval = ctx.GlobalDuration("ping-interval")

	c, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, err := lomoframe.NewHandler(c, &config)
	if err != nil {
		return err
	}
	defer func() {
		if err := h.Close(); err != nil {
			logrus.Fatal(err)
		}
	}()

	go signalHandler(h, cancel)

	logrus.Printf("Start serving at ::%d", config.Port)

	return http.ListenAndServe(fmt.Sprintf(":%d", config.Port), h.CreateRouter())
}
