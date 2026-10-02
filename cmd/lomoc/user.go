package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/security"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/manifoldco/promptui"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func resetPassword(ctx *cli.Context) error {
	if len(ctx.Args()) != 2 {
		return errors.New("usage: [user name] [password]")
	}
	prompt := promptui.Select{
		Label: "Confirm reset password",
		Items: []string{"Yes", "No"},
	}

	idx, _, err := prompt.Run()
	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return err
	}
	if idx == 1 {
		fmt.Println("skip password reset")
		return nil
	}

	username := ctx.Args()[0]
	passwd := security.EncryptPassword(username, ctx.Args()[1])

	db, err := sql.Open("sqlite3", ctx.String("db"))
	if err != nil {
		return err
	}
	defer db.Close()

	return dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		//return home dir if request is from localhost
		res, err := tx.Exec("update user set password = ? where user_name = ?", passwd, username)
		if err != nil {
			return errors.Wrap(err, "update password in db")
		}
		count, err := res.RowsAffected()
		if err != nil {
			return errors.Wrapf(err, "check affected rows")
		}
		if count != 1 {
			return errors.Errorf("password change impact %d user, please contact support", count)
		}
		return nil
	})
}

func resetHomeDir(ctx *cli.Context) error {
	userName := ""
	switch len(ctx.Args()) {
	case 1:
	case 2:
		userName = ctx.Args()[1]
	default:
		return errors.New("Invalid number of arguments are provided. Usage: [new media mount dir] ([user name]). If username is not specified, it will reset all users' home directory")
	}
	return resetDir(ctx.Args()[0], userName, ctx.String("db"), true)
}

func resetBackupDir(ctx *cli.Context) error {
	userName := ""
	switch len(ctx.Args()) {
	case 1:
	case 2:
		userName = ctx.Args()[1]
	default:
		return errors.New("Invalid number of arguments are provided. Usage: [new media mount dir] ([user name]). If username is not specified, it will reset all users' home directory")
	}
	return resetDir(ctx.Args()[0], userName, ctx.String("db"), false)
}

func resetDir(newMountDir, userName, dbFilename string, isHomeDir bool) error {
	if !filepath.IsAbs(newMountDir) {
		return errors.New("Please use absolute path for new media mount dir")
	}
	db, err := sql.Open("sqlite3", dbFilename)
	if err != nil {
		return err
	}
	defer db.Close()

	return dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		// check if assets are migrated or not. If not prompt user to
		if userName != "" {
			return resetDirUser(ctx, tx, newMountDir, userName, isHomeDir)
		}
		var admin int
		rows, err := tx.QueryContext(ctx, "select user_name, admin from user")
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			err = rows.Scan(&userName, &admin)
			if err != nil {
				return err
			}
			if user.IsBotUser(admin) {
				continue
			}
			err = resetDirUser(ctx, tx, newMountDir, userName, isHomeDir)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func resetDirUser(ctx context.Context, tx *sql.Tx, newMountDir, userName string, isHomeDir bool) error {
	newDir := filepath.Join(newMountDir, userName)
	master, _ := common.GetUserPhotoDir(newDir)
	_, err := os.Stat(master)
	if err != nil {
		return err
	}
	statement := "update user set "
	if isHomeDir {
		statement += "home_dir=?"
	} else {
		statement += "backup_dir=?"
	}
	statement += " where user_name=?"

	_, err = tx.ExecContext(ctx, statement, newDir, userName)
	return err
}
