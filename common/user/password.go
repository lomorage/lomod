package user

import (
	"context"
	"crypto/subtle"
	"database/sql"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/security"
)

// VerifyPassword checks a plain-text password, as sent by clients that can't hash it the way
// the apps do (e.g. WebDAV Basic auth), against username's account. It hashes it exactly like
// the apps (security.EncryptPassword) and compares with the stored value. Returns the user id.
func VerifyPassword(ctx context.Context, tx *sql.Tx, username, password string) (int, error) {
	userid, stored, err := getUserIDPassword(ctx, tx, username)
	if err != nil {
		if common.IsErrNoRows(err) {
			return -1, common.ErrInvalidPasswd
		}
		return -1, err
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(security.EncryptPassword(username, password))) != 1 {
		return -1, common.ErrInvalidPasswd
	}
	return userid, nil
}
