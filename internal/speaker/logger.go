package speaker

import (
	"github.com/osrg/gobgp/v3/pkg/log"
	"github.com/sirupsen/logrus"
)

// implement github.com/osrg/gobgp/v3/pkg/log/Logger interface
type Logger struct {
	logger *logrus.Logger
}

func NewLogger() *Logger {
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		DisableColors: false,
		FullTimestamp: true,
	})
	logger.SetLevel(logrus.DebugLevel)
	return &Logger{
		logger: logger,
	}
}

func (l *Logger) Panic(msg string, fields log.Fields) {
	l.logger.WithFields(logrus.Fields(fields)).Panic(msg)
}

func (l *Logger) Fatal(msg string, fields log.Fields) {
	l.logger.WithFields(logrus.Fields(fields)).Fatal(msg)
}

func (l *Logger) Error(msg string, fields log.Fields) {
	l.logger.WithFields(logrus.Fields(fields)).Error(msg)
}

func (l *Logger) Warn(msg string, fields log.Fields) {
	l.logger.WithFields(logrus.Fields(fields)).Warn(msg)
}

func (l *Logger) Info(msg string, fields log.Fields) {
	l.logger.WithFields(logrus.Fields(fields)).Info(msg)
}

func (l *Logger) Debug(msg string, fields log.Fields) {
	l.logger.WithFields(logrus.Fields(fields)).Debug(msg)
}

func (l *Logger) SetLevel(level log.LogLevel) {
	l.logger.SetLevel(logrus.Level(level))
}

func (l *Logger) GetLevel() log.LogLevel {
	return log.LogLevel(l.logger.GetLevel())
}
