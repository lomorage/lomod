package user

import (
	"bitbucket.org/lomoware/lomo-backend/common/jwt"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/conf"
	"github.com/sirupsen/logrus"
)

// LoginResp contains login result
type LoginResp struct {
	Token  string
	Userid int
}

// GetUserLogin returns user's login username and password
func GetUserLogin(r *http.Request) (string, string, string, error) {
	// try header firstly
	if !strings.HasPrefix(r.Header.Get("Authorization"), common.BasicAuthPrefix) {
		return "", "", "", common.ErrNotImplementedFormat
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(r.Header.Get("Authorization"), common.BasicAuthPrefix))
	if err != nil {
		return "", "", "", err
	}
	parts := strings.SplitN(string(data), ":", 3)
	if len(parts) < 3 {
		return "", "", "", common.ErrInvalidPasswd
	}
	return parts[0], parts[1], parts[2], nil
}

func createTokens() (string, error) {
	f, err := ioutil.TempDir("", "")
	defer os.Remove(f)
	if err != nil {
		return "", err
	}
	return filepath.Base(f), nil
}

// ValidateTokenAll gets user and device all information by its token
func ValidateTokenAll(ctx context.Context, tx *sql.Tx, token string) (int, string, int, string, error) {
	stmt, err := tx.Prepare("select u.id, user_name, d.id, device_name from token as t inner join (select id, user_name from user) as u on u.id = t.user_id inner join (select id, device_name from device) as d on d.id = t.device_id where t.token = ? ")
	if err != nil {
		logrus.Warnf("prepare select token got error: %s\n", err)
		return 0, "", 0, "", err
	}
	defer stmt.Close()
	var (
		uid        int
		did        int
		username   string
		devicename string
	)
	err = stmt.QueryRowContext(ctx, token).Scan(&uid, &username, &did, &devicename)
	return uid, username, did, devicename, err
}

// ValidateAndRenewToken renew user's token by its token
func ValidateAndRenewToken(ctx context.Context, tx *sql.Tx, uid int, token, device string) (string, error) {
	userID := 0
	err := tx.QueryRowContext(ctx, "select user_id from token where user_id = ? and token = ?", uid, token).Scan(&userID)
	if err != nil {
		return "", err
	}

	if userID != uid {
		logrus.Warnf("unable to locate %d's token %s", uid, token)
		return "", common.ErrInvalidToken
	}
	return GetToken(ctx, tx, uid, device)
}

// Login opens the db, and return valid token and userid
func Login(ctx context.Context, tx *sql.Tx, username, passwd, device string, duration time.Duration) (string, int, error) {
	// create new token
	token, err := createTokens()
	if err != nil {
		return "", -1, err
	}

	userid, password, err := getUserIDPassword(ctx, tx, username)
	if err != nil {
		return "", -1, err
	}
	if password != passwd {
		return "", -1, common.ErrInvalidPasswd
	}

	deviceid, err := selectDeviceID(ctx, tx, userid, device)
	if err != nil {
		if !common.IsErrNoRows(err) {
			return "", -1, err
		}
		deviceid, err = insertDeviceID(ctx, tx, userid, device)
		if err != nil {
			return "", -1, err
		}
	}

	// insert new token
	return token, userid, updateOrInsertToken(ctx, tx, token, userid, deviceid, duration)
}

// GetToken return user's latest token
func GetToken(ctx context.Context, tx *sql.Tx, userid int, device string) (string, error) {
	deviceid, err := selectDeviceID(ctx, tx, userid, device)
	if err != nil {
		return "", err
	}
	return selectToken(ctx, tx, userid, deviceid)
}

func updateOrInsertToken(ctx context.Context, tx *sql.Tx, token string, userid, deviceid int, d time.Duration) error {
	_, err := selectToken(ctx, tx, userid, deviceid)
	if err != nil {
		if !common.IsErrNoRows(err) {
			return err
		}
		return insertToken(ctx, tx, token, userid, deviceid, d)
	}
	return updateToken(ctx, tx, token, userid, deviceid, d)
}

func selectToken(ctx context.Context, tx *sql.Tx, userid, deviceid int) (string, error) {
	stmt, err := tx.Prepare("select token from token where user_id = ? and device_id = ?")
	if err != nil {
		return "", err
	}
	defer stmt.Close()

	t := ""
	err = stmt.QueryRowContext(ctx, userid, deviceid).Scan(&t)
	return t, err
}

