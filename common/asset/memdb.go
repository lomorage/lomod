package asset

import (
	"context"
	"crypto/sha1"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"database/sql"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const aspectRatioFactor = 1000

type memAsset struct {
	Name        string
	Hash        string
	AspectRatio int // truncated to save memory space
	Status      int
	Location    string
	CreateTime  time.Time
}

func (ma memAsset) ToAsset() types.Asset {
	return types.Asset{Name: ma.Name, Hash: ma.Hash, Status: ma.Status, Date: types.LomoTime{Time: ma.CreateTime}}
}

func (ma memAsset) ToAssetHash() types.AssetHash {
	typ := "image"
	parts := strings.Split(ma.Name, ".")
	id := parts[0]
	if len(parts) > 1 {
		if ext.IsLivePhoto(ma.Name) {
			typ = "video"
			id += "_video.mp4"
		} else if ext.IsVideoFile(parts[1]) {
			typ = "video"
			id += ".mp4"
		} else {
			id += ".webp"
		}
	}

	return types.AssetHash{
		ID: ma.Name, Hash: ma.Hash, URL: id, Type: typ, AspectRatio: float32(ma.AspectRatio) / aspectRatioFactor,
	}
}

// SortedAssets is array to sort asset by its sha
type SortedAssets []memAsset

func (a SortedAssets) Len() int           { return len(a) }
func (a SortedAssets) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a SortedAssets) Less(i, j int) bool { return a[i].Hash < a[j].Hash }

// Memdb is to put asset info in db to speed up access
type Memdb struct {
	lock         *sync.Mutex
	assetsList   map[int]map[int][][]SortedAssets
	assetsByID   map[int]map[int][]int8
	assetsByHash map[int]map[string][]int8
}

// NewMemDB creates one instance of memdb
func NewMemDB() *Memdb {
	return &Memdb{lock: &sync.Mutex{},
		assetsList:   map[int]map[int][][]SortedAssets{},
		assetsByID:   map[int]map[int][]int8{},
		assetsByHash: map[int]map[string][]int8{},
	}
}

// GetAssetsList returns all assets
func (md *Memdb) GetAssetsList() map[int]map[int][][][]types.Asset {
	assets := map[int]map[int][][][]types.Asset{}
	for uid, recordU := range md.assetsList {
		u := map[int][][][]types.Asset{}
		for year, recordY := range recordU {
			y := make([][][]types.Asset, 12)
			for month, recordM := range recordY {
				m := make([][]types.Asset, common.DaysInMonth[month])
				for day, recordD := range recordM {
					d := []types.Asset{}
					for _, a := range recordD {
						d = append(d, a.ToAsset())
					}
					m[day] = d
				}
				y[month] = m
			}
			u[year] = y
		}
		assets[uid] = u
	}
	return assets
}

// GetUserAssets returns all assets belong to one user
func (md *Memdb) GetUserAssets(uid int) (map[int][][]SortedAssets, error) {
	assets, ok := md.assetsList[uid]
	if !ok {
		return nil, common.ErrEmptyAsset
	}
	return assets, nil
}

// Build scans db and build the memdb
func (md *Memdb) Build(db *sql.DB) error {
	return dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		stmt, err := tx.Prepare(
			"select user_id, year, month, day, hash, id, ext_id, create_time, aspect_ratio, status from asset")
		if err != nil {
			return err
		}
		defer stmt.Close()

		rows, err := stmt.QueryContext(ctx)
		if err != nil {
			return err
		}
		defer rows.Close()

		var (
			a      types.Asset
			year   int
			month  int
			day    int
			uid    int
			aid    int
			eid    int
			status int
			ar     float32
			dt     string
		)
		for rows.Next() {
			err := rows.Scan(&uid, &year, &month, &day, &a.Hash, &aid, &eid, &dt, &ar, &status)
			if err != nil {
				return err
			}
			a.Name, err = ext.MkAssetNameByID(aid, eid)
			if err != nil {
				logrus.Warnf("while building memdb got extension error: %v", err)
				continue
			}
			a.Date, err = types.ParseDBTime(dt)
			if err != nil {
				logrus.Warnf("while building memdb got invalid create time: %s", dt)
				continue
			}
			if err := md.Insert(a, uid, year, month, day); err != nil {
				logrus.Warnf("while building memdb insert asset %v: %s", a, err)
				continue
			}
			if err := md.UpdateAspectRatio(uid, year, month, day, a.Name, ar); err != nil {
				logrus.Warnf("while building memdb update asset aspect ratio %v: %s", a, err)
			}
			/*
				err = tx.QueryRowContext(ctx, "select value from metadata_geo where (name like '"+
					types.MetadataGeoCityIos +"%' or name like '"+types.MetadataGeoCityAndroid +"%') and asset_id=?", aid).Scan(&location)
				if err != nil {
					logrus.Warnf("while building memdb query asset location %v: %s", a, err)
				} else if location != "" {
					if err := md.UpdateLocation(uid, year, month, day, a.Name, location); err != nil {
						logrus.Warnf("while building memdb update asset location %v: %s", a, err)
					}
				}
			*/
			if status == 0 {
				continue
			}
			if _, err := md.UpdateStatus(uid, year, month, day, a.Name, status); err != nil {
				logrus.Warnf("while building memdb update asset status %v: %s", a, err)
			}
		}
		return rows.Err()
	})
}

