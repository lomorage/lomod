package main

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/urfave/cli"

	// use go-sqlite3
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	app := cli.NewApp()

	app.Usage = "migrate scripts and databases safely"
	app.Commands = []cli.Command{
		{
			Name:      "sql",
			ArgsUsage: "[db file with absolute path] [directory of numbered sql files]",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "schema-version, sv",
					Usage: "Print the schema version and exit",
				},
			},
			Action: runSQL,
		},
		{
			Name:      "script",
			ArgsUsage: "[db file with absolute path] [directory of numbered shell scripts]",
			Action:    runScripts,
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runScripts(ctx *cli.Context) error {
	if len(ctx.Args()) != 2 {
		return errors.Errorf("invalid args, see help")
	}

	//return migrator.Run(ctx.Args()[0], ctx.Args()[1], migrator.ClassScript, ctx.Bool("schema-version"), migrator.LoopScripts)
	return nil
}

func runSQL(ctx *cli.Context) error {
	if len(ctx.Args()) != 2 {
		return errors.Errorf("invalid args, see help")
	}

	//return migrator.Run(ctx.Args()[0], ctx.Args()[1], migrator.ClassDB, ctx.Bool("schema-version"), migrator.LoopSQLs)
	return nil
}
