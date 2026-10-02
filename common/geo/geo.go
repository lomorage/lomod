package geo

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// CountryState is structure for country + state
type CountryState struct {
	Country    string
	State      string
	CountryLat float32
	CountryLon float32
	StateLat   float32
	StateLon   float32
}

// NormalizeCountryState normalize country and state name based on db
func NormalizeCountryState(ctx context.Context, tx *sql.Tx, csc *CountryState) error {
	if csc.Country == "" {
		return common.ErrGeoNoCountry
	}
	if csc.State == "" {
		return common.ErrGeoNoState
	}

	var countryID int
	err := tx.QueryRowContext(ctx, "select id, latitude, longitude from countries where name=?",
		csc.Country).Scan(&countryID, &csc.CountryLat, &csc.CountryLon)
	if err != nil {
		return err
	}
	return tx.QueryRowContext(ctx, "select name, latitude, longitude from states where (name=? or iso2=?) and country_id=?",
		csc.State, csc.State, countryID).Scan(&csc.State, &csc.StateLat, &csc.StateLon)
}

// SelectOrInsertCountryState insert place country and state
func SelectOrInsertCountryState(ctx context.Context, tx *sql.Tx, place CountryState) ([2]int, error) {
	var ids [2]int
	id, err := selectOrInsertPlaceCountry(ctx, tx, types.GeoLocation{
		Country: place.Country, Latitude: place.CountryLat, Longitude: place.CountryLon,
	})
	ids[0] = int(id)
	if err != nil {
		return ids, err
	}
	id, err = selectOrInsertPlaceState(ctx, tx, types.GeoLocation{
		Country: place.Country, State: place.State, Latitude: place.StateLat, Longitude: place.StateLon,
	})
	ids[1] = int(id)
	return ids, err
}

// FindCountryByAssetID looks up country in English by asset ID
func FindCountryByAssetID(ctx context.Context, tx *sql.Tx, assetID int) (country string, err error) {
	err = tx.QueryRowContext(ctx, `select value from metadata_geo where asset_id=? and name like '%`+types.MetadataCategoryGeo+
		"."+types.MetadataGeoCountry+"."+types.MetadataLangEn+"'", assetID).Scan(&country)
	return
}

// FindStateByAssetID looks up state in English by asset ID
func FindStateByAssetID(ctx context.Context, tx *sql.Tx, assetID int) (state string, err error) {
	err = tx.QueryRowContext(ctx, `select value from metadata_geo where asset_id=? and name like '%`+types.MetadataCategoryGeo+
		"."+types.MetadataGeoState+"."+types.MetadataLangEn+"'", assetID).Scan(&state)
	return
}

// IsPlaceExist returns if metadata is exist or not
func IsPlaceExist(ctx context.Context, tx *sql.Tx, id int) bool {
	existID := 0
	err := tx.QueryRowContext(ctx, "select id from album_geo where id=?", id).Scan(&existID)
	return err == nil
}

// GetGeoLocation returns geo location by its ID
func GetGeoLocation(ctx context.Context, tx *sql.Tx, id int) (*types.GeoLocation, error) {
	place := &types.GeoLocation{}
	err := tx.QueryRowContext(ctx, `
select level, longitude, latitude, country, state, district, city, locality, neighborhood, street, substreet, poi from album_geo
where id=?
`, id).Scan(&place.Level, &place.Longitude, &place.Latitude, &place.Country, &place.State,
		&place.District, &place.City, &place.Locality, &place.Neighborhood, &place.Street, &place.SubStreet,
		&place.POI)
	return place, err
}

// GetGeoLocations returns places.
func GetGeoLocations(ctx context.Context, tx *sql.Tx, allPlaces bool) ([]types.GeoLocation, error) {
	statement := "select id, level, longitude, latitude, country, state, district, city, locality, neighborhood, street, substreet, poi from album_geo"
	if !allPlaces {
		statement += " where longitude=0.0 or latitude=0.0"
	}
	rows, err := tx.QueryContext(ctx, statement)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	places := []types.GeoLocation{}
	for rows.Next() {
		place := types.GeoLocation{}
		err = rows.Scan(&place.ID, &place.Level, &place.Longitude, &place.Latitude, &place.Country, &place.State,
			&place.District, &place.City, &place.Locality, &place.Neighborhood, &place.Street, &place.SubStreet,
			&place.POI)
		if err != nil {
			return nil, err
		}
		places = append(places, place)
	}
	return places, rows.Err()
}