// ExistByHash checks if the asset is exist or not
func (md *Memdb) ExistByHash(uid int, hash string) *types.Asset {
	recordU, ok := md.assetsByHash[uid]
	if !ok {
		return nil
	}
	date, ok := recordU[hash]
	if !ok {
		return nil
	}
	assets := md.getAssetsByDay(uid, int(date[0])+common.StartYear, int(date[1]), int(date[2]))
	for _, a := range assets {
		if a.Hash == hash {
			ta := a.ToAsset()
			return &ta
		}
	}
	return nil
}

// ExistByID checks if the asset is exist or not by its ID
func (md *Memdb) ExistByID(uid, aid int) *types.Asset {
	var (
		date []int8
		ok   bool
	)
	if uid == -1 {
		// scan all user
		for _, recordU := range md.assetsByID {
			date, ok = recordU[aid]
			if ok {
				break
			}
		}
		return nil
	} else {
		recordU, ok := md.assetsByID[uid]
		if !ok {
			return nil
		}
		date, ok = recordU[aid]
		if !ok {
			return nil
		}
	}
	assets := md.getAssetsByDay(uid, int(date[0])+common.StartYear, int(date[1]), int(date[2]))
	id := strconv.Itoa(aid)
	for _, a := range assets {
		parts := strings.Split(a.Name, ".")
		if id == parts[0] {
			ta := a.ToAsset()
			return &ta
		}
	}
	return nil
}

func (md *Memdb) findHashByID(uid, aid int) string {
	// find y, m, d
	recordU, ok := md.assetsByID[uid]
	if !ok {
		return ""
	}
	ymd, ok := recordU[aid]
	if !ok {
		return ""
	}
	if len(ymd) != 3 {
		logrus.Warnf("user %d - %d has invalid records in assetsByID table %v", uid, aid, ymd)
		return ""
	}

	// find hash
	listU, ok := md.assetsList[uid]
	if !ok {
		logrus.Warnf("user %d - %d has no records in assetsHashList", uid, aid)
		return ""
	}
	listY, ok := listU[int(ymd[0])+common.StartYear]
	if !ok {
		logrus.Warnf("user %d - %d has no year records in assetsHashList", uid, aid)
		return ""
	}
	listD := listY[int(ymd[1])][int(ymd[2])]
	id := strconv.Itoa(aid)
	for _, a := range listD {
		if id == strings.Split(a.Name, ".")[0] {
			return a.Hash
		}
	}
	logrus.Warnf("user %d - %d unable find in assetsHashList", uid, aid)
	return ""
}

// Insert inserts one asset into the memory db
func (md *Memdb) Insert(a types.Asset, uid, year, month, day int) error {
	month--
	day--

	aid, err := ParseAssetID(a.Name)
	if err != nil {
		return err
	}
	if err := md.insertAssetByID(aid, uid, year, month, day); err != nil {
		return err
	}
	if err := md.insertAssetByHash(a.Hash, uid, year, month, day); err != nil {
		return err
	}
	md.insertHashList(a, uid, year, month, day)
	return nil
}

