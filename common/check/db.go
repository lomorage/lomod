package check

import (
	"context"
	"database/sql"

	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
)

var (
	days = [12]int{31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
)

func loadDBAssetsNoGPS(db *sql.DB) (map[int]map[int][][][]types.Asset, map[int]string, error) {
	var (
		assets   map[int]map[int][][][]types.Asset
		homeDirs map[int]string
	)
	return assets, homeDirs, dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		assets, err = loadDBAssetsNoGPSAssets(ctx, tx)
		if err != nil {
			return err
		}
		users, err := user.ListUsers(ctx, tx)
		if err != nil {
			return err
		}
		for _, u := range users.Users {
			homeDirs[u.ID] = u.HomeDir
		}
		return nil
	})
}
func loadDBAssetsNoGPSAssets(ctx context.Context, tx *sql.Tx) (map[int]map[int][][][]types.Asset, error) {
	assets := map[int]map[int][][][]types.Asset{}
	stmt, err := tx.Prepare("select user_id, hash, id, ext_id, create_time, latitude, longitude from asset where latitude = 0 or longitude = 0")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var (
		a   types.Asset
		uid int
		aid int
		eid int
		dt  string
	)
	for rows.Next() {
		err := rows.Scan(&uid, &a.Hash, &aid, &eid, &dt, &a.Latitude, &a.Longitude)
		if err != nil {
			return nil, err
		}
		a.Name, err = ext.MkAssetNameByID(aid, eid)
		if err != nil {
			return nil, err
		}
		a.Date, err = types.ParseDBTime(dt)
		if err != nil {
			return nil, err
		}
		recordU, ok := assets[uid]
		if !ok {
			recordU = map[int][][][]types.Asset{}
			assets[uid] = recordU
		}

		month := int(a.Date.Month()) - 1
		recordY, ok := recordU[a.Date.Year()]
		if !ok {
			recordY = [][][]types.Asset{}
			for m := 0; m < 12; m++ {
				recordM := [][]types.Asset{}
				for d := 0; d < days[m]; d++ {
					recordD := []types.Asset{}
					recordM = append(recordM, recordD)
				}
				recordY = append(recordY, recordM)
			}
			recordU[month] = recordY
		}

		recordY[month][a.Date.Day()-1] = append(recordY[month][a.Date.Day()-1], a)
	}
	return assets, rows.Err()
}
