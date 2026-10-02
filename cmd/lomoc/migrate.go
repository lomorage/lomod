package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/leslie-wang/govips/pkg/vips"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func migratePreview(ctx *cli.Context) error {
	previewDims, err := types.NewDimensions(ctx.String("preview-size"))
	if err != nil {
		return err
	}
	previewDims = append(previewDims, types.Dimension{Width: common.DefaultPreviewWidth})

	db, err := sql.Open("sqlite3", ctx.GlobalString("db"))
	if err != nil {
		return err
	}
	defer db.Close()

	memdb := asset.NewMemDB()
	if err := memdb.Build(db); err != nil {
		return err
	}

	userHomeDirs := map[int]string{}
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
			userHomeDirs[u.ID] = u.HomeDir
		}
		return nil
	}); err != nil {
		return err
	}

	for uid, userAssets := range memdb.GetAssetsList() {
		homedir, ok := userHomeDirs[uid]
		if !ok {
			return errors.Errorf("unable to find %d's home dir", uid)
		}
		for year, recordY := range userAssets {
			for m, recordM := range recordY {
				month := m + 1
				for d, recordD := range recordM {
					day := d + 1
					for _, a := range recordD {
						if !ext.IsImageFile(a.Name) {
							continue
						}
						master, preview, err := common.GetUserPhotoMasterPreviewDir(homedir, year, month, day, common.DefaultFolderPermission)
						if err != nil {
							return err
						}
						filename := ext.NormalizeAssetNameString(year, month, day, a.Name)
						parts := strings.Split(filename, ".")
						masterFilename := filepath.Join(master, filename)
						for _, dim := range previewDims {
							previewFilenameJpg := filepath.Join(preview,
								ext.MkPreviewAssetName(parts[0], parts[1], dim.Width, dim.Height, false, false))
							createJpg := false
							stat, err := os.Stat(previewFilenameJpg)
							if err != nil {
								if !os.IsNotExist(err) {
									return err
								}
								createJpg = true
							} else if stat.Size() == 0 {
								createJpg = true
							}

							previewFilenameWebp := ""
							if parts[1] == "png" {
								previewFilenameWebp = strings.Replace(previewFilenameJpg, "png", "webp", -1)
							} else {
								previewFilenameWebp = strings.Replace(previewFilenameJpg, "jpg", "webp", -1)
							}
							createWebp := false
							stat, err = os.Stat(previewFilenameWebp)
							if err != nil {
								if !os.IsNotExist(err) {
									return err
								}
								createWebp = true
							} else if stat.Size() == 0 {
								createWebp = true
							}

							previewFilenamePpm := strings.Replace(previewFilenameWebp, "webp", "ppm", -1)
							createPpm := false
							if ctx.Bool("save-ppm") {
								stat, err = os.Stat(previewFilenamePpm)
								if err != nil {
									if !os.IsNotExist(err) {
										return err
									}
									createPpm = true
								} else if stat.Size() == 0 {
									createPpm = true
								}
							}

							if !createJpg && !createWebp && !createPpm {
								fmt.Printf("skip existing width %d preview %s\n", dim.Width, masterFilename)
								continue
							}

							outImage, err := vips.Thumbnail(masterFilename, int(dim.Width))
							if err != nil {
								if outImage != nil {
									vips.FreeImage(outImage)
								}
								return errors.Wrap(err, "while generating jpg thumbnail")
							}

							vips.RemoveImageMetadata(outImage, "jpeg-thumbnail-data")
							vips.RemoveImageMetadata(outImage, "exif-data")
							vips.RemoveImageMetadata(outImage, "xmp-data")
							vips.RemoveImageMetadata(outImage, "iptc-data")
							vips.RemoveImageMetadata(outImage, "icc-profile-data")

							if createJpg && ctx.GlobalBool("use-jpg") {
								fmt.Printf("save jpg preview: %s by %s\n", previewFilenameJpg, masterFilename)
								if err := vips.Jpegsave(outImage, previewFilenameJpg); err != nil {
									return errors.Wrap(err, "while saving jpg preview")
								}
							}
							if createWebp && !ctx.GlobalBool("use-jpg") {
								fmt.Printf("save webp preview: %s by %s\n", previewFilenameWebp, masterFilename)
								if err := vips.Webpsave(outImage, previewFilenameWebp); err != nil {
									return errors.Wrap(err, "while saving webp preview")
								}
							}

							// save image lossless for quality measurement
							if ctx.Bool("save-ppm") && createPpm {
								fmt.Printf("save lossless preview: %s by %s\n", previewFilenamePpm, masterFilename)
								if err := vips.Ppmsave(outImage, previewFilenamePpm); err != nil {
									return errors.Wrap(err, "while saving ppm preview")
								}
							}
							vips.FreeImage(outImage)
						}
					}
				}
			}
		}
	}

	return nil
}