// UpdateAspectRatio update one asset aspect ratio in memdb
func (md *Memdb) UpdateAspectRatio(uid, year, month, day int, name string, ar float32) error {
	recordU, ok := md.assetsList[uid]
	if !ok {
		return errors.Errorf("no assets for user %d", uid)
	}

	recordY, ok := recordU[year]
	if !ok {
		return errors.Errorf("no assets for user %d at year %d", uid, year)
	}

	month--
	day--
	if month < 0 || month >= len(recordY) {
		return errors.Errorf("invalid month %d for user %d at year %d", month+1, uid, year)
	}
	if len(recordY[month]) == 0 {
		return errors.Errorf("no assets for user %d at %d-%d", uid, year, month+1)
	}

	if day < 0 || day >= len(recordY[month]) {
		return errors.Errorf("invalid day %d for user %d at %d-%d", day+1, uid, year, month)
	}
	if len(recordY[month][day]) == 0 {
		return errors.Errorf("no assets for user %d at %d-%d-%d", uid, year, month+1, day+1)
	}

	for i, a := range recordY[month][day] {
		if a.Name != name {
			continue
		}
		a.AspectRatio = int(ar * aspectRatioFactor)
		recordY[month][day][i] = a
		return nil
	}
	return common.ErrAssetNotExistForUser
}

// UpdateLocation update one asset location in memdb
func (md *Memdb) UpdateLocation(uid, year, month, day int, name, location string) error {
	recordU, ok := md.assetsList[uid]
	if !ok {
		return errors.Errorf("no assets for user %d", uid)
	}

	recordY, ok := recordU[year]
	if !ok {
		return errors.Errorf("no assets for user %d at year %d", uid, year)
	}

	month--
	day--
	if month < 0 || month >= len(recordY) {
		return errors.Errorf("invalid month %d for user %d at year %d", month+1, uid, year)
	}
	if len(recordY[month]) == 0 {
		return errors.Errorf("no assets for user %d at %d-%d", uid, year, month+1)
	}

	if day < 0 || day >= len(recordY[month]) {
		return errors.Errorf("invalid day %d for user %d at %d-%d", day+1, uid, year, month)
	}
	if len(recordY[month][day]) == 0 {
		return errors.Errorf("no assets for user %d at %d-%d-%d", uid, year, month+1, day+1)
	}

	for i, a := range recordY[month][day] {
		if a.Name != name {
			continue
		}
		a.Location = location
		recordY[month][day][i] = a
		return nil
	}
	return common.ErrAssetNotExistForUser
}

// UpdateStatus update one asset status in memdb
func (md *Memdb) UpdateStatus(uid, year, month, day int, name string, status int) (*types.Asset, error) {
	recordU, ok := md.assetsList[uid]
	if !ok {
		return nil, errors.Errorf("no assets for user %d", uid)
	}

	recordY, ok := recordU[year]
	if !ok {
		return nil, errors.Errorf("no assets for user %d at year %d", uid, year)
	}

	month--
	day--
	if month < 0 || month >= len(recordY) {
		return nil, errors.Errorf("invalid month %d for user %d at year %d", month+1, uid, year)
	}
	if len(recordY[month]) == 0 {
		return nil, errors.Errorf("no assets for user %d at %d-%d", uid, year, month+1)
	}

	if day < 0 || day >= len(recordY[month]) {
		return nil, errors.Errorf("invalid day %d for user %d at %d-%d", day+1, uid, year, month)
	}
	if len(recordY[month][day]) == 0 {
		return nil, errors.Errorf("no assets for user %d at %d-%d-%d", uid, year, month+1, day+1)
	}

	for i, a := range recordY[month][day] {
		if a.Name != name {
			continue
		}
		a.Status = status
		recordY[month][day][i] = a
		ra := a.ToAsset()
		return &ra, nil
	}
	return nil, common.ErrAssetNotExistForUser
}

func (md *Memdb) insertHashList(asset types.Asset, uid, year, month, day int) {
	recordU, ok := md.assetsList[uid]
	if !ok {
		recordU = map[int][][]SortedAssets{}
		md.assetsList[uid] = recordU
	}

	recordY, ok := recordU[year]
	if !ok {
		recordY = [][]SortedAssets{}
		for m := 0; m < 12; m++ {
			recordM := []SortedAssets{}
			for d := 0; d < common.DaysInMonth[m]; d++ {
				recordD := SortedAssets{}
				recordM = append(recordM, recordD)
			}
			recordY = append(recordY, recordM)
		}
		recordU[year] = recordY
	}

	recordY[month][day] = append(recordY[month][day], memAsset{Name: asset.Name,
		CreateTime: asset.Date.Time, Hash: asset.Hash})

	sort.Sort(recordY[month][day])
}

