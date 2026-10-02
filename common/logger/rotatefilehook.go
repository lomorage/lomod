package logger

import (
	"io"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// From https://github.com/snowzach/rotatefilehook with new API Close to close log file

// RotateFileConfig is configuration for the hook
type RotateFileConfig struct {
	Filename   string
	MaxSize    int
	MaxBackups int
	MaxAge     int
	Level      logrus.Level
	Formatter  logrus.Formatter
}

// RotateFileHook is structure for the hook
type RotateFileHook struct {
	Config    RotateFileConfig
	LogWriter io.WriteCloser
}

// NewRotateFileHook creates new rotate file hook
func NewRotateFileHook(config RotateFileConfig) (*RotateFileHook, error) {
	hook := RotateFileHook{
		Config: config,
		LogWriter: &lumberjack.Logger{
			Filename:   config.Filename,
			MaxSize:    config.MaxSize,
			MaxBackups: config.MaxBackups,
			MaxAge:     config.MaxAge,
		},
	}

	return &hook, nil
}

// Levels returns logrus levels
func (hook *RotateFileHook) Levels() []logrus.Level {
	return logrus.AllLevels[:hook.Config.Level+1]
}

// Fire write logs
func (hook *RotateFileHook) Fire(entry *logrus.Entry) (err error) {
	b, err := hook.Config.Formatter.Format(entry)
	if err != nil {
		return err
	}
	hook.LogWriter.Write(b)
	return nil
}

// Close closes the log file
func (hook *RotateFileHook) Close() error {
	return hook.LogWriter.Close()
}
