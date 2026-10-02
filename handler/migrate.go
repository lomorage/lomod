package handler

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/asset"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/exif"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/geo"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/sirupsen/logrus"
)

const (
	statementSize        = "select id, ext_id, year, month, day from asset where user_id=? and size=0 limit 100"
	statementAspectRatio = "select id, ext_id, year, month, day from asset where user_id=? and aspect_ratio=0.0 limit 100"
	statementEXIF        = `
select id, ext_id, year, month, day from asset
where user_id=? and asset.id not in (select asset_id in asset_exif)
limit 100
`
)

func (h *Handler) migrate() {
	h.fixBadAssetSize()
	h.migrateAssetSize()
	h.fixBadAssetAspectRatio()
	h.migrateAssetAspectRatio()
	go func() {
		h.cleanupOrphanAssetIDs()
		//h.migratePlaces()
		h.migrateScenes()
		//h.migrateExif()
	}()
}

func (h *Handler) fixBadAssetSize() {
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "update asset set size=0 where size is NULL")
		return err
	})
	if err != nil {
		logrus.Warnf("while fix bad asset size for migrate asset: %s", err)
	}
}

func (h *Handler) fixBadAssetAspectRatio() {
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "update asset set aspect_ratio=0.0 where aspect_ratio is NULL")
		return err
	})
	if err != nil {
		logrus.Warnf("while fix bad asset size for migrate asset: %s", err)
	}
}

func (h *Handler) migrateAssets(typ, statement string, cb func(map[int]string, map[int][]string)) {
	users := map[int]string{}
	for {
		err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
			//return home dir if request is from localhost
			us, err := user.ListUsers(ctx, tx)
			if err != nil {
				return err
			}
			for _, u := range us.Users {
				if u.IsBotUser() {
					continue
				}
				users[u.ID] = u.HomeDir
			}
			return nil
		})
		if err == nil {
			break
		}
		logrus.Warnf("while listing user for migrate asset %s: %s", typ, err)
		time.Sleep(time.Minute)
	}

	for {
		assetPaths := map[int][]string{}
		if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
			for uid, homeDir := range users {
				rows, err := tx.QueryContext(ctx, statement, uid)
				if err != nil {
					return err
				}
				defer rows.Close()

				for rows.Next() {
					var id, eid, y, m, d int
					if err := rows.Scan(&id, &eid, &y, &m, &d); err != nil {
						return err
					}
					master, preview, err := common.GetUserPhotoMasterPreviewDir(homeDir, y, m, d, h.conf.FolderPerm)
					if err != nil {
						return err
					}
					e, err := ext.GetExtString(eid)
					if err != nil {
						logrus.Warnf("%d has invalid extension %d: %s", id, eid, err)
						continue
					}
					assetPaths[id] = []string{
						filepath.Join(master, ext.NormalizeAssetName(y, m, d, id)+"."+e),
						filepath.Join(preview, ext.NormalizeAssetName(y, m, d, id)),
					}
				}
				if len(assetPaths) != 0 {
					break
				}
				if rows.Err() != nil {
					return rows.Err()
				}
			}
			return nil
		}); err != nil {
			logrus.Warnf("while getting unprocess asset file %s: %s", typ, err)
			time.Sleep(time.Minute)
			continue
		}

		cb(users, assetPaths)

		if len(assetPaths) == 0 {
			logrus.Infof("complete migrate asset %s", typ)
			break
		}
	}
}

func (h *Handler) migrateAssetSize() {
	users := map[int]string{}
	h.migrateAssets("size", statementSize, func(us map[int]string, assetPaths map[int][]string) {
		if len(users) == 0 {
			users = us
		}

		assetSizes := map[int]int{}
		for id, p := range assetPaths {
			stat, err := os.Stat(p[0])
			if err != nil {
				logrus.Warnf("%s stat got: %s", p[0], err)
				assetSizes[id] = -1
				continue
			}
			assetSizes[id] = int(stat.Size())
		}
		for {
			if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
				for id, size := range assetSizes {
					_, err := tx.ExecContext(ctx, "update asset set size = ? where id = ?", size, id)
					if err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				logrus.Warnf("while migrate asset file size: %s", err)
				time.Sleep(time.Minute)
				continue
			}
			break
		}
	})
	uids := []string{}
	for uid := range users {
		uids = append(uids, strconv.Itoa(uid))
	}
	statement := "select user_id, ext_id, count(*), sum(size) from asset where user_id in (" +
		strings.Join(uids, ",") + ") group by ext_id, user_id"
	for {
		if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx, statement)
			if err != nil {
				return err
			}
			defer rows.Close()

			for rows.Next() {
				var uid, eid int
				var count, sum int64
				if err := rows.Scan(&uid, &eid, &count, &sum); err != nil {
					return err
				}
				h.updateAssetSummary(uid, eid, count, sum)
			}
			return rows.Err()
		}); err != nil {
			logrus.Warnf("calculate user asset summary: %s", err)
			time.Sleep(time.Minute)
			continue
		}
		break
	}
}

