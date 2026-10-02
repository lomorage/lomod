package main

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"strings"

	"path/filepath"

	"fmt"

	"archive/zip"
	"io"
	"path"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/release"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

type renameAsset struct {
	isdir bool
	src   string
	dst   string
}

func main() {
	app := cli.NewApp()

	app.Version = release.Version
	app.Usage = "migrate assets with new naming convention and also live photos"
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

	app.Action = migrate

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
}

func migrate(ctx *cli.Context) error {
	run := ctx.GlobalBool("run")
	basedir := ctx.GlobalString("base")
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

		allAssets := map[*user.User][]renameAsset{}
		for _, u := range users.Users {
			assets, err := scanAssets(tx, u)
			if err != nil {
				return err
			}
			allAssets[u] = assets
		}
		for k, v := range allAssets {
			logrus.Infof("inserting for users %s", k.Name)
			for _, a := range v {
				if err := moveAsset(tx, a, run); err != nil {
					logrus.Warnf("move asset %v got %v", a, err)
				}
			}
		}
		return nil
	})
}

func parseAssetID(p string) (int, error) {
	_, f := filepath.Split(p)
	if strings.HasSuffix(f, "_image.jpg") {
		return strconv.Atoi(strings.TrimSuffix(f, "_image.jpg"))
	} else if strings.HasSuffix(f, "_video.mov") {
		return strconv.Atoi(strings.TrimSuffix(f, "_video.mov"))
	} else {
		return strconv.Atoi(strings.TrimSuffix(f, filepath.Ext(f)))
	}
}

func mkMonthDir(rootDir string, y, m int, old bool) string {
	if old {
		return filepath.Join(filepath.Join(rootDir, fmt.Sprintf("%d", y)), fmt.Sprintf("%d", m))
	}
	return filepath.Join(filepath.Join(rootDir, fmt.Sprintf("%d", y)), fmt.Sprintf("%02d", m))
}

func mkDayDir(rootDir string, y, m, d int, old bool) string {
	if old {
		return filepath.Join(filepath.Join(filepath.Join(rootDir, fmt.Sprintf("%d", y)),
			fmt.Sprintf("%02d", m)), fmt.Sprintf("%d", d))
	}
	return filepath.Join(filepath.Join(filepath.Join(rootDir, fmt.Sprintf("%d", y)),
		fmt.Sprintf("%02d", m)), fmt.Sprintf("%02d", d))
}

func mkAssetDir(rootDir string, y, m, d, id int, old bool) string {
	if old {
		return filepath.Join(
			filepath.Join(
				filepath.Join(
					filepath.Join(rootDir, fmt.Sprintf("%d", y)),
					fmt.Sprintf("%02d", m)),
				fmt.Sprintf("%02d", d),
				fmt.Sprintf("%d", id)))
	}
	return filepath.Join(
		filepath.Join(
			filepath.Join(
				filepath.Join(rootDir, fmt.Sprintf("%d", y)),
				fmt.Sprintf("%02d", m)),
			fmt.Sprintf("%02d", d),
			fmt.Sprintf("%d%02d%02d_%d", y, m, d, id)))
}