func (md *Memdb) insertAssetByID(aid, uid, year, month, day int) error {
	recordU, ok := md.assetsByID[uid]
	if !ok {
		recordU = map[int][]int8{}
		md.assetsByID[uid] = recordU
	}

	_, ok = recordU[aid]
	if ok {
		return errors.Wrapf(common.ErrDuplicate, "while insert asset id %d for %d", aid, uid)
	}
	recordU[aid] = []int8{int8(year - common.StartYear), int8(month), int8(day)}
	return nil
}

func (md *Memdb) insertAssetByHash(hash string, uid, year, month, day int) error {
	recordU, ok := md.assetsByHash[uid]
	if !ok {
		recordU = map[string][]int8{}
		md.assetsByHash[uid] = recordU
	}

	_, ok = recordU[hash]
	if ok {
		return errors.Wrapf(common.ErrDuplicate, "while insert asset hash %s for %d", hash, uid)
	}
	recordU[hash] = []int8{int8(year - common.StartYear), int8(month), int8(day)}
	return nil
}

func (md *Memdb) removeHashListByHash(uid, year, month, day int, hash string) {
	as := md.assetsList[uid][year][month][day]
	for i, d := range as {
		if d.Hash != hash {
			continue
		}
		copy(as[i:], as[i+1:])
		md.assetsList[uid][year][month][day] = as[:len(as)-1]
		return
	}
}

func (md *Memdb) removeAssetByID(uid, aid int) []int8 {
	ymd := md.assetsByID[uid][aid]
	delete(md.assetsByID[uid], aid)
	return ymd
}

func (md *Memdb) removeAssetByHash(uid int, hash string) {
	delete(md.assetsByHash[uid], hash)
}

func (md *Memdb) resolveAndValidateHash(uid int, hash string) (int, error) {
	a := md.ExistByHash(uid, hash)
	if a == nil {
		return 0, errors.Errorf("unable to find hash %s for user %d", hash, uid)
	}
	aid, err := ParseAssetID(a.Name)
	if err != nil {
		return 0, errors.Wrapf(err, "invalid asset ID in hashlist for %d - %s", uid, hash)
	}

	vh := md.findHashByID(uid, aid)
	if vh == "" {
		return 0, errors.Errorf("user %d has inconsistent asset cache (%d - %s)", uid, aid, hash)
	}
	if vh != hash {
		return 0, errors.Errorf("user %d - %d has different cached hash (%s - %s)", uid, aid, hash, vh)
	}
	return aid, nil
}

func (md *Memdb) resolveAndValidateID(uid int, id string) (int, string, error) {
	aid, err := ParseAssetID(id)
	if err != nil {
		return 0, "", err
	}
	hash := md.findHashByID(uid, aid)
	if hash == "" {
		return 0, "", errors.Errorf("unable to find HASH by user %d - %s)", uid, id)
	}
	a := md.ExistByHash(uid, hash)
	if a == nil {
		return 0, "", errors.Errorf("unable to relate hash %s for user %d - %s", hash, uid, id)
	}
	a1 := strconv.Itoa(aid)
	a2 := strings.Split(a.Name, ".")[0]
	if a1 != a2 {
		return 0, "", errors.Errorf("user %d has inconsistent asset ID (%s - %s), %s", uid, a1, a2, hash)
	}
	return aid, hash, nil
}

// Remove removes one asset from db
func (md *Memdb) Remove(uid int, id string, typ types.AssetIDType) error {
	var (
		aid  int
		hash string
		err  error
	)
	if typ == types.Hash {
		hash = id
		aid, err = md.resolveAndValidateHash(uid, hash)
	} else {
		aid, hash, err = md.resolveAndValidateID(uid, id)
	}
	if err != nil {
		return err
	}

	ymd := md.removeAssetByID(uid, aid)
	md.removeAssetByHash(uid, hash)
	md.removeHashListByHash(uid, int(ymd[0])+common.StartYear, int(ymd[1]), int(ymd[2]), hash)
	return nil
}