// GetGeoLocationsByUser returns places which the user has been.
func GetGeoLocationsByUser(ctx context.Context, tx *sql.Tx, userID int) ([]types.GeoLocation, error) {
	statement := `
select ag.id, level, ag.longitude, ag.latitude, country, state, district, city, locality, neighborhood, street, substreet, poi
  from album_geo as ag
  inner join asset_album_geo as aag on ag.id=aag.album_id
  inner join asset as a on a.id=aag.asset_id
  inner join user as u on u.id=a.user_id
  where u.id=? group by ag.id, level, country, state, district, city, locality, neighborhood, street, substreet, poi
`
	rows, err := tx.QueryContext(ctx, statement, userID)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	places := []types.GeoLocation{}
	for rows.Next() {
		place := types.GeoLocation{}
		err = rows.Scan(&place.ID, &place.Level, &place.Longitude, &place.Latitude, &place.Country, &place.State,
			&place.District, &place.City, &place.Locality, &place.Neighborhood, &place.Street, &place.SubStreet,
			&place.POI)
		if err != nil {
			return nil, err
		}
		places = append(places, place)
	}
	return places, rows.Err()
}

// ListAssetNames returns the list of asset names in given scene
func ListAssetNames(ctx context.Context, tx *sql.Tx, userID, placeID, page, limit int) ([]string, error) {
	stmt, err := tx.Prepare(`
select a.id, a.ext_id from asset_album_geo as aag
  inner join asset as a on a.id=aag.asset_id
  inner join user as u on u.id=a.user_id
  where u.id=? and aag.album_id=? order by asset_id DESC limit ? offset ?
`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, userID, placeID, limit, page*limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names := []string{}
	for rows.Next() {
		id := 0
		eid := 0
		err = rows.Scan(&id, &eid)
		if err != nil {
			return nil, err
		}

		e, err := ext.GetExtString(eid)
		if err != nil {
			logrus.Warnf("asst %d has invalid extension: %d", id, eid)
			continue
		}
		names = append(names, strconv.Itoa(id)+"."+e)
	}

	return names, rows.Err()
}

// InsertPlaceLangs insert multiple languages for place
func InsertPlaceLangs(ctx context.Context, tx *sql.Tx, place types.MetadataForeignLang) error {
	if place.Lang == "" {
		logrus.Warnf("while inserting place %v, not found language value", place)
		return nil
	} else if place.NameEn == "" {
		logrus.Warnf("while inserting place %v, not found original english version", place)
		return nil
	} else if place.Lang == types.MetadataLangEn {
		logrus.Warnf("while inserting place %v, skip english", place)
		return nil
	}
	name := ""
	err := tx.QueryRowContext(ctx, "select name_en from album_geo_lang where lang=? and name=?",
		place.Lang, place.Name).Scan(&name)
	if err == nil {
		// exist already
		return nil
	} else if !common.IsErrNoRows(err) {
		return errors.Wrapf(err, "while find place geo lang: %v", place)
	}
	_, err = tx.ExecContext(ctx, "insert into album_geo_lang (lang, name, name_en) values (?, ?, ?)",
		place.Lang, place.Name, place.NameEn)
	return err
}

// SelectOrInsertPlace inserts place' metadata.
func SelectOrInsertPlace(ctx context.Context, tx *sql.Tx, place types.GeoLocation) ([]int, error) {
	ids := make([]int, 9)
	id, err := selectOrInsertPlaceCountry(ctx, tx, place)
	if err != nil {
		return ids, err
	}
	ids[0] = int(id)
	id, err = selectOrInsertPlaceState(ctx, tx, place)
	if err != nil {
		return ids, err
	}
	ids[1] = int(id)
	id, err = selectOrInsertPlaceDistrict(ctx, tx, place)
	if err != nil {
		return ids, err
	}
	ids[2] = int(id)
	id, err = selectOrInsertPlaceCity(ctx, tx, place)
	if err != nil {
		return ids, err
	}
	ids[3] = int(id)
	id, err = selectOrInsertPlaceLocality(ctx, tx, place)
	if err != nil {
		return ids, err
	}
	ids[4] = int(id)
	id, err = selectOrInsertPlaceNeighbor(ctx, tx, place)
	if err != nil {
		return ids, err
	}
	ids[5] = int(id)
	id, err = selectOrInsertPlaceStreet(ctx, tx, place)
	if err != nil {
		return ids, err
	}
	ids[6] = int(id)
	id, err = selectOrInsertPlaceSubStreet(ctx, tx, place)
	if err != nil {
		return ids, err
	}
	ids[7] = int(id)
	id, err = selectOrInsertPlacePOI(ctx, tx, place)
	ids[8] = int(id)
	return ids, err
}

func selectOrInsertPlaceCountry(ctx context.Context, tx *sql.Tx, place types.GeoLocation) (int64, error) {
	if place.Country == "" {
		return 0, nil
	}
	return selectOrInsertPlaceExec(ctx, tx,
		"country=? and level=?", []interface{}{place.Country, types.MetadataGeoLevelCountry},
		types.GeoLocation{Level: types.MetadataGeoLevelCountry, Country: place.Country,
			Longitude: place.Longitude, Latitude: place.Latitude})
}

func selectOrInsertPlaceState(ctx context.Context, tx *sql.Tx, place types.GeoLocation) (int64, error) {
	if place.State == "" {
		return 0, nil
	}
	return selectOrInsertPlaceExec(ctx, tx,
		"state=? and country=? and level=?",
		[]interface{}{place.State, place.Country, types.MetadataGeoLevelState},
		types.GeoLocation{Level: types.MetadataGeoLevelState, Country: place.Country, State: place.State,
			Longitude: place.Longitude, Latitude: place.Latitude})
}

func selectOrInsertPlaceDistrict(ctx context.Context, tx *sql.Tx, place types.GeoLocation) (int64, error) {
	if place.District == "" {
		return 0, nil
	}
	return selectOrInsertPlaceExec(ctx, tx,
		"district=? and state=? and country=? and level=?",
		[]interface{}{place.District, place.State, place.Country, types.MetadataGeoLevelDistrict},
		types.GeoLocation{Level: types.MetadataGeoLevelDistrict, Country: place.Country, State: place.State,
			District: place.District, Longitude: place.Longitude, Latitude: place.Latitude})
}

func selectOrInsertPlaceCity(ctx context.Context, tx *sql.Tx, place types.GeoLocation) (int64, error) {
	if place.City == "" {
		return 0, nil
	}
	return selectOrInsertPlaceExec(ctx, tx,
		"city=? and country=? and state=? and level=?",
		[]interface{}{place.City, place.Country, place.State, types.MetadataGeoLevelCity},
		types.GeoLocation{Level: types.MetadataGeoLevelCity, Country: place.Country, State: place.State,
			District: place.District, City: place.City, Longitude: place.Longitude, Latitude: place.Latitude})
}

func selectOrInsertPlaceLocality(ctx context.Context, tx *sql.Tx, place types.GeoLocation) (int64, error) {
	if place.Locality == "" {
		return 0, nil
	}
	return selectOrInsertPlaceExec(ctx, tx,
		"locality=? and city=? and country=? and state=? and level=?",
		[]interface{}{place.Locality, place.City, place.Country, place.State, types.MetadataGeoLevelLocality},
		types.GeoLocation{Level: types.MetadataGeoLevelLocality, Country: place.Country, State: place.State,
			District: place.District, City: place.City, Locality: place.Locality,
			Longitude: place.Longitude, Latitude: place.Latitude})
}

func selectOrInsertPlaceNeighbor(ctx context.Context, tx *sql.Tx, place types.GeoLocation) (int64, error) {
	if place.Neighborhood == "" {
		return 0, nil
	}
	return selectOrInsertPlaceExec(ctx, tx,
		"neighborhood=? and country=? and state=? and city=? and level=?",
		[]interface{}{place.Neighborhood, place.Country, place.State, place.City, types.MetadataGeoLevelNeighborhood},
		types.GeoLocation{Level: types.MetadataGeoLevelNeighborhood, Country: place.Country, State: place.State,
			District: place.District, City: place.City, Locality: place.Locality, Neighborhood: place.Neighborhood,
			Longitude: place.Longitude, Latitude: place.Latitude})
}

func selectOrInsertPlaceStreet(ctx context.Context, tx *sql.Tx, place types.GeoLocation) (int64, error) {
	if place.Street == "" {
		return 0, nil
	}
	return selectOrInsertPlaceExec(ctx, tx,
		"street=? and country=? and state=? and city=? and level=?",
		[]interface{}{place.Street, place.Country, place.State, place.City, types.MetadataGeoLevelStreet},
		types.GeoLocation{Level: types.MetadataGeoLevelStreet, Country: place.Country, State: place.State,
			District: place.District, City: place.City, Locality: place.Locality, Neighborhood: place.Neighborhood,
			Street: place.Street, Longitude: place.Longitude, Latitude: place.Latitude})
}

func selectOrInsertPlaceSubStreet(ctx context.Context, tx *sql.Tx, place types.GeoLocation) (int64, error) {
	if place.SubStreet == "" {
		return 0, nil
	}
	return selectOrInsertPlaceExec(ctx, tx,
		"substreet=? and country=? and state=? and city=? and street=? and level=?",
		[]interface{}{place.SubStreet, place.Country, place.State, place.City, place.Street, types.MetadataGeoLevelSubStreet},
		types.GeoLocation{Level: types.MetadataGeoLevelSubStreet, Country: place.Country, State: place.State,
			District: place.District, City: place.City, Locality: place.Locality, Neighborhood: place.Neighborhood,
			Street: place.Street, SubStreet: place.SubStreet, Longitude: place.Longitude, Latitude: place.Latitude})
}

func selectOrInsertPlacePOI(ctx context.Context, tx *sql.Tx, place types.GeoLocation) (int64, error) {
	if place.POI == "" {
		return 0, nil
	}
	return selectOrInsertPlaceExec(ctx, tx,
		"poi=? and country=? and state=? and city=? and street=? and level=?",
		[]interface{}{place.POI, place.Country, place.State, place.City, place.Street, types.MetadataGeoLevelPOI},
		types.GeoLocation{Level: types.MetadataGeoLevelPOI, Country: place.Country, State: place.State,
			District: place.District, City: place.City, Locality: place.Locality, Neighborhood: place.Neighborhood,
			Street: place.Street, SubStreet: place.SubStreet, POI: place.POI,
			Longitude: place.Longitude, Latitude: place.Latitude})
}

func selectOrInsertPlaceExec(ctx context.Context, tx *sql.Tx, query string, queryArgs []interface{},
	place types.GeoLocation) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, "select id from album_geo where "+query, queryArgs...).Scan(&id)
	if err == nil {
		// exist already
		return id, nil
	} else if !common.IsErrNoRows(err) {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, "insert into album_geo "+
		"(level, country, state, district, city, locality, neighborhood, street, substreet, poi, latitude, longitude, create_time) "+
		"values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		place.Level, place.Country, place.State, place.District, place.City, place.Locality, place.Neighborhood, place.Street,
		place.SubStreet, place.POI, place.Latitude, place.Longitude, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdatePlaces update places' metadata.
func UpdatePlaces(ctx context.Context, tx *sql.Tx, place types.GeoLocation) error {
	if !IsPlaceExist(ctx, tx, place.ID) {
		return errors.Errorf("place is not in database: %v", place)
	}

	_, err := tx.ExecContext(ctx, "update album_geo set longitude=?, latitude=? where id=?",
		place.Longitude, place.Latitude, place.ID)
	return err
}

// GetPlacesByAssetID returns associated place album IDs by asset ID
func GetPlacesByAssetID(ctx context.Context, tx *sql.Tx, assetID int) ([]int, error) {
	rows, err := tx.QueryContext(ctx, "select album_id from asset_album_geo where asset_id=?", assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []int{}
	for rows.Next() {
		id := 0
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// AssociateAssetWithPlaces add given asset into list of geo place
func AssociateAssetWithPlaces(ctx context.Context, tx *sql.Tx, assetID int, placeIDs []int) error {
	statement := "insert OR IGNORE into asset_album_geo (asset_id, album_id, create_time) values "
	times := []interface{}{}
	aid := strconv.Itoa(assetID)
	places := []string{}
	for _, id := range placeIDs {
		if id == 0 {
			// skip not found place ID
			continue
		}
		times = append(times, time.Now().UTC())
		places = append(places, "("+aid+","+strconv.Itoa(id)+", ?)")
	}
	if len(places) == 0 {
		return errors.Errorf("empty place IDs %v for asset %d", placeIDs, assetID)
	}
	stmt, err := tx.Prepare(statement + strings.Join(places, ","))
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, times...)
	return err
}

// DeAssociateAsset remove given asset from geo place album
func DeAssociateAsset(ctx context.Context, tx *sql.Tx, assetID int) error {
	_, err := tx.ExecContext(ctx, "delete from asset_album_geo where asset_id=?", assetID)
	return err
}

// GetAssetsCountInPlace returns assets count in one place
func GetAssetsCountInPlace(ctx context.Context, tx *sql.Tx, userID, albumID int) (count int, err error) {
	statement := `
select count(aag.asset_id) from album_geo as ag
  inner join asset_album_geo as aag on ag.id=aag.album_id
  inner join asset as a on a.id=aag.asset_id
  inner join user as u on u.id=a.user_id
  where u.id=? and ag.id=?
`
	err = tx.QueryRowContext(ctx, statement, userID, albumID).Scan(&count)
	return
}

// GetAssetsInPlace returns assets in one place
func GetAssetsInPlace(ctx context.Context, tx *sql.Tx, userID, albumID, count int) ([]string, error) {
	statement := `
select aag.asset_id, a.ext_id from album_geo as ag
  inner join asset_album_geo as aag on ag.id=aag.album_id
  inner join asset as a on a.id=aag.asset_id
  inner join user as u on u.id=a.user_id
  where u.id=? and ag.id=? and a.ext_id in (?,?,?)`
	if count != 0 {
		statement = statement + " order by aag.asset_id DESC limit " + strconv.Itoa(count)
	}

	rows, err := tx.QueryContext(ctx, statement, userID, albumID, ext.JPG, ext.JPEG, ext.WebP)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	names := []string{}
	for rows.Next() {
		var id, eid int
		err = rows.Scan(&id, &eid)
		if err != nil {
			return nil, err
		}
		name, err := ext.MkAssetNameByID(id, eid)
		if err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
