package user

import (
	"context"
	"crypto/subtle"
	"database/sql"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/security"
)

// StoredPassword returns username's stored password hash, "" if there's no such user.
func StoredPassword(ctx context.Context, tx *sql.Tx, username string) (string, error) {
	_, stored, err := getUserIDPassword(ctx, tx, username)
	if common.IsErrNoRows(err) {
		return "", nil
	}
	return stored, err
}

// PasswordMatches checks a plain-text password, as sent by clients that can't hash it the way
// the apps do (e.g. WebDAV Basic auth), against the stored hash from StoredPassword: it hashes
// it exactly like the apps (security.EncryptPassword). It hashes even for an unknown user (stored
// ""), so the time taken doesn't tell which accounts exist. Call it outside a DB transaction.
func PasswordMatches(username, password, stored string) bool {
	hashed := security.EncryptPassword(username, password)
	return stored != "" && subtle.ConstantTimeCompare([]byte(stored), []byte(hashed)) == 1
}
