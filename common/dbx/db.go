package dbx

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	nanosPerMillisec = 1000000
	traceDriverName  = "sqlite3_tracing"
)

var (
	traceAll        = os.Getenv("TEST_DEBUG") == "1"
	selectQuery     = regexp.MustCompile("select")
	insertMeta      = "insert into metadata(category, source_device, asset_id, name, value, model, version, create_time, last_modified_time) values(3, 0, 153, 'ios.vision.text'"
	insertMetaLen   = len(insertMeta)
	insertMetaQuery = regexp.MustCompile("insert into metadata") // only dump structure, not actual data
)

// LogCallback is interface for logger during trace.
type LogCallback interface {
	LogCallback(string, string, string, bool)
}

// DBTrace is structure to ease trace.
type DBTrace struct {
	Log LogCallback
}

// GetDBFile return target db file name. If file is not created, created.
func GetDBFile(basedir, dbfilename string, folderPerm os.FileMode) (string, error) {
	// create folder firstly
	vardir := common.GetVarDir(basedir)
	if err := os.MkdirAll(vardir, folderPerm); err != nil {
		return "", err
	}

	// migration to new folder hierarchy
	dbfile := filepath.Join(basedir, dbfilename)
	_, err := os.Stat(dbfile)
	if err == nil {
		// move current DB to var folder for migration purpose
		dbfileNew := filepath.Join(vardir, dbfilename)
		return dbfileNew, os.Rename(dbfile, dbfileNew)
	} else if !os.IsNotExist(err) {
		return "", err
	}

	// DB is not under base dir, now check if it is exist in var dir or not
	dbfile = filepath.Join(vardir, dbfilename)
	_, err = os.Stat(dbfile)
	if err == nil {
		return dbfile, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	f, err := os.Create(dbfile)
	if err != nil {
		return "", err
	}
	return dbfile, f.Close()
}

// OpenDB opens db with given filename.
func OpenDB(filename string, dt *DBTrace) (*sql.DB, error) {
	eventMask := sqlite3.TraceStmt | sqlite3.TraceRow | sqlite3.TraceProfile | sqlite3.TraceClose

	found := false
	for _, d := range sql.Drivers() {
		if d == traceDriverName {
			found = true
			break
		}
	}
	if !found {
		conf := &sqlite3.TraceConfig{
			EventMask:       eventMask,
			WantExpandedSQL: true,
		}
		if dt != nil {
			conf.Callback = dt.dbTraceCallback
		}
		sql.Register(traceDriverName, &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				return conn.SetTrace(conf)
			},
		})
	}

	return sql.Open(traceDriverName, filename)
}

func (dt *DBTrace) dbTraceCallback(info sqlite3.TraceInfo) int {
	if info.EventCode != sqlite3.TraceStmt && info.EventCode != sqlite3.TraceProfile {
		return 0
	}

	// Show the Statement-or-Trigger text in curly braces ('{', '}')
	// since from the *paired* ASCII characters they are
	// the least used in SQL syntax, therefore better visual delimiters.
	// Maybe show 'ExpandedSQL' the same way as 'StmtOrTrigger'.
	//
	// A known use of curly braces (outside strings) is
	// for ODBC escape sequences. Not likely to appear here.
	//
	// Template languages, etc. don't matter, we should see their *result*
	// at *this* level.
	// Strange curly braces in SQL code that reached the database driver
	// suggest that there is a bug in the application.
	// The braces are likely to be either template syntax or
	// a programming language's string interpolation syntax.

	msg := ""
	isTxn := false
	if info.ExpandedSQL != "" {
		if info.ExpandedSQL == info.StmtOrTrigger {
			msg = fmt.Sprintf("StmtOrTrigger {%s}", info.StmtOrTrigger)
		} else {
			msg = strings.ToLower(info.ExpandedSQL)
			if traceAll {
				isTxn = true
			} else if !selectQuery.MatchString(msg) {
				isTxn = true
				if insertMetaQuery.MatchString(msg) {
					msg = msg[:insertMetaLen]
				}
			}
		}
	} else if info.EventCode == sqlite3.TraceProfile {
		msg = "profiling"
	} else {
		msg = info.StmtOrTrigger
	}

	// SQLite docs as of September 6, 2016: Tracing and Profiling Functions
	// https://www.sqlite.org/c3ref/profile.html
	//
	// The profile callback time is in units of nanoseconds, however
	// the current implementation is only capable of millisecond resolution
	// so the six least significant digits in the time are meaningless.
	// Future versions of SQLite might provide greater resolution on the profiler callback.

	runTimeText := ""
	if info.RunTimeNanosec != 0 {
		if info.RunTimeNanosec%nanosPerMillisec == 0 {
			runTimeText = fmt.Sprintf("%d ms", info.RunTimeNanosec/nanosPerMillisec)
		} else {
			// unexpected: better than millisecond resolution
			runTimeText = fmt.Sprintf("%d ns!!!", info.RunTimeNanosec)
		}
	} else if info.EventCode == sqlite3.TraceProfile {
		runTimeText = "0 ms"
	}

	dbErrText := ""
	if info.DBError.Code != 0 || info.DBError.ExtendedCode != 0 {
		// skip another row available
		if info.DBError.Code != 100 {
			dbErrText = fmt.Sprintf("DB error: %#v", info.DBError)
		}
	}

	if dt.Log != nil {
		dt.Log.LogCallback(runTimeText, dbErrText, msg, isTxn)
	}
	return 0
}

// InQuery runs the function, and on error, rolls back. On success, commits.
func InQuery(db *sql.DB, fun func(ctx context.Context, tx *sql.Tx) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), common.DBTimeOut)
	defer cancel()

	for {
		if err := runQuery(ctx, db, fun); err == nil {
			return nil
		} else if !isDBLocked(err) {
			return err
		}
		logrus.Info("sqlite database is locked, sleep and try again")
		timeout := time.After(time.Second)
		select {
		case <-timeout:
		case <-ctx.Done():
		}
	}
}

func runQuery(ctx context.Context, db *sql.DB, fun func(ctx context.Context, tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := fun(ctx, tx); err != nil {
		if err2 := tx.Rollback(); err2 != nil {
			err = errors.Wrapf(err, "Rollback: %v", err2)
		}

		return err
	}

	for {
		if err := tx.Commit(); err == nil {
			return nil
		} else if !isDBLocked(err) {
			return err
		}
		logrus.Info("sqlite database is locked during commit, sleep and try again")
		timeout := time.After(time.Second)
		select {
		case <-timeout:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func isDBLocked(err error) bool {
	if strings.Contains(strings.ToLower(err.Error()), "database is locked") {
		return true
	}
	serr, ok := err.(sqlite3.Error)
	if !ok {
		return false
	}
	if serr.Code == sqlite3.ErrLocked || serr.Code == sqlite3.ErrBusy {
		return true
	}
	return false
}