func scanAssets(tx *sql.Tx, u *user.User) ([]renameAsset, error) {
	logrus.Infof("start scanning users %s", u.Name)
	assets := []renameAsset{}
	master, _ := common.GetUserPhotoDir(u.HomeDir)

	toBeRenamed := map[string]struct{}{}
	rows, err := tx.Query("select id, year, month, day, ext_id from asset where user_id = ? order by id", u.ID)
	if err != nil {
		return nil, err
	}
	var (
		id, y, m, d, eid int
	)
	for rows.Next() {
		if err := rows.Scan(&id, &y, &m, &d, &eid); err != nil {
			return nil, err
		}
		for _, rootDir := range []string{master} {
			if m < 10 {
				// rename month folder
				newDir := mkMonthDir(rootDir, y, m, false)
				_, ok := toBeRenamed[newDir]
				if !ok {
					toBeRenamed[newDir] = struct{}{}
					assets = append(assets, renameAsset{isdir: true, src: mkMonthDir(rootDir, y, m, true), dst: newDir})
				}
			}
			if d < 10 {
				// rename month folder
				newDir := mkDayDir(rootDir, y, m, d, false)
				_, ok := toBeRenamed[newDir]
				if !ok {
					toBeRenamed[newDir] = struct{}{}
					assets = append(assets, renameAsset{isdir: true, src: mkDayDir(rootDir, y, m, d, true), dst: newDir})
				}
			}
		}
		oldPrefix := mkAssetDir(master, y, m, d, id, true)
		newPrefix := mkAssetDir(master, y, m, d, id, false)
		e, err := ext.GetExtString(eid)
		if err != nil {
			return nil, err
		}
		if eid == ext.ZIP {
			// video.mov is left to process at 2nd run
			assets = append(assets, renameAsset{src: fmt.Sprintf("%s_video.mov", oldPrefix), dst: fmt.Sprintf("%s.%s", newPrefix, e)})
			assets = append(assets, renameAsset{src: fmt.Sprintf("%s_image.jpg", oldPrefix), dst: fmt.Sprintf("%s_image.jpg", newPrefix)})
		} else {
			assets = append(assets, renameAsset{src: fmt.Sprintf("%s.%s", oldPrefix, e), dst: fmt.Sprintf("%s.%s", newPrefix, e)})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return assets, nil
}

func moveAsset(tx *sql.Tx, ra renameAsset, run bool) error {
	if !strings.HasSuffix(ra.src, "_video.mov") {
		if ra.isdir {
			logrus.Infof("rename directory from %s to %s", ra.src, ra.dst)
		} else {
			logrus.Infof("RENAME asset from %s to %s", ra.src, ra.dst)
		}
		if run {
			return os.Rename(ra.src, ra.dst)
		}
		return nil
	}

	// read old sha
	aid, err := parseAssetID(ra.src)
	if err != nil {
		return err
	}
	oldsha := ""
	if err := tx.QueryRow("select hash from asset where id = ?", aid).Scan(&oldsha); err != nil {
		return err
	}

	d, p := filepath.Split(strings.TrimSuffix(ra.src, "_video.mov"))
	logrus.Infof("RECREATE live photo %s", ra.dst)

	if run {
		f, err := os.Create(ra.dst)
		if err != nil {
			return err
		}
		defer f.Close()
		w := zip.NewWriter(f)

		src := []string{path.Join(d, fmt.Sprintf("%s_image.jpg", p)), ra.src}
		dst := []string{fmt.Sprintf("%s.jpg", p), fmt.Sprintf("%s.mov", p)}
		for i := 0; i < 2; i++ {
			logrus.Infof("  ADD live photo asset %s into %s", src[i], dst[i])
			sf, err := os.Open(src[i])
			if err != nil {
				return err
			}
			df, err := w.Create(dst[i])
			if err != nil {
				return err
			}
			_, err = io.Copy(df, sf)
			if err != nil {
				return err
			}
		}
		if err := w.Close(); err != nil {
			return err
		}

		// reset DB hash
		sha, err := common.GetFileSHA(ra.dst)
		if err != nil {
			return err
		}

		logrus.Infof("  RESET live photo asset's hash from %s to %s", oldsha, sha)
		_, err = tx.Exec("update asset set hash = ? where id = ?", sha, aid)
		if err != nil {
			return err
		}

		logrus.Infof("  REMOVE live photo asset's video %s", ra.src)
		return os.Remove(ra.src)
	}
	src := []string{path.Join(d, fmt.Sprintf("%s_image.jpg", p)), ra.src}
	dst := []string{fmt.Sprintf("%s.jpg", p), fmt.Sprintf("%s.mov", p)}
	for i := 0; i < 2; i++ {
		logrus.Infof("  ADD live photo asset %s into %s", src[i], dst[i])
	}

	logrus.Infof("  RESET live photo asset's hash")
	logrus.Infof("  REMOVE live photo asset's video %s", ra.src)
	return nil
}