// Lock locks the memdb
func (md *Memdb) Lock() {
	md.lock.Lock()
}

// Unlock unlocks the memdb
func (md *Memdb) Unlock() {
	md.lock.Unlock()
}

func (md *Memdb) mkAssets(day int, assets []memAsset) types.Day {
	dayHash := []byte{}
	d := types.Day{Day: day + 1}
	d.Assets = make([]types.Asset, len(assets))
	for i, a := range assets {
		d.Assets[i] = a.ToAsset()
		dayHash = append(dayHash, []byte(a.Hash)...)
	}
	if len(dayHash) != 0 {
		d.Hash = fmt.Sprintf("%x", sha1.Sum(dayHash))
	}
	return d
}

func (md *Memdb) getAssetsByDay(uid, year, month, day int) []memAsset {
	recordU, ok := md.assetsList[uid]
	if !ok {
		return nil
	}
	recordY, ok := recordU[year]
	if !ok {
		return nil
	}
	return recordY[month][day]
}

// GetAssetsByDay returns asset by day
func (md *Memdb) GetAssetsByDay(uid, year, month, day int) types.Day {
	month--
	day--
	assets := md.getAssetsByDay(uid, year, month, day)
	if assets == nil {
		return types.Day{Day: day + 1, Assets: []types.Asset{}}
	}
	return md.mkAssets(day, assets)
}

// GetAssetsByMonth returns asset by month
func (md *Memdb) GetAssetsByMonth(uid, year, month int) types.Month {
	month--
	m := types.Month{Month: month + 1, Days: []types.Day{}}

	recordU, ok := md.assetsList[uid]
	if !ok {
		return m
	}
	recordY, ok := recordU[year]
	if !ok {
		return m
	}

	monthHash := []byte{}
	for i, da := range recordY[month] {
		if da == nil || len(da) == 0 {
			continue
		}
		d := md.mkAssets(i, da)
		m.Days = append(m.Days, d)
		monthHash = append(monthHash, []byte(d.Hash)...)
	}
	if len(monthHash) != 0 {
		m.Hash = fmt.Sprintf("%x", sha1.Sum(monthHash))
	}
	return m
}

// GetAssetsByYear returns all assets in one year
func (md *Memdb) GetAssetsByYear(uid, year int, dayDetail bool) types.Year {
	y := types.Year{Year: year, Months: []types.Month{}}

	recordU, ok := md.assetsList[uid]
	if !ok {
		return y
	}
	recordY, ok := recordU[year]
	if !ok {
		return y
	}

	yearHash := []byte{}
	for m := range recordY {
		if recordY[m] == nil || len(recordY[m]) == 0 {
			continue
		}
		month := types.Month{Month: m + 1, Days: []types.Day{}}
		monthHash := []byte{}

		for d, da := range recordY[m] {
			if recordY[m][d] == nil || len(recordY[m][d]) == 0 {
				continue
			}
			var day types.Day
			if dayDetail {
				day = md.mkAssets(d, da)
				month.Days = append(month.Days, day)
			} else {
				hash := []byte{}
				for _, a := range recordY[m][d] {
					hash = append(hash, []byte(a.Hash)...)
				}
				day.Hash = fmt.Sprintf("%x", sha1.Sum(hash))
			}
			monthHash = append(monthHash, []byte(day.Hash)...)
		}
		if len(monthHash) == 0 {
			continue
		}
		month.Hash = fmt.Sprintf("%x", sha1.Sum(monthHash))
		y.Months = append(y.Months, month)

		yearHash = append(yearHash, []byte(month.Hash)...)
	}

	if len(yearHash) != 0 {
		y.Hash = fmt.Sprintf("%x", sha1.Sum(yearHash))
	}
	return y
}

// GetAssetsByYears returns all assets in all year
func (md *Memdb) GetAssetsByYears(uid int, detail bool) types.Years {
	years := types.Years{Years: []types.Year{}}
	recordU, ok := md.assetsList[uid]
	if !ok {
		return years
	}

	ys := []int{}
	for y := range recordU {
		ys = append(ys, y)
	}
	sort.Ints(ys)

	yearsHash := []byte{}
	for _, y := range ys {
		year := md.GetAssetsByYear(uid, y, detail)
		if len(year.Months) == 0 {
			continue
		}
		years.Years = append(years.Years, year)
		yearsHash = append(yearsHash, []byte(year.Hash)...)
	}

	if len(yearsHash) != 0 {
		years.Hash = fmt.Sprintf("%x", sha1.Sum(yearsHash))
	}

	return years
}

