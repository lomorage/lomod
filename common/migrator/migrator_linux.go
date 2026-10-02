package migrator

import (
	// use go-sqlite3
	"strings"

	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// UserPermission is to migrate user permission
func UserPermission(dbFile, lomodBaseDir string) error {
	// TODO: remove the deprecated codes
	/*
		db, err := sql.Open("sqlite3", dbFile)
		if err != nil {
			return err
		}
		defer db.Close()

		userNames := []string{common.LomodUserName}
		homeDirs := []string{lomodBaseDir}
		users := []*user.User{{Name: common.LomodUserName, Password: common.LomodUserName, HomeDir: lomodBaseDir}}
		if err := dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
			//return home dir if request is from localhost
			us, err := user.ListUsers(ctx, tx)
			if err != nil {
				return err
			}
			for _, u := range us.Users {
				if u.BotUser {
					users = append(users, u)
					userNames = append(userNames, u.Name)
				} else {
					userNames = append(userNames, common.LomodUserName)
				}
				homeDirs = append(homeDirs, u.HomeDir)
			}
			return nil
		}); err != nil {
			return err
		}

		if err := user.CheckAndCreateGroup(common.LomoGroupName); err != nil {
			return err
		}
		if err := user.CheckAndCreateUsers(users); err != nil {
			return err
		}

		for i, n := range userNames {
			if err := chown(n, homeDirs[i]); err != nil {
				logrus.Warnf("while chown %s, got: %s", n, err)
				continue
			}
			if err := chmod(n, homeDirs[i]); err != nil {
				logrus.Warnf("while chmod %s, got: %s", n, err)
				continue
			}
		}
	*/
	return nil
}

func getUG(dir string) (string, string, error) {
	out, err := cmd.Run("stat", "-c", "%U %G", dir)
	if err != nil {
		return "", "", errors.Wrapf(err, "while stat %s", dir)
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), " ", 2)
	if len(parts) != 2 {
		return "", "", errors.Errorf("invalid result of stat %s result %s", dir, out)
	}
	return parts[0], parts[1], nil
}

func chown(userName, groupName, binFile, home string) error {
	if userName == "" {
		// probe user name and group name by check /opt/lomorage/bin/lomod
		var err error
		userName, groupName, err = getUG(binFile)
		if err != nil {
			return err
		}
	}

	logrus.Infof("migrate %s owner group to %s:%s", home, userName, groupName)
	if err := user.Chown(userName, groupName, home); err != nil {
		return err
	}
	logrus.Info(userName + " home directory is set to right owner")
	return nil
}
