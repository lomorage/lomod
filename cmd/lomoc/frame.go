package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/share"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/urfave/cli"
)

func sharePhotoRelink(ctx *cli.Context) error {
	db, err := sql.Open("sqlite3", ctx.String("db"))
	if err != nil {
		return err
	}
	defer db.Close()

	userDirs := map[int]string{}
	frameUserIDs := map[int]string{}
	links := map[string]string{}
	if err := dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		// find all frame users
		var err error
		//return home dir if request is from localhost
		us, err := user.ListUsers(ctx, tx)
		if err != nil {
			return err
		}
		for _, user := range us.Users {
			if user.IsBotUser() {
				frameUserIDs[user.ID] = user.HomeDir
			} else {
				userDirs[user.ID] = user.HomeDir
			}
		}
		// find all photos shared to the user
		for uid, homeDir := range frameUserIDs {
			shareDir := common.GetUserPhotoSharedDir(homeDir)
			_, err := os.Stat(shareDir)
			if err != nil {
				if !os.IsNotExist(err) {
					log.Printf("stat %s: %v", shareDir, err)
				} else {
					if err := os.MkdirAll(shareDir, common.DefaultFolderPermission); err != nil {
						return err
					}
				}
			}
			records, err := share.GetAllReceiveHistoryByAssets(ctx, tx, uid)
			if err != nil {
				return err
			}
			for _, r := range records.Records {
				assetID, err := ext.GetAssetIDByName(r.AssetID)
				if err != nil {
					return err
				}
				masterfile, _, err := asset.GetAssetMasterPreviewPath(ctx, tx, r.SenderID, assetID, 0, 0,
					nil, nil, common.DefaultFolderPermission)
				if err != nil {
					return err
				}
				links[filepath.Join(shareDir, r.AssetID)] = masterfile
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// setup link
	for dst, src := range links {
		_, err := os.Stat(dst)
		if err == nil {
			log.Printf("skip link: %s -> %s", dst, src)
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if !ctx.Bool("yes") {
			log.Printf("dryrun link: %s -> %s", dst, src)
			continue
		}
		if err := cmd.Link(src, dst); err != nil {
			return err
		}
		log.Printf("setup link: %s -> %s", dst, src)
	}

	return nil
}
