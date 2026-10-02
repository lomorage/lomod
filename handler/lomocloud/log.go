package lomocloud

import (
	"path/filepath"

	"bitbucket.org/lomoware/lomo-backend/common/logger"
	"github.com/sirupsen/logrus"
)

const (
	logExt        = ".log"
	debugLogFile  = "lomocloud" + logExt
	accessLogFile = "lomocloud_access" + logExt
	txnTypeDB     = "DB"
	logFieldTime  = "runtime"
	logFieldError = "error"
)

type rawFormatter struct {
}

func (rf *rawFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	return []byte(entry.Message + "\n"), nil
}

func (h *Handler) getLogDir() string {
	logdir := h.logdir
	if logdir == "" {
		logdir, _ = filepath.Split(h.df)
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
	h.accessLogger = logger.NewAccessLogger(logrus.New(), h.db, "", nil, h.isMaintenance)
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
		lg.SetOutput(rotateFileHook.LogWriter)
	} else {
		logrus.SetFormatter(formatter)
		logrus.AddHook(rotateFileHook)
		logrus.SetOutput(rotateFileHook.LogWriter)
	}

	return rotateFileHook, nil
}
