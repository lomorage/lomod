package lomoframe

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

const (
	logExt        = ".log"
	debugLogFile  = "lomoframe" + logExt
	accessLogFile = "lomoframe_access" + logExt
)

type rawFormatter struct {
}

func (rf *rawFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	return []byte(entry.Message + "\n"), nil
}

func (h *Handler) getLogDir() string {
	logdir := h.conf.LogDir
	if logdir == "" {
		logdir, _ = filepath.Split(h.conf.BaseDir)
	}
	return logdir
}

func (h *Handler) initLog() error {
	logdir := h.getLogDir()

	formatter := &logrus.TextFormatter{}
	err := h.initDebugLogger(filepath.Join(logdir, debugLogFile), formatter)
	if err != nil {
		return err
	}
	filename := filepath.Join(logdir, accessLogFile)
	logrus.Infof("access log file is %s", filename)
	h.accessLogger = logger.NewAccessLogger(logrus.New(), nil, "",
		map[string]map[string]struct{}{"/system": {http.MethodGet: {}}},
		func(string) bool { return false })
	h.accessFile, err = h.initLogger(filename, h.accessLogger.Logger, &rawFormatter{})
	return err
}

func (h *Handler) initDebugLogger(filename string, formatter logrus.Formatter) error {
	var err error
	logrus.Infof("debug log file is %s", filename)

	h.logFile, err = h.initLogger(filename, nil, formatter)
	return err
}

func (h *Handler) initLogger(filename string, lg *logrus.Logger, formatter logrus.Formatter) (*logger.RotateFileHook, error) {
	rotateFileHook, err := logger.NewRotateFileHook(logger.RotateFileConfig{
		Filename:   filename,
		MaxSize:    10,
		MaxBackups: 10,
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
	} else {
		logrus.SetFormatter(formatter)
		logrus.AddHook(rotateFileHook)
	}

	return rotateFileHook, nil
}

func (h *Handler) downloadLogs(w http.ResponseWriter, r *http.Request) {
	files := logger.SystemLogFiles
	if err := filepath.Walk(h.getLogDir(), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logrus.Warnf("accessing path %q got: %v\n", path, err)
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(info.Name()) == logExt {
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

func (h *Handler) setLogLevel(w http.ResponseWriter, r *http.Request) {
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
}
