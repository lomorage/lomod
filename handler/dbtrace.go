package handler

import (
	"context"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common/dbx"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	_ "github.com/mattn/go-sqlite3"
)

func (h *Handler) openSqliteDB() error {
	// add cache=shared and max open connections to avoid 'database is locked'
	var err error
	if h.dbtrace == nil {
		h.dbtrace = &dbx.DBTrace{Log: h}
	}
	h.db, err = dbx.OpenDB(h.conf.DbFilename+"?cache=shared", h.dbtrace)
	if err != nil {
		return err
	}

	h.db.SetMaxOpenConns(1)

	_, err = h.db.ExecContext(context.Background(), `PRAGMA busy_timeout = 5000;`)
	if err != nil {
		return errors.Wrap(err, "PRAGMA busy_timeout = 5000;")
	}
	_, err = h.db.ExecContext(context.Background(), `PRAGMA synchronous = NORMAL;`)
	if err != nil {
		return errors.Wrap(err, "PRAGMA synchronous = NORMAL")
	}
	return nil
}

// LogCallback is log callback function for db trace.
func (h *Handler) LogCallback(runTimeText, dbErrText, msg string, isTxn bool) {
	fields := logrus.Fields{}
	if runTimeText != "" {
		fields[logFieldTime] = runTimeText
	}
	if dbErrText != "" {
		fields[logFieldError] = dbErrText
	}

	if dbErrText != "" {
		logrus.WithFields(fields).Warn(msg)
	} else {
		logrus.WithFields(fields).Trace(msg)
	}

	if isTxn && h.accessLogger != nil {
		h.accessLogger.Infof("%s [%d:%d]: %s", txnTypeDB, time.Now().Minute(), time.Now().Second(), msg)
	}
}
