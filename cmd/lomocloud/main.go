package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/migrator"
	"bitbucket.org/lomoware/lomo-backend/common/release"
	"bitbucket.org/lomoware/lomo-backend/handler/lomocloud"
	lsql "bitbucket.org/lomoware/lomo-backend/migrations/sqls/lomocloud"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func main() {
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("%s\n", c.App.Version)
	}

	app := cli.NewApp()

	app.Version = release.Version
	app.Usage = "personal photo backup solution cloud backend daemon"
	app.Email = "support@lomorage.com"

	dir, err := os.Getwd()
	if err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
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
			Name:  "domain, d",
			Usage: "domain name for DDNS",
			Value: "hub.lomorage.com",
		},
		cli.UintFlag{
			Name:  "port, p",
			Value: common.LomoCloudPort,
		},
		cli.DurationFlag{
			Name:  "dead-timeout",
			Value: 480 * time.Hour,
		},
	}

	app.Action = bootService

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
}

func bootService(ctx *cli.Context) error {
	config := lomocloud.Config{
		ListenPort: ctx.GlobalInt("port"),
		DomainName: ctx.GlobalString("domain"),
	}

	baseDir := ctx.GlobalString("base")
	dbFile, err := dbx.GetDBFile(baseDir, "records.db", common.DefaultFolderPermission)
	if err != nil {
		return err
	}
	if err := migrator.StartLomocloud(dbFile, lsql.SchemaStatements); err != nil {
		logrus.Errorf("continue although db migration failure: %v", err)
	}
	config.DbFilename = dbFile
	config.LogDir = common.GetLogDir(baseDir)
	config.DeadTimeout = ctx.GlobalDuration("dead-timeout")
	if err := os.MkdirAll(config.LogDir, common.DefaultFolderPermission); err != nil {
		return err
	}

	// set allowed path for lomocloud
	logger.AllowedPath = map[string]map[string]bool{
		"/account/ip_map": {http.MethodPost: false, http.MethodGet: false},
	}

	c, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, err := lomocloud.NewHandler(c, &config)
	if err != nil {
		return err
	}
	defer func() {
		if err := h.Close(); err != nil {
			logrus.Fatal(err)
		}
	}()

	go signalHandler(h, cancel)

	/*
		TODO: do we need open port 80 ?
		go func() {
			logrus.Printf("Start serving at 80")
			if err := http.ListenAndServe(":80", h.CreateRouterHTTP()); err != nil {
				logrus.Fatal(err)
			}
		}()
	*/

	/* skip DNS for now
	go func() {
		logrus.Printf("Start DNS UDP server")
		dnserver := &dns.Server{Net: "udp", Handler: h}
		if err := dnserver.ListenAndServe(); err != nil {
			logrus.Fatal(err)
		}
	}()

	go func() {
		logrus.Printf("Start DNS TCP server")
		dnserver := &dns.Server{Net: "tcp", Handler: h}
		if err := dnserver.ListenAndServe(); err != nil {
			logrus.Fatal(err)
		}
	}()
	*/

	logrus.Printf("Start serving at ::%d", ctx.GlobalInt("port"))
	return http.ListenAndServe(fmt.Sprintf(":%d", ctx.GlobalInt("port")), h.CreateRouter())
}
