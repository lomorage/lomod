package migrator

import (
	"context"
	"database/sql"
	"os"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	// use go-sqlite3
	_ "github.com/mattn/go-sqlite3"
)

const (
	schemaSelect = `select latest from schema_migrations order by latest DESC limit 1;`
	schemaInsert = `insert into schema_migrations (latest, update_time) values ($1, $2);`
)

// StartLomod migrates sql for lomod application
func StartLomod(dbFile, docDir string, statements []string, dirPerm os.FileMode) error {
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		return err
	}
	defer db.Close()

	homeDirs := map[string]string{}
	err = dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		latest := 0
		freshInstall := false
		if err := tx.QueryRowContext(ctx, schemaSelect).Scan(&latest); err != nil {
			if !common.IsErrNoRows(err) && !common.IsErrNoTable(err) {
				return errors.Wrap(err, "querying latest schema migration")
			}
			freshInstall = true
		}

		curr := latest
		for ; curr < len(statements); curr++ {
			logrus.Info(statements[curr])
			if _, err := tx.ExecContext(ctx, statements[curr]); err != nil {
				return errors.Wrapf(err, "executing migration %d", latest)
			}
		}

		if freshInstall || latest != curr {
			_, err = tx.ExecContext(ctx, schemaInsert, curr, time.Now())
			if err != nil {
				return errors.Wrapf(err, "writing migration state (last value: %d)", curr)
			}
		} else {
			logrus.Info("No changes needed!")
		}
		us, err := user.ListUsers(ctx, tx)
		if err != nil {
			logrus.Errorf("list users: %v", err)
			return nil
		}
		for _, u := range us.Users {
			if u.IsBotUser() {
				continue
			}
			homeDirs[u.Name] = u.HomeDir
		}
		return nil
	})
	err2 := db.Close()
	if err != nil {
		if err2 != nil {
			return errors.Wrap(err, err2.Error())
		}
		return err
	} else if err2 != nil {
		return err2
	}

	// no need migrate home directory any more
	//migrateHomeDirPermission(dbFile, homeDirs)

	if docDir == "" {
		// skip migrate document dirs
		return nil
	}

	return copyDocuments(docDir, homeDirs, dirPerm)
}

func migrateHomeDirPermission(dbFile string, homeDirs map[string]string) {
	for _, homeDir := range homeDirs {
		/*
			if err := chmod(username, homeDir, dirPerm); err != nil {
				logrus.Errorf("chmod %s: %v", homeDir, err)
			}
		*/
		// change to the same user as lomod user
		if err := chown("", "", dbFile, homeDir); err != nil {
			logrus.Errorf("chmod %s: %v", homeDir, err)
		}
	}
}

func copyDocuments(srcDocDir string, homeDirs map[string]string, dirPerm os.FileMode) error {
	for _, homeDir := range homeDirs {
		if err := common.CopyDocs(srcDocDir, homeDir, dirPerm); err != nil {
			return err
		}
	}
	return nil
}

// StartLomocloud migrates sql for lomocloud application
func StartLomocloud(dbFile string, statements []string) error {
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		return err
	}
	defer db.Close()

	err = dbx.InQuery(db, func(ctx context.Context, tx *sql.Tx) error {
		latest := 0
		freshInstall := false
		if err := tx.QueryRowContext(ctx, schemaSelect).Scan(&latest); err != nil {
			if !common.IsErrNoRows(err) && !common.IsErrNoTable(err) {
				return errors.Wrap(err, "querying latest schema migration")
			}
			freshInstall = true
		}

		curr := latest
		for ; curr < len(statements); curr++ {
			logrus.Info(statements[curr])
			if _, err := tx.ExecContext(ctx, statements[curr]); err != nil {
				return errors.Wrapf(err, "executing migration %d", latest)
			}
		}

		if freshInstall || latest != curr {
			_, err = tx.ExecContext(ctx, schemaInsert, curr, time.Now())
			if err != nil {
				return errors.Wrapf(err, "writing migration state (last value: %d)", curr)
			}
		} else {
			logrus.Info("No changes needed!")
		}
		return nil
	})
	err2 := db.Close()
	if err != nil {
		if err2 != nil {
			return errors.Wrap(err, err2.Error())
		}
		return err
	}
	return err2
}
