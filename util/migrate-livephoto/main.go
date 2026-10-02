package main

import (
	"context"
	"database/sql"
	"os"
	"strconv"

	"path/filepath"

	"fmt"

	"io"

	"hash"

	"encoding/json"

	"os/exec"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/check"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/release"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()

	app.Version = release.Version
	app.Usage = "migrate live photo with new hash calculation"
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
			Name:  "user",
			Usage: "user name to be migrated",
		},
		cli.BoolFlag{
			Name:  "run",
			Usage: "run restore action. Without it, it just scan and report",
		},
	}

	app.Action = migrate

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
}

func migrate(ctx *cli.Context) error {
	run := ctx.GlobalBool("run")
	basedir := ctx.GlobalString("base")
	username := ctx.GlobalString("user")
	fmt.Printf("run: %#v\n", run)
	fmt.Printf("basedir: %#v\n", basedir)

	dbFile, err := dbx.GetDBFile(basedir, "assets.db", common.DefaultFolderPermission)
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		return err
	}

	return dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		users, err := user.ListUsers(ctx, tx)
		if err != nil {
			return err
		}

		for _, u := range users.Users {
			if username != "" && u.Name != username {
				fmt.Printf("skip user %s, who is different from specified name %s\n", u.Name, username)
				continue
			}
			assets, err := listLivePhotos(ctx, tx, u)
			if err != nil {
				return err
			}
			assetsMap := map[string]*check.AssetFileInfo{}
			deleteAssetMap := map[string]*check.AssetFileInfo{}
			noConflict := true
			for _, a := range assets {
				a.LiphHASH, err = types.GetLivePhotoFileSHAByContent(a.Path, func(name string, imgSHA hash.Hash) (io.Writer, error) { return imgSHA, nil })
				if err != nil {
					return err
				}
				fmt.Printf("%s - %s's original HASH: %s; new HASH: %s\n", u.Name, a.Path, a.Hash, a.LiphHASH.TotalSHA1)
				a2, ok := assetsMap[a.LiphHASH.TotalSHA1]
				if !ok {
					assetsMap[a.LiphHASH.TotalSHA1] = a
				} else {
					noConflict = false
					fmt.Printf("%s - %s has conflict HASH with %s\n", u.Name, a.Path, a2.Path)
					// delete a2
					deleteAssetMap[a.LiphHASH.TotalSHA1] = a2
				}
			}

			for _, removeAsset := range deleteAssetMap {
				fmt.Printf("delete %s ", removeAsset.Path)
				fmt.Printf("delete from db, id = %d", removeAsset.ID)

				if run {
					if err := os.Remove(removeAsset.Path); err != nil {
						fmt.Printf("Error when delete asset %s from filesystem", removeAsset.Path)
					}

					_, err := tx.Exec("delete from asset where id = $1", removeAsset.ID)
					if err != nil {
						fmt.Printf("Error when delete asset %d from db: %s", removeAsset.ID, err)
					}
				}
			}

			if noConflict && run {
				if err := updateLivePhotosSHA(ctx, tx, assetsMap); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func listLivePhotos(ctx context.Context, tx *sql.Tx, u *user.User) ([]*check.AssetFileInfo, error) {
	stmt, err := tx.Prepare("select id, year, month, day, hash from asset where user_id = ? and ext_id = 0 order by id")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []*check.AssetFileInfo{}
	for rows.Next() {
		a := &check.AssetFileInfo{}
		err := rows.Scan(&a.ID, &a.Year, &a.Month, &a.Day, &a.Hash)
		if err != nil {
			return nil, err
		}
		master, _, err := common.GetUserPhotoMasterPreviewDirCreate(u.HomeDir, a.Year, a.Month, a.Day,
			common.DefaultFolderPermission)
		if err != nil {
			return nil, err
		}
		a.Path = filepath.Join(master, ext.NormalizeAssetNameString(a.Year, a.Month, a.Day,
			strconv.Itoa(a.ID)+".zip"))
		list = append(list, a)
	}
	return list, rows.Err()
}

func updateLivePhotosSHA(ctx context.Context, tx *sql.Tx, assetsMap map[string]*check.AssetFileInfo) error {
	for hash, a := range assetsMap {
		statement := "update asset set hash = '" + hash + "' where hash = '" + a.Hash + "'"
		fmt.Printf("run sql: %s\n", statement)
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}

		fmt.Printf("update %s's comment %v\n", a.Path, a.LiphHASH)
		if err := updateLivePhotosComment(a.Path, a.LiphHASH); err != nil {
			return err
		}
	}
	return nil
}

func updateLivePhotosComment(filename string, hash *types.LivePhotoHash) error {
	buf, err := json.Marshal(hash)
	if err != nil {
		return err
	}

	cmds := []string{"sh", "-c", "echo '" + string(buf) + "' | zip -z " + filename}
	out, err := exec.Command(cmds[0], cmds[1:]...).CombinedOutput()
	fmt.Printf("run %v got %s\n", cmds, string(out))
	return err
}
