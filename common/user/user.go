package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/security"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// AdminFlag means the user has admin permission
	AdminFlag = 1 << 0
	// BotFlag means the user is bot user
	BotFlag = 1 << 1
	// CastFlag means the user is chromecast
	CastFlag = 1 << 2
)

var regexUsername = regexp.MustCompile(`^[a-zA-Z]([a-zA-Z0-9_-]{0,31})$`)

// KeepaliveStatus is user
type KeepaliveStatus struct {
	IP       string
	Port     int
	LastSeen string
}

// Setting is setting configuration for each user
type Setting struct {
	WebdevDirLayout int
}

// User is response structure for each user
type User struct {
	ID        int
	admin     int
	Status    common.UserStatus
	Name      string
	Password  string
	Phone     string
	Email     string
	NickName  string
	HomeDir   string
	BackupDir string
	SubDomain string
	LastLogin string
	Keepalive map[string]KeepaliveStatus `json:",omitempty"`
	BotUser   bool
	Metadatas map[string]interface{}
}

// Validate check user information
func (u *User) Validate() error {
	if !regexUsername.MatchString(u.Name) {
		return errors.Errorf("%v (%s): must start with alphabet, and following by alpha-number or - or _.",
			common.ErrInvalidName, u.Name)
	}
	return nil
}

// SetAdminFlag set admin flag for the user
func (u *User) SetAdminFlag(flag int) {
	u.admin = flag
}

// IsBotUser return if user is bot user or not
func (u *User) IsBotUser() bool {
	return (u.admin & BotFlag) == BotFlag
}

// IsChromecast returns if user is chromecast or not
func (u *User) IsChromecast() bool {
	return (u.admin & CastFlag) == CastFlag
}

// Users is response structure for listusers request
type Users struct {
	Users []*User `json:"Users,omitempty"`
}

