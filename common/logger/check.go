package logger

import (
	"github.com/sirupsen/logrus"
)

// CheckLogger is for writing check log
type CheckLogger struct {
	*logrus.Logger
}

// NewCheckLogger creates logger
func NewCheckLogger(l *logrus.Logger) *CheckLogger {
	return &CheckLogger{Logger: l}
}

// Middleware intercepts requests and log request
func (cl *CheckLogger) Write(data []byte) (int, error) {
	cl.Logger.Info(string(data))
	return len(data), nil
}
