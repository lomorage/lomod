package asset

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/sirupsen/logrus"
)

// GetAssetsByDay return all assets in given day
func GetAssetsByDay(ctx context.Context, tx *sql.Tx, userid, y, m, d int) (types.Day, error) {
	day := types.Day{Day: d, Assets: []types.Asset{}}
	stmt, err := tx.Prepare("select id, hash, ext_id, status, create_time from (select * from asset as a inner join (select * from device) as d on a.device_id = d.id where a.user_id = ? and year = ? and month = ? and day = ?) order by hash")
	if err != nil {
		return day, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, userid, y, m, d)
	if err != nil {
		return day, err
	}

	defer rows.Close()

	dayHash := []string{}
	for rows.Next() {
		var (
			assetid int
			extID   int
			dt      string
			a       types.Asset
		)
		err := rows.Scan(&assetid, &a.Hash, &extID, &a.Status, &dt)
		if err != nil {
			logrus.Warnf("error scanning db: %v", err)
		}
		a.Name, err = ext.MkAssetNameByID(assetid, extID)
		if err != nil {
			return day, err
		}
		a.Date, err = types.ParseDBTime(dt)
		if err != nil {
			logrus.Warnf("error format time stamp %s", dt)
		}
		day.Assets = append(day.Assets, a)
		dayHash = append(dayHash, a.Hash)
	}
	if rows.Err() != nil {
		return day, err
	}
	if len(dayHash) != 0 {
		data := []byte(strings.Join(dayHash, ""))
		day.Hash = fmt.Sprintf("%x", sha1.Sum(data))
	}
	return day, nil
}

func getAssetsDayHash(ctx context.Context, tx *sql.Tx, userid, y, m, d int) (string, error) {
	stmt, err := tx.Prepare("select group_concat(hash, '') from (select hash from asset where user_id = ? and year = ? and month = ? and day = ? order by hash)")
	if err != nil {
		return "", err
	}
	defer stmt.Close()

	hash := ""
	err = stmt.QueryRowContext(ctx, userid, y, m, d).Scan(&hash)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", sha1.Sum([]byte(hash))), nil
}

func getDistinctDays(ctx context.Context, tx *sql.Tx, userid, y, m int) ([]int, error) {
	ds := []int{}
	stmt, err := tx.Prepare("select distinct day as d from asset where user_id = ? and year = ? and month = ? order by d")
	if err != nil {
		return ds, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, userid, y, m)
	if err != nil {
		return ds, err
	}

	defer rows.Close()

	for rows.Next() {
		var d int
		err = rows.Scan(&d)
		if err != nil {
			return ds, err
		}
		ds = append(ds, d)
	}
	return ds, rows.Err()
}

// GetAssetsByMonth return all assets in one month
func GetAssetsByMonth(ctx context.Context, tx *sql.Tx, userid, y, m int) (types.Month, error) {
	// get all days at that month
	month := types.Month{Month: m, Days: []types.Day{}}
	ds, err := getDistinctDays(ctx, tx, userid, y, m)
	if err != nil {
		return month, err
	}
	monthHash := []string{}
	for _, d := range ds {
		day, err := GetAssetsByDay(ctx, tx, userid, y, m, d)
		if err != nil {
			return month, err
		}
		if len(day.Assets) == 0 {
			logrus.Warnf("%d-%d-%d should get asset hash, but not", y, m, d)
			continue
		}
		month.Days = append(month.Days, day)
		monthHash = append(monthHash, day.Hash)
	}
	if len(monthHash) != 0 {
		data := []byte(strings.Join(monthHash, ""))
		month.Hash = fmt.Sprintf("%x", sha1.Sum(data))
	}
	return month, nil
}

func getDaysMap(ctx context.Context, tx *sql.Tx, userid, y int) ([12][31]bool, error) {
	var daysMap [12][31]bool
	stmt, err := tx.Prepare("select distinct month as m, day as d from asset where user_id = ? and year= ? order by m, d")
	if err != nil {
		return daysMap, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, userid, strconv.Itoa(y))
	if err != nil {
		return daysMap, err
	}

	defer rows.Close()

	for rows.Next() {
		var m, d int
		err = rows.Scan(&m, &d)
		if err != nil {
			logrus.Warnf("error scanning db: %v", err)
			return daysMap, err
		}
		daysMap[m-1][d-1] = true
	}
	return daysMap, rows.Err()
}

// GetAssetsByYear returns all assets in one year
func GetAssetsByYear(ctx context.Context, tx *sql.Tx, userid, y int, dayDetail bool) (types.Year, error) {
	year := types.Year{Year: y, Months: []types.Month{}}

	daysMap, err := getDaysMap(ctx, tx, userid, y)
	if err != nil {
		return year, err
	}

	yearHash := []string{}
	for m, mm := range daysMap {
		month := types.Month{Month: m + 1, Days: []types.Day{}}
		monthHash := []string{}
		for d, value := range mm {
			if !value {
				continue
			}
			var day types.Day
			if dayDetail {
				day, err = GetAssetsByDay(ctx, tx, userid, y, m+1, d+1)
				if err != nil {
					return year, err
				}
				if len(day.Assets) == 0 {
					logrus.Warnf("%d-%d-%d should have asset, not find", y, m+1, d+1)
					continue
				}
				month.Days = append(month.Days, day)
			} else {
				day.Hash, err = getAssetsDayHash(ctx, tx, userid, y, m+1, d+1)
				if err != nil {
					return year, err
				}
				if day.Hash == "" {
					logrus.Warnf("%d-%d-%d should get day hash, but not", y, m+1, d+1)
					continue
				}
			}
			monthHash = append(monthHash, day.Hash)
		}
		if len(monthHash) == 0 {
			continue
		}
		data := []byte(strings.Join(monthHash, ""))
		month.Hash = fmt.Sprintf("%x", sha1.Sum(data))
		year.Months = append(year.Months, month)

		yearHash = append(yearHash, month.Hash)
	}
	if len(yearHash) != 0 {
		data := []byte(strings.Join(yearHash, ""))
		year.Hash = fmt.Sprintf("%x", sha1.Sum(data))
	}
	return year, nil
}

func getDistinctYears(ctx context.Context, tx *sql.Tx, userid int) ([]int, error) {
	ys := []int{}
	stmt, err := tx.Prepare("select distinct year as y from asset where user_id = ? order by y")
	if err != nil {
		return ys, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, userid)
	if err != nil {
		return ys, err
	}

	defer rows.Close()

	for rows.Next() {
		var y int
		err = rows.Scan(&y)
		if err != nil {
			return ys, err
		}
		ys = append(ys, y)
	}
	return ys, rows.Err()
}

// GetAssetsByYears returns all assets in all year
func GetAssetsByYears(ctx context.Context, tx *sql.Tx, userid int, detail bool) (types.Years, error) {
	years := types.Years{Years: []types.Year{}}
	ys, err := getDistinctYears(ctx, tx, userid)
	if err != nil {
		return years, err
	}

	yearsHash := []string{}
	for _, y := range ys {
		year, err := GetAssetsByYear(ctx, tx, userid, y, detail)
		if err != nil {
			return years, err
		}
		years.Years = append(years.Years, year)
		yearsHash = append(yearsHash, year.Hash)
	}
	if len(yearsHash) != 0 {
		data := []byte(strings.Join(yearsHash, ""))
		years.Hash = fmt.Sprintf("%x", sha1.Sum(data))
	}

	return years, nil
}
