package check

import (
	"database/sql"
	"path/filepath"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/exif"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"github.com/pkg/errors"
)

// GpsFileInfo is file gps information
type GpsFileInfo struct {
	Filename  string
	Longitude float64
	Latitude  float64
}

// ScanGPSInfo scan unprocess asset's GPS information
func ScanGPSInfo(dbfile string) (map[int]map[int][][][]GpsFileInfo, error) {
	db, err := sql.Open("sqlite3", dbfile)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	assetsDB, homeDirs, err := loadDBAssetsNoGPS(db)
	if err != nil {
		return nil, err
	}
	assetsFilename := map[int]map[int][][][]GpsFileInfo{}
	for uid, recordU := range assetsDB {
		u := map[int][][][]GpsFileInfo{}
		homeDir, ok := homeDirs[uid]
		if !ok {
			return nil, errors.Errorf("not found home dir for %d", uid)
		}
		for year, recordY := range recordU {
			y := [][][]GpsFileInfo{}
			for _, recordM := range recordY {
				m := [][]GpsFileInfo{}
				for _, recordD := range recordM {
					d := []GpsFileInfo{}
					for _, a := range recordD {
						master, _, err := common.GetUserPhotoMasterPreviewDirCreate(homeDir, a.Date.Year(), int(a.Date.Month()), a.Date.Day(), 0)
						if err != nil {
							return nil, err
						}
						info := GpsFileInfo{}
						info.Filename = filepath.Join(master, ext.NormalizeAssetNameString(a.Date.Year(), int(a.Date.Month()), a.Date.Day(), a.Name))
						tags, err := exif.NewTags(info.Filename, "exiftool", "")
						if err != nil {
							return nil, err
						}
						info.Longitude = tags.GetLongitude()
						info.Latitude = tags.GetLatitude()
						d = append(d, info)
					}
					if len(d) == 0 {
						continue
					}
					m = append(m, d)
				}
				if len(m) == 0 {
					continue
				}
				y = append(y, m)
			}
			if len(y) == 0 {
				continue
			}
			u[year] = y
		}
		assetsFilename[uid] = u
	}

	return assetsFilename, nil
}