func (h *Handler) migrateAssetAspectRatio() {
	h.migrateAssets("aspect_ratio", statementAspectRatio, func(us map[int]string, assetPaths map[int][]string) {
		assetAspectRatios := map[int]float32{}
		for id, p := range assetPaths {
			filename := ext.MkPreviewAssetName(p[1], strings.TrimPrefix(filepath.Ext(p[0]), "."), common.MaxPreviewWidth, 0, false, !h.conf.UseJpg)
			tags, err := exif.NewTags(filename, h.exiftool, "")
			if err != nil {
				logrus.Warnf("%v - %s exif analysis got: %s", p, filename, err)
				assetAspectRatios[id] = -1.0
				continue
			}
			assetAspectRatios[id] = float32(tags.GetWidth()) / float32(tags.GetHeight())
		}
		for {
			if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
				for id, ar := range assetAspectRatios {
					_, err := tx.ExecContext(ctx, "update asset set aspect_ratio = ? where id = ?", ar, id)
					if err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				logrus.Warnf("while migrate asset file aspect_ratio: %s", err)
				time.Sleep(time.Minute)
				continue
			}
			break
		}
	})
}

func (h *Handler) migratePlaces() {
	// iterative for all assets
	assetGeos := map[int][]types.Metadata{}
	for {
		err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
			var err error
			assetGeos, err = asset.GetAllMetadataByCategory(ctx, tx, types.MetadataCategoryGeo)
			return err
		})
		if err == nil {
			break
		}
		logrus.Warnf("while listing metadata for migrate asset geo location: %s", err)
		time.Sleep(time.Minute)
	}

	for {
		for assetID, ms := range assetGeos {
			place, placeLangs, err := h.parsePlaceFromMetadatas(ms)
			if err != nil {
				logrus.Warnf("while migrate asset %d geo location: %s", assetID, err)
				break
			}
			var geoAlbumIDs []int
			err = dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
				// TODO: one metadata update contains multiple assets
				for _, placeLang := range placeLangs {
					if placeLang.Lang == "" || placeLang.NameEn == "" || placeLang.Name == "" {
						continue
					}
					err := geo.InsertPlaceLangs(ctx, tx, placeLang)
					if err != nil {
						return err
					}
				}
				if place == nil {
					return nil
				}
				geoAlbumIDs, err = geo.SelectOrInsertPlace(ctx, tx, *place)
				if err != nil {
					return err
				}
				return geo.AssociateAssetWithPlaces(ctx, tx, assetID, geoAlbumIDs)
			})
			if err == nil {
				delete(assetGeos, assetID)
			} else {
				logrus.Warnf("while migrating asset %d geo metadata %v: %s", assetID, ms, err)
			}
		}

		if len(assetGeos) == 0 {
			break
		}
		logrus.Warnf("%d asset geo location migration fail, sleep and try again", len(assetGeos))
		time.Sleep(time.Minute)
	}
}

func (h *Handler) migrateScenes() {
	// iterative for all assets
	assetScenes := map[int][]types.Metadata{}
	for {
		err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
			var err error
			assetScenes, err = asset.GetAllMetadataByCategory(ctx, tx, types.MetadataCategoryScene)
			return err
		})
		if err == nil {
			break
		}
		logrus.Warnf("while listing metadata for migrate asset scene: %s", err)
		time.Sleep(time.Minute)
	}
	for {
		for assetID, ms := range assetScenes {
			err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
				sceneLabel, sceneProb := h.parseSceneFromMetadatas(ms)
				if sceneLabel == "" || sceneProb == 0.0 {
					return nil
				}
				return h.processMetadatasScene(ctx, tx, assetID, sceneLabel, sceneProb)
			})
			if err == nil {
				delete(assetScenes, assetID)
			} else {
				logrus.Warnf("while migrating asset %d scene metadata %v: %s", assetID, ms, err)
			}
		}

		if len(assetScenes) == 0 {
			break
		}
		logrus.Warnf("%d asset scene migration fail, sleep and try again", len(assetScenes))
		time.Sleep(time.Minute)
	}
}

func (h *Handler) migrateExif() {
	h.migrateAssets("exif", statementEXIF, func(us map[int]string, assetPaths map[int][]string) {
		assets := map[int]exif.RawTags{}
		for id, p := range assetPaths {
			_, err := os.Stat(p[0])
			if err != nil {
				logrus.Warnf("%s stat got: %s", p[0], err)
				// set empty tags
				assets[id] = exif.RawTags{}
				continue
			}
			tags, err := exif.NewTags(p[0], h.exiftool, "")
			if err != nil {
				logrus.Warnf("%s exif analysis got: %s", p[0], err)
				assets[id] = exif.RawTags{}
				continue
			}
			assets[id] = tags.RawTags
		}
		for {
			if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
				for id, tags := range assets {
					err := exif.InsertAssetEXIF(ctx, tx, id, tags)
					if err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				logrus.Warnf("while migrate asset exif: %s", err)
				time.Sleep(time.Minute)
				continue
			}
			break
		}
	})
}

func (h *Handler) cleanupOrphanAssetIDs() {
	err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		for _, tbl := range assetTables {
			_, err := tx.ExecContext(ctx, "delete from "+tbl+
				" where asset_id not in (select id from asset)")
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		logrus.Warnf("clean orphan metadata got: %v", err)
	}
}