// Validate check user information
func (us *Users) Validate() error {
	for _, u := range us.Users {
		if err := u.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// IsExist check if given username is in db or not
// err is not nil, means user is not in DB, otherwise, it is in db
func IsExist(ctx context.Context, tx *sql.Tx, username string) (int, error) {
	stmt, err := tx.Prepare("select id from user where user_name = ?")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	var userid int
	if err := stmt.QueryRowContext(ctx, username).Scan(&userid); err != nil {
		return 0, common.ErrNotExistUser
	}

	return userid, nil
}

// IsExistByID check if given userid is in db or not
// err is not nil, means user is not in DB, otherwise, it is in db
func IsExistByID(ctx context.Context, tx *sql.Tx, userID int) (string, error) {
	stmt, err := tx.Prepare("select user_name from user where id = ?")
	if err != nil {
		return "", err
	}
	defer stmt.Close()
	userName := ""
	if err := stmt.QueryRowContext(ctx, userID).Scan(&userName); err != nil {
		return "", common.ErrNotExistUser
	}

	return userName, nil
}

// Get returns user data structure
func Get(ctx context.Context, tx *sql.Tx, userID int) (*User, error) {
	stmt, err := tx.Prepare("select user_name, home_dir, admin, metadata from user where id = ?")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	var (
		metadata string
		u        User
	)
	if err := stmt.QueryRowContext(ctx, userID).Scan(&u.Name, &u.HomeDir, &u.admin, &metadata); err != nil {
		return nil, common.ErrNotExistUser
	}
	if metadata != "" {
		if err := json.Unmarshal([]byte(metadata), &u.Metadatas); err != nil {
			return nil, err
		}
	}

	u.ID = userID
	u.BotUser = IsBotUser(u.admin)
	return &u, nil
}

// GetByName returns user data structure by name
func GetByName(ctx context.Context, tx *sql.Tx, username string) (*User, error) {
	stmt, err := tx.Prepare("select id, home_dir, admin, metadata from user where user_name = ?")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	metadata := ""
	u := User{Name: username}
	if err := stmt.QueryRowContext(ctx, username).Scan(&u.ID, &u.HomeDir, &u.admin, &metadata); err != nil {
		if common.IsErrNoRows(err) {
			return nil, common.ErrNotExistUser
		}
		return nil, err
	}
	if metadata != "" {
		if err := json.Unmarshal([]byte(metadata), &u.Metadatas); err != nil {
			return nil, err
		}
	} else {
		u.Metadatas = map[string]interface{}{}
	}

	u.BotUser = IsBotUser(u.admin)
	return &u, nil
}

func getUserIDPassword(ctx context.Context, tx *sql.Tx, username string) (int, string, error) {
	stmt, err := tx.Prepare("select id, password from user where user_name = ?")
	if err != nil {
		return 0, "", err
	}
	defer stmt.Close()

	userid := 0
	password := ""
	err = stmt.QueryRowContext(ctx, username).Scan(&userid, &password)

	return userid, password, err
}

// GetUsername returns username by its userid
func GetUsername(ctx context.Context, tx *sql.Tx, userid int) (string, error) {
	stmt, err := tx.Prepare("select user_name from user where id = ?")
	if err != nil {
		return "", err
	}
	defer stmt.Close()
	var username string
	if err := stmt.QueryRowContext(ctx, userid).Scan(&username); err != nil {
		return "", common.ErrNotExistUser
	}

	return username, nil
}

// GetUserID returns userid by its username
func GetUserID(ctx context.Context, tx *sql.Tx, username string) (int, error) {
	stmt, err := tx.Prepare("select id from user where user_name = ?")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	var userid int
	if err := stmt.QueryRowContext(ctx, username).Scan(&userid); err != nil {
		return 0, common.ErrNotExistUser
	}

	return userid, nil
}

// GetHomedir returns user's home dir
func GetHomedir(ctx context.Context, tx *sql.Tx, userid int) (string, error) {
	// check username password
	stmt, err := tx.Prepare("select home_dir from user where id = ?")
	if err != nil {
		return "", err
	}
	defer stmt.Close()
	var homedir string
	if err := stmt.QueryRowContext(ctx, userid).Scan(&homedir); err != nil {
		return "", err
	}

	return homedir, nil
}

// GetLoginDeviceName returns user's login device name
func GetLoginDeviceName(ctx context.Context, tx *sql.Tx, did int) (string, error) {
	stmt, err := tx.Prepare("select device_name from device where id = ?")
	if err != nil {
		return "", err
	}
	defer stmt.Close()
	var dname string
	if err := stmt.QueryRowContext(ctx, did).Scan(&dname); err != nil {
		return "", err
	}

	return dname, nil
}

// ListUsers returns users in the system
func ListUsers(ctx context.Context, tx *sql.Tx) (*Users, error) {
	sqlStatement := `select id, user_name, phone, email, nick_name, home_dir, backup_dir, 
	metadata, admin, status, last_login_time from user order by user_name`
	stmt, err := tx.Prepare(sqlStatement)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := &Users{Users: []*User{}}
	for rows.Next() {
		user := &User{}
		t := ""
		metadata := ""
		err := rows.Scan(&user.ID, &user.Name, &user.Phone, &user.Email, &user.NickName,
			&user.HomeDir, &user.BackupDir, &metadata, &user.admin,
			&user.Status, &t)
		if err != nil {
			return nil, err
		}
		if metadata != "" {
			if err := json.Unmarshal([]byte(metadata), &user.Metadatas); err != nil {
				return nil, err
			}
		} else {
			user.Metadatas = map[string]interface{}{}
		}
		user.BotUser = user.IsBotUser()
		user.LastLogin, err = common.ParseAndFormatTime(t)
		if err != nil {
			logrus.Warnf("while paring %s's login time, got: %v", user.Name, err)
		}
		users.Users = append(users.Users, user)
	}
	return users, rows.Err()
}

// listUsernamePassword returns users in the system
func listUsernamePassword(ctx context.Context, tx *sql.Tx) (*Users, error) {
	sqlStatement := "select user_name, password, home_dir, admin from user order by user_name"
	stmt, err := tx.Prepare(sqlStatement)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := &Users{Users: []*User{}}
	for rows.Next() {
		user := &User{}
		err := rows.Scan(&user.Name, &user.Password, &user.HomeDir, &user.admin)
		if err != nil {
			return nil, err
		}
		users.Users = append(users.Users, user)
	}
	return users, rows.Err()
}

// Counts returns number of users
func Counts(ctx context.Context, tx *sql.Tx, includeBotUser bool) (int, error) {
	count := 0
	if includeBotUser {
		stmt, err := tx.Prepare("select count(id) from user")
		if err != nil {
			return -1, err
		}
		defer stmt.Close()

		err = stmt.QueryRowContext(ctx).Scan(&count)
		return count, err
	}
	users, err := ListUsers(ctx, tx)
	if err != nil {
		return -1, err
	}
	for _, u := range users.Users {
		if u.IsBotUser() {
			continue
		}
		count++
	}
	return count, nil
}

// CreateUser adds one user
// admin is combination of different flags:
//   0/1 - admin permission or not
//   10/00 - enable samba mount or not
func CreateUser(ctx context.Context, tx *sql.Tx, u *User, sambaConf string) (err error) {
	_, err = GetUserID(ctx, tx, u.Name)
	if err == nil {
		return common.ErrDuplicate
	} else if err != common.ErrNotExistUser {
		return err
	}

	if u.IsBotUser() && !u.IsChromecast() {
		// create OS user and samba user before inserting DB entry
		// use sha1 of user's password as OS password
		passwd := security.LomoPasswdToOSPasswd(u.Password)
		if err := CreateOSUser(u.Name, passwd, u.HomeDir); err != nil {
			return err
		}

		defer func() {
			if err != nil {
				if err2 := DeleteOSUser(u.Name); err2 != nil {
					logrus.Warnf("delete os user %s: %s", u.Name, err2)
				}
			}
		}()

		if err := Chown(u.Name, common.GetLomoGroupName(), u.HomeDir); err != nil {
			return err
		}

		us, err := listUsernamePassword(ctx, tx)
		if err != nil {
			return err
		}

		if err := CreateSambaUser(u.Name, passwd); err != nil {
			return err
		}

		defer func() {
			if err != nil {
				if err2 := DeleteSambaUser(u.Name); err2 != nil {
					logrus.Warnf("delete samba user %s: %s", u.Name, err2)
				}
			}
		}()

		usc := SambaUsersConf{{Name: u.Name, Password: passwd, Path: u.HomeDir}}
		for _, ud := range us.Users {
			if !IsBotUser(ud.admin) {
				continue
			}
			usc = append(usc, SambaUserConf{Name: ud.Name, Password: security.LomoPasswdToOSPasswd(ud.Password), Path: ud.HomeDir})
		}

		if err := ReloadSamba(sambaConf, usc); err != nil {
			return err
		}
	}

	metadata := ""
	if len(u.Metadatas) > 0 {
		content, err := json.Marshal(u.Metadatas)
		if err != nil {
			return err
		}
		metadata = string(content)
	}

	stmt, err := tx.Prepare(`insert into user(user_name, password, phone, email, nick_name, 
	home_dir, backup_dir, admin, status, metadata, 
	create_time, last_modified_time, last_login_time) 
	values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	result, err := stmt.ExecContext(ctx, u.Name, u.Password, u.Phone, u.Email, u.NickName, u.HomeDir,
		u.BackupDir, u.admin, common.UserStatusUnknown, metadata,
		time.Now().UTC(), time.Now().UTC(), time.Now().UTC())
	if err != nil {
		return err
	}
	num, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if num != 1 {
		return errors.Errorf("expect insert 1 user, but actual %d", num)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	u.ID = int(id)
	return nil
}

// UpdateUser updates user email/phone/password
func UpdateUser(ctx context.Context, tx *sql.Tx, u *User) error {
	statement := "update user "
	keys := []string{"set last_modified_time = ? "}
	values := []interface{}{time.Now()}
	if u.Password != "" {
		keys = append(keys, " password = ? ")
		values = append(values, u.Password)
		// FIXME: check user type and update password for samba enabled user
		//if err := UpdateOSUserPassword(u.Name, u.Password); err != nil {
		//	return err
		//}
	}
	if u.Phone != "" {
		keys = append(keys, " phone = ? ")
		values = append(values, u.Phone)
	}
	if u.Email != "" {
		keys = append(keys, " email = ? ")
		values = append(values, u.Email)
	}
	if u.NickName != "" {
		keys = append(keys, " nick_name = ? ")
		values = append(values, u.NickName)
	}
	if len(u.Metadatas) > 1 {
		keys = append(keys, " metadata = ? ")
		content, err := json.Marshal(u.Metadatas)
		if err != nil {
			return err
		}
		values = append(values, string(content))
	}
	statement = statement + strings.Join(keys, ", ") + " where user_name = ?"
	values = append(values, u.Name)

	stmt, err := tx.Prepare(statement)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, values...)
	return err
}

// UpdateUserStatus updates user online status
func UpdateUserStatus(ctx context.Context, tx *sql.Tx, userID int, status common.UserStatus) error {
	stmt, err := tx.Prepare("update user set status = ?, last_login_time = ? where id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, status, time.Now(), userID)
	return err
}

// DeleteUser delete user by name
func DeleteUser(ctx context.Context, tx *sql.Tx, username string) error {
	stmt, err := tx.Prepare("select id from user where user_name = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	var id int
	if err := stmt.QueryRowContext(ctx, username).Scan(&id); err != nil {
		return err
	}

	if err := DeleteSambaUser(username); err != nil {
		logrus.Warnf("delete samba user %s: %s", username, err)
	}
	if err := DeleteOSUser(username); err != nil {
		logrus.Warnf("delete os user %s: %s", username, err)
	}
	if _, err := tx.ExecContext(ctx, "delete from token where user_id = ?", id); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "delete from user where user_name = ?", username)
	return err
}

// DeleteTokenByUserID delete user's old token
func DeleteTokenByUserID(ctx context.Context, tx *sql.Tx, uid int) error {
	_, err := tx.ExecContext(ctx, "delete from token where user_id = ?", uid)
	return err
}

// IsBotUser return if user is bot user or not
func IsBotUser(admin int) bool {
	return (admin & BotFlag) == BotFlag
}
