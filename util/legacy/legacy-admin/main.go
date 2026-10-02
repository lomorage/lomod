package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()

	app.Version = "0.0.1"
	app.Usage = "manage system remotely over http"
	app.Email = "support@lomorage.com"

	dir, err := os.Getwd()
	if err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "base, b",
			Value: dir,
		},
		cli.StringFlag{
			Name:  "mount, m",
			Value: "/mnt/lomo",
		},
		cli.UintFlag{
			Name:  "port, p",
			Value: 8001,
		},
	}

	app.Action = bootService

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
}

func bootService(ctx *cli.Context) {
	h := NewHandler(ctx.GlobalString("base"), ctx.GlobalString("mount"))
	http.ListenAndServe(fmt.Sprintf(":%d", ctx.GlobalInt("port")), h.CreateRouter())
}
