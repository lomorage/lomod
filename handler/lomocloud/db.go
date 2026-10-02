package lomocloud

import (
	"time"

	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"github.com/sirupsen/logrus"
)

func (h *Handler) openSqliteDB() error {
	var err error
	h.dbtrace = &dbx.DBTrace{Log: h}
	h.db, err = dbx.OpenDB(h.df, h.dbtrace)
	return err
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
