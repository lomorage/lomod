package handler

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/dbx"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

const (
	logExt        = ".log"
	debugLogFile  = "lomod" + logExt
	accessLogFile = "lomod_access" + logExt
	backupLogFile = "lomod_backup" + logExt
	checkLogFile  = "lomod_check" + logExt
	txnTypeDB     = "DB"
	logFieldTime  = "runtime"
	logFieldError = "error"
)

type rawFormatter struct {
}

func (rf *rawFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	return []byte(entry.Message + "\n"), nil
}

func (h *Handler) initLog() error {
	formatter := &logrus.TextFormatter{DisableColors: true}
	logrus.Infof("debug log file is %s", filepath.Join(h.conf.LogDir, debugLogFile))

	if h.conf.Debug {
		logrus.SetLevel(logrus.TraceLevel)
	} else {
		logrus.SetLevel(logrus.DebugLevel)
	}

	var err error
	h.logFile, err = h.initLogger(filepath.Join(h.conf.LogDir, debugLogFile), nil, formatter)
	if err != nil {
		return err
	}

	filename := filepath.Join(h.conf.LogDir, accessLogFile)
	logrus.Infof("access log file is %s", filename)
	h.accessLogger = logger.NewAccessLogger(logrus.New(), h.db, h.conf.AdminToken, nil, h.isMaintenance)
	h.accessFile, err = h.initLogger(filename, h.accessLogger.Logger, &rawFormatter{})
	if err != nil {
		return err
	}

	filename = filepath.Join(h.conf.LogDir, backupLogFile)
	logrus.Infof("backup log file is %s", filename)
	h.backupLogger = logrus.New()
	h.backupFile, err = h.initLogger(filename, h.backupLogger, formatter)
	if err != nil {
		return err
	}

	filename = filepath.Join(h.conf.LogDir, checkLogFile)
	logrus.Infof("inconsistent check log file is %s", filename)
	h.checkLogger = logger.NewCheckLogger(logrus.New())
	h.checkFile, err = h.initLogger(filename, h.checkLogger.Logger, formatter)

	return err
}

func (h *Handler) initLogger(filename string, lg *logrus.Logger,
	formatter logrus.Formatter) (*logger.RotateFileHook, error) {
	// probe and see if log file is healthy or not
	file, err := common.CreateOrOpenFile(filename, h.conf.FilePerm)
	if err != nil {
		return nil, err
	}
	if err = file.Close(); err != nil {
		return nil, err
	}

	rotateFileHook, err := logger.NewRotateFileHook(logger.RotateFileConfig{
		Filename:   filename,
		MaxSize:    10,
		MaxBackups: 5,
		MaxAge:     7200,
		Level:      logrus.InfoLevel,
		Formatter:  formatter,
	})
	if err != nil {
		return nil, err
	}

	if lg != nil {
		lg.SetFormatter(formatter)
		lg.AddHook(rotateFileHook)
		if h.conf.Debug {
			lg.SetLevel(logrus.TraceLevel)
		}
	} else {
		logrus.AddHook(rotateFileHook)
		if h.conf.HasStdout {
			logrus.SetFormatter(&logrus.TextFormatter{})
		}
	}

	return rotateFileHook, nil
}

func (h *Handler) downloadLogs(w http.ResponseWriter, r *http.Request) {
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		count, err := user.Counts(ctx, tx, false)
		if err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		return w.(*logger.ResponseLogger).Err
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	files := logger.SystemLogFiles
	if err := filepath.Walk(h.conf.LogDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logrus.Warnf("accessing path %q got: %v\n", path, err)
			return err
		}
		if info.IsDir() {
			return nil
		}
		switch filepath.Ext(info.Name()) {
		case pcapExt:
			fallthrough
		case pingExt:
			fallthrough
		case logExt:
			files = append(files, path)
		}
		return nil
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	if err := logger.WriteHTTP(files, w); err != nil {
		common.WriteError(w, err)
		return
	}
}

func (h *Handler) downloadAccessLog(w http.ResponseWriter, r *http.Request) {
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		count, err := user.Counts(ctx, tx, false)
		if err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		return w.(*logger.ResponseLogger).Err
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	if err := logger.WriteHTTP([]string{h.accessFile.Config.Filename}, w); err != nil {
		common.WriteError(w, err)
		return
	}
}

func (h *Handler) uploadLogs(w http.ResponseWriter, r *http.Request) {
	wl := w.(*logger.ResponseLogger)
	if wl.Err != nil {
		common.WriteError(w, wl.Err)
		return
	}

	f, err := os.Create(filepath.Join(h.conf.LogDir, wl.Username+".log"))
	if err != nil {
		common.WriteError(w, err)
		return
	}
	defer f.Close()
	defer r.Body.Close()

	_, err = io.Copy(f, r.Body)
	if err != nil {
		common.WriteError(w, err)
		return
	}
}

func (h *Handler) setLogLevel(w http.ResponseWriter, r *http.Request) {
	if err := dbx.InQuery(h.db, func(ctx context.Context, tx *sql.Tx) error {
		count, err := user.Counts(ctx, tx, false)
		if err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		return w.(*logger.ResponseLogger).Err
	}); err != nil {
		common.WriteError(w, err)
		return
	}

	l, err := strconv.Atoi(mux.Vars(r)["level"])
	if err != nil {
		common.WriteError(w, err)
		return
	}
	level := logrus.Level(l)
	if level > logrus.TraceLevel || level < logrus.PanicLevel {
		common.WriteError(w, common.ErrBadRequest)
		return
	}
	logrus.SetLevel(level)
	if level == logrus.TraceLevel {
		h.conf.Debug = true
	} else {
		h.conf.Debug = false
	}
}
