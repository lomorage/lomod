package device

import (
	"context"
	"database/sql"

	"bitbucket.org/lomoware/lomo-backend/common"
)

// GetDevicename returns device name by its device id.
func GetDevicename(db *sql.DB, deviceid int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), common.DBTimeOut)
	defer cancel()

	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("select device_name from device where id = ?")
	if err != nil {
		return "", err
	}
	defer stmt.Close()
	var devicename string
	if err := stmt.QueryRowContext(ctx, deviceid).Scan(&devicename); err != nil {
		return "", common.ErrNotExistUser
	}

	return devicename, tx.Commit()
}

// GetDeviceIDs returns device ids for given user id.
func GetDeviceIDs(ctx context.Context, tx *sql.Tx, userid int) ([]int, error) {
	stmt, err := tx.Prepare("select id from device where user_id = ?")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	deviceids := []int{}
	rows, err := stmt.QueryContext(ctx, userid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		deviceid := 0
		err := rows.Scan(&deviceid)
		if err != nil {
			return nil, common.ReturnCheckErrNoRows(err)
		}
		deviceids = append(deviceids, deviceid)
	}
	if err := rows.Err(); err != nil {
		return nil, common.ReturnCheckErrNoRows(err)
	}
	return deviceids, err
}
