package conf

import (
	"context"
	"database/sql"

	"bitbucket.org/lomoware/lomo-backend/common"
)

// GetConfValue get configuration value by its key.
func GetConfValue(ctx context.Context, tx *sql.Tx, key string) (string, error) {
	stmt, err := tx.Prepare("select value from conf where key = ?")
	if err != nil {
		return "", err
	}
	defer stmt.Close()

	value := ""
	err = stmt.QueryRowContext(ctx, key).Scan(&value)
	return value, err
}

// SetConfValue set configuration value and key.
func SetConfValue(ctx context.Context, tx *sql.Tx, key, value string) error {
	currValue, err := GetConfValue(ctx, tx, key)
	if err != nil {
		if !common.IsErrNoRows(err) {
			return err
		}
		stmt, err := tx.Prepare("insert into conf (key, value) values (?, ?)")
		if err != nil {
			return err
		}
		defer stmt.Close()

		_, err = stmt.ExecContext(ctx, key, value)
		return err
	}
	if currValue == value {
		return nil
	}
	stmt, err := tx.Prepare("update conf set value = ? where key = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, value, key)
	return err
}

// RemoveConfValue remove configuration value by its key.
func RemoveConfValue(ctx context.Context, tx *sql.Tx, key string) error {
	stmt, err := tx.Prepare("delete from conf where key = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, key)
	return err
}