// GetDayAssets returns list of photos in one day
func (md *Memdb) GetDayAssets(uid, year, month, day int) (names []string) {
	recordU, ok := md.assetsList[uid]
	if !ok {
		return
	}
	recordY, ok := recordU[year]
	if !ok {
		return
	}

	for _, a := range recordY[month-1][day-1] {
		names = append(names, a.Name)
	}
	return
}

// GetDays returns list of days having photos
func (md *Memdb) GetDays(uid, year, month int) (days []int) {
	recordU, ok := md.assetsList[uid]
	if !ok {
		return
	}
	recordY, ok := recordU[year]
	if !ok {
		return
	}

	for i, da := range recordY[month-1] {
		if da == nil || len(da) == 0 {
			continue
		}
		days = append(days, i+1)
	}
	return
}

// GetMonths returns list of months having photos
func (md *Memdb) GetMonths(uid, year int) (months []int) {
	recordU, ok := md.assetsList[uid]
	if !ok {
		return
	}
	recordY, ok := recordU[year]
	if !ok {
		return
	}

	for m := range recordY {
		if recordY[m] == nil || len(recordY[m]) == 0 {
			continue
		}
		found := false
		for d := range recordY[m] {
			if recordY[m][d] == nil || len(recordY[m][d]) == 0 {
				continue
			}
			found = true
			break
		}
		if found {
			months = append(months, m+1)
		}
	}
	return
}

// GetYears returns list of years having photos
func (md *Memdb) GetYears(uid int) (years []int) {
	recordU, ok := md.assetsList[uid]
	if !ok {
		return
	}

	for y := range recordU {
		years = append(years, y)
	}
	sort.Ints(years)
	return
}

// ListAssetsCountByDate returns all assets by date
func (md *Memdb) ListAssetsCountByDate(uid int) *[]types.AssetsByDay {
	allAssets := []types.AssetsByDay{}
	recordU, ok := md.assetsList[uid]
	if !ok {
		return &allAssets
	}

	ys := []int{}
	for y := range recordU {
		ys = append(ys, y)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ys)))

	for _, y := range ys {
		recordY, ok := recordU[y]
		if !ok {
			continue
		}

		ms := []int{}
		for m := range recordY {
			if recordY[m] == nil || len(recordY[m]) == 0 {
				continue
			}
			ms = append(ms, m)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(ms)))

		for _, m := range ms {
			ds := []int{}
			dl := map[int]string{}
			for d := range recordY[m] {
				if recordY[m][d] == nil || len(recordY[m][d]) == 0 {
					continue
				}
				// skip hidden photos
				hasOne := false
				locations := []string{}
				lm := map[string]struct{}{}
				for _, a := range recordY[m][d] {
					if types.IsAssetStatus(a.Status, types.AssetStatusHidden) {
						continue
					}
					hasOne = true
					if a.Location == "" {
						continue
					}
					_, ok := lm[a.Location]
					if ok {
						continue
					}
					lm[a.Location] = struct{}{}
					locations = append(locations, a.Location)
				}
				if hasOne {
					ds = append(ds, d)
					if len(locations) != 0 {
						dl[d] = strings.Join(locations, " ,")
					}
				}
			}
			sort.Sort(sort.Reverse(sort.IntSlice(ds)))

			for _, d := range ds {
				id := types.MkAlbumDateID(y, m+1, d+1)
				allAssets = append(allAssets, types.AssetsByDay{
					ID:            id,
					Date:          id,
					Incomplete:    true,
					NumberOfItems: 0,
					Location:      dl[d],
					Assets:        []types.AssetHash{},
				})
			}
		}
	}

	return &allAssets
}