func updateToken(ctx context.Context, tx *sql.Tx, token string, userid, deviceid int, d time.Duration) error {
	stmt, err := tx.Prepare("update token set token = ?, create_time = ?, expire_time = ? where user_id = ? and device_id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, token, time.Now().UTC(), time.Now().UTC().Add(d), userid, deviceid)
	return err
}

func insertToken(ctx context.Context, tx *sql.Tx, token string, userid, deviceid int, d time.Duration) error {
	stmt, err := tx.Prepare("insert into token(token, user_id, device_id, create_time, expire_time) values(?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, token, userid, deviceid, time.Now(), time.Now().Add(d))
	return err
}

func selectDeviceID(ctx context.Context, tx *sql.Tx, userid int, device string) (int, error) {
	stmt, err := tx.Prepare("select id from device where user_id = ? and device_name = ?")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	deviceid := 0
	if err := stmt.QueryRowContext(ctx, userid, device).Scan(&deviceid); err != nil {
		return 0, err
	}

	return deviceid, nil
}

func insertDeviceID(ctx context.Context, tx *sql.Tx, userid int, device string) (int, error) {
	stmt, err := tx.Prepare("insert into device (user_id, device_name) values (?, ?)")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	result, err := stmt.ExecContext(ctx, userid, device)
	if err != nil {
		return 0, err
	}
	deviceid64, err := result.LastInsertId()
	return int(deviceid64), err
}

// jwtSecret is the HMAC key for JWTs. Each install gets its own random key,
// created on first start and kept in the conf table (see LoadJWTSecret).
var jwtSecret string

var errNoJWTSecret = errors.New("jwt secret not loaded")

// LoadJWTSecret loads this install's JWT signing key, creating it on first use.
func LoadJWTSecret(ctx context.Context, tx *sql.Tx) error {
	secret, err := conf.GetConfValue(ctx, tx, common.ConfJWTSecret)
	if err == nil && secret != "" {
		jwtSecret = secret
		return nil
	}
	if err != nil && !common.IsErrNoRows(err) {
		return err
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	secret = hex.EncodeToString(buf)
	if err := conf.SetConfValue(ctx, tx, common.ConfJWTSecret, secret); err != nil {
		return err
	}
	jwtSecret = secret
	return nil
}

type JWTClaim struct {
	Confidence float32 `json:"confidence"`
	Exp int64 `json:"exp"`
	IssueAt int64 `json:"iat"`
	Token string `json:"jti"`
	TokenType string `json:"token_type"`
	UserID int `json:"user_id"`
	FirstName string `json:"first_name"`
	LastName string `json:"last_name"`
	Name string `json:"name"`
	IsAdmin bool `json:"is_admin"`
	NextcloudServerAddress string `json:"nextcloud_server_address"`
	NextcloudUsername string `json:"nextcloud_username"`
	ScanDir string `json:"scan_directory"`
	SearchTopk int `json:"semantic_search_topk"`
}

// EncodeJWTClaim create new jwt claim
func EncodeJWTClaim(uid int, uname, token, tokenType string, duration time.Duration) (string, error){
	if jwtSecret == "" {
		return "", errNoJWTSecret
	}
	algorithm := jwt.HmacSha256(jwtSecret)

	claims := jwt.NewClaim()

	claims.Set("token_type", tokenType)
	claims.SetTime("exp", time.Now().Add(duration))
	claims.Set("jti", token)
	claims.Set("user_id", uid)
	claims.Set("name", uname)
	claims.Set("is_admin", false)
	claims.Set("first_name", "")
	claims.Set("last_name", "")
	claims.Set("scan_directory", "")
	claims.Set("confidence", 0.1)
	claims.Set("semantic_search_topk", 0)
	claims.Set("nextcloud_server_address", nil)
	claims.Set("nextcloud_username", nil)

	return algorithm.Encode(claims)
}

// DecodeJWTClaim checks the jwt signature and expiry, then decodes its claims
func DecodeJWTClaim(token string) (*JWTClaim, error) {
	if jwtSecret == "" {
		return nil, errNoJWTSecret
	}
	algorithm := jwt.HmacSha256(jwtSecret)
	token = strings.TrimSpace(token)
	if err := algorithm.Validate(token); err != nil {
		return nil, err
	}

	claims := &JWTClaim{}
	return claims, algorithm.DecodeWithBody(token, claims)
}