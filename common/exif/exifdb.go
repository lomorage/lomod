package exif

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/pkg/errors"
	"strconv"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/ext"
)

// RawTags is raw exif tag data
type RawTags []map[string]interface{}

// NewRawTags creates new instance of exif tag
func NewRawTags(content string) (RawTags, error) {
	t := RawTags{}
	if err := json.Unmarshal([]byte(content), &t); err != nil {
		return nil, errors.Wrap(err, string(content))
	}
	return t, nil
}

// SetDateZero is to reset several fields to zero for testing purpose
func (t RawTags) SetDateZero() {
	for i, tags := range t {
		for _, key := range []string {"FileAccessDate", "FileInodeChangeDate", "FileModifyDate"} {
			tags[key] = ""
		}
		t[i] = tags
	}
}

// CameraMake is structure for camera make and model
type CameraMake struct {
	ID    int
	Make  string
	Model string
}

// ListCameraMakes list makes in the system
func ListCameraMakes(ctx context.Context, tx *sql.Tx) ([]CameraMake, error) {
	rows, err := tx.QueryContext(ctx, "select id, make, model from camera_make order by make")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	makes := []CameraMake{}
	for rows.Next() {
		var make CameraMake
		err = rows.Scan(&make.ID, &make.Make, &make.Model)
		if err != nil {
			return nil, err
		}

		makes = append(makes, make)
	}

	return makes, rows.Err()
}

// InsertOrGetCameraMakeID returns camera make ID by camera make and model, insert new one is not exist
func InsertOrGetCameraMakeID(ctx context.Context, tx *sql.Tx, maker, model string) (int, error) {
	var makeID int
	err := tx.QueryRowContext(ctx, "select id from camera_make where make=? and model=?", maker, model).Scan(&makeID)
	if err != nil {
		if !common.IsErrNoRows(err) {
			return 0, err
		}
		return AddCameraMake(ctx, tx, maker, model)
	}
	return makeID, err
}

// AddCameraMake add new camera make and return its ID
func AddCameraMake(ctx context.Context, tx *sql.Tx, maker, model string) (int, error) {
	result, err := tx.ExecContext(ctx,
		"insert into camera_make (make,model,create_time) values (?,?,?)",
		maker, model, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

// AssociateAssetWithCameraMake add given asset into one camera make
func AssociateAssetWithCameraMake(ctx context.Context, tx *sql.Tx, assetID, makeID int) error {
	_, err := tx.ExecContext(ctx,
		"insert into asset_album_camera_make (make_id, asset_id, create_time) values (?,?,?)",
		makeID, assetID, time.Now().UTC())
	return err
}

// DeAssociateAsset remove given asset from one camera make ID
func DeAssociateAsset(ctx context.Context, tx *sql.Tx, assetID int) error {
	_, err := tx.ExecContext(ctx, "delete from asset_album_camera_make where asset_id=?", assetID)
	return err
}

// GetAssetsInCameraMake returns assets in one camera make
func GetAssetsInCameraMake(ctx context.Context, tx *sql.Tx, userID, makeID, count int) ([]string, error) {
	statement := `
select aasl.asset_id, a.ext_id from asset_album_camera_make as aacm
  inner join asset as a on a.id=aacm.asset_id
  inner join user as u on u.id=a.user_id
  where u.id=? and aacm.make_id=?`
	if count != 0 {
		statement = statement + " order by aacm.asset_id DESC limit " + strconv.Itoa(count)
	}
	rows, err := tx.QueryContext(ctx, statement, userID, makeID)
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

// GetAssetEXIF returns asset's exif data
func GetAssetEXIF(ctx context.Context, tx *sql.Tx, assetID int) (*RawTags, error) {
	var content string
	err := tx.QueryRowContext(ctx, "select raw_data from asset_exif where asset_id=?", assetID).Scan(&content)
	if err != nil {
		return nil, err
	}
	var rawTags RawTags
	return &rawTags, json.Unmarshal([]byte(content), &rawTags)
}

// InsertAssetEXIF insert exif data
func InsertAssetEXIF(ctx context.Context, tx *sql.Tx, assetID int, tags RawTags) error {
	content, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		"update asset set exif=? where id=?",
		string(content), assetID)
	return err
}