// ListAllAssetsByDate returns all assets by date
func (md *Memdb) ListAssetsByDate(uid int, y, m, d int) *types.AssetsByDay {
	id := types.MkAlbumDateID(y, m, d)
	allAssets := &types.AssetsByDay{ID: id, Date: id}
	recordU, ok := md.assetsList[uid]
	if !ok {
		logrus.Warnf("no assets for user %d", uid)
		return allAssets
	}
	recordY, ok := recordU[y]
	if !ok {
		logrus.Warnf("no assets for user %d at year %d", uid, y)
		return allAssets
	}

	m -= 1
	if recordY[m] == nil || len(recordY[m]) == 0 {
		logrus.Warnf("no assets for user %d at %d-%d", uid, y, m+1)
		return allAssets
	}

	d -= 1
	if recordY[m][d] == nil || len(recordY[m][d]) == 0 {
		logrus.Warnf("no assets for user %d at %d-%d-%d", uid, y, m+1, y+1)
		return allAssets
	}

	allAssets.Incomplete = false
	allAssets.NumberOfItems = len(recordY[m][d])

	locations := []string{}
	lm := map[string]struct{}{}
	for _, a := range recordY[m][d] {
		if types.IsAssetStatus(a.Status, types.AssetStatusHidden) {
			continue
		}
		_, ok := lm[a.Location]
		if ok {
			continue
		}
		lm[a.Location] = struct{}{}
		if a.Location != "" {
			locations = append(locations, a.Location)
		}
		allAssets.Assets = append(allAssets.Assets, a.ToAssetHash())
	}

	allAssets.Location = strings.Join(locations, " ,")
	return allAssets
}

// ListAllAssetsByStatus returns all hidden assets by date
func (md *Memdb) ListAllAssetsByStatus(uid int, flag types.AssetStatus) *[]types.AssetsByDay {
	allAssets := []types.AssetsByDay{}
	recordU, ok := md.assetsList[uid]
	if !ok {
		return &allAssets
	}

	ys := []int{}
	for y := range recordU {
		ys = append(ys, y)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ys)))

	for _, y := range ys {
		recordY, ok := recordU[y]
		if !ok {
			continue
		}

		ms := []int{}
		for m := range recordY {
			if recordY[m] == nil || len(recordY[m]) == 0 {
				continue
			}
			ms = append(ms, m)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(ms)))

		for _, m := range ms {
			for d := len(recordY[m]) - 1; d >= 0; d-- {
				if recordY[m][d] == nil || len(recordY[m][d]) == 0 {
					continue
				}
				var as *types.AssetsByDay
				for _, a := range recordY[m][d] {
					if !types.IsAssetStatus(a.Status, flag) {
						continue
					}
					if as == nil {
						id := types.MkAlbumDateID(y, m+1, d+1)
						as = &types.AssetsByDay{ID: id, Date: id}
					}
					as.Assets = append(as.Assets, a.ToAssetHash())
				}
				if as != nil {
					as.NumberOfItems = len(as.Assets)
					allAssets = append(allAssets, *as)
				}
			}
		}
	}

	return &allAssets
}

// GetAssetHashByIDs returns all assets hashes from given ids
func (md *Memdb) GetAssetHashGroupByIDs(uid int, names []string) *[]types.AssetHashGroup {
	idMap := map[string]struct{}{}
	for _, name := range names {
		idMap[name] = struct{}{}
	}

	allAssets := []types.AssetHashGroup{}
	recordU, ok := md.assetsList[uid]
	if !ok {
		return &allAssets
	}

	ys := []int{}
	for y := range recordU {
		ys = append(ys, y)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ys)))

	for _, y := range ys {
		recordY, ok := recordU[y]
		if !ok {
			continue
		}

		for m := len(recordY) - 1; m >= 0; m-- {
			if recordY[m] == nil || len(recordY[m]) == 0 {
				continue
			}
			for d := len(recordY[m]) - 1; d >= 0; d-- {
				as := []types.AssetHash{}
				if recordY[m][d] == nil || len(recordY[m][d]) == 0 {
					continue
				}
				for _, a := range recordY[m][d] {
					if types.IsAssetStatus(a.Status, types.AssetStatusHidden) {
						continue
					}
					_, ok := idMap[a.Name]
					if !ok {
						continue
					}
					as = append(as, a.ToAssetHash())
				}
				if len(as) == 0 {
					continue
				}
				allAssets = append(allAssets, types.AssetHashGroup{
					CreateDate: fmt.Sprintf("%d-%02d-%02d", y, m+1, d+1),
					Assets:     as,
				})
			}
		}
	}

	return &allAssets
}
