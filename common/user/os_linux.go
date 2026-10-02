package user

import (
	"os/user"
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"github.com/sirupsen/logrus"
)

// md5 selects MD5-crypt ($1$) in `openssl passwd`.
const md5 = "1"

func genOSPasswd(algo, passwd, salt string) (string, error) {
	n := "openssl"
	args := []string{"passwd", "-" + algo}
	if salt != "" {
		args = append(args, "-salt", salt)
	}
	args = append(args, passwd)
	out, err := cmd.Run(n, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// CreateOSUser check if user exist or not, if not, create user.
func CreateOSUser(username, passwd, home string) error {
	_, err := user.Lookup(username)
	if err == nil {
		return nil
	}
	if !strings.HasPrefix(err.Error(), "user: unknown user ") {
		return err
	}

	newPass, err := genOSPasswd(md5, passwd, "")
	if err != nil {
		return err
	}
	return cmd.ExecWithSudo("useradd", "-p", newPass, "-d", home, "-G", common.GetLomoGroupName(), username)
}

// DeleteOSUser deletes user
func DeleteOSUser(username string) error {
	return cmd.ExecWithSudo("userdel", "-r", "-f", username)
}

// CheckAndCreateUsers check if users are exist or not, and create new one if not exist
func CheckAndCreateUsers(users []*User) error {
	for _, u := range users {
		_, err := user.Lookup(u.Name)
		if err == nil {
			continue
		}
		_, ok := err.(user.UnknownUserError)
		if !ok {
			logrus.Warnf("unknown user %s: %s", u.Name, err)
			continue
		}

		if err := CreateOSUser(u.Name, u.Password, u.HomeDir); err != nil {
			return err
		}
	}
	return nil
}

// CheckAndCreateGroup check if group is exist or not, and create new one if not exist
func CheckAndCreateGroup(name string) error {
	_, err := user.LookupGroup(name)
	if err == nil {
		return nil
	}
	_, ok := err.(user.UnknownGroupError)
	if !ok {
		return err
	}

	return cmd.ExecWithSudo("groupadd", name)
}

func deleteOSGroup(name string) error {
	return cmd.ExecWithSudo("groupdel", name)
}

// AddUserIntoGroup adds OS user into OS group
func AddUserIntoGroup(username, groupname string) error {
	return cmd.ExecWithSudo("usermod", "-a", "-G", groupname, username)
}

// Chown changes owner
func Chown(username, group, dir string) error {
	return cmd.ChownDir(username+":"+group, dir)
}

// Chmod changes mod
func Chmod(dir, mod string) error {
	return cmd.ExecWithSudo("chmod", mod, dir)
}
