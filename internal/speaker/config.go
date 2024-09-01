package speaker

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

type Config struct {
	AnycastIP string `yaml:"anycast_ip"`
	ASN       uint32 `yaml:"asn"`
	Neighbors []struct {
		Address string `yaml:"address"`
		ASN     uint32 `yaml:"asn"`
	} `yaml:"neighbors"`
	HealthCheckURL  string  `yaml:"health_check_url"`
	UpdateFIBMetric *uint32 `yaml:"update_fib_metric"`
}

type LogLevel string

const (
	Panic LogLevel = "panic"
	Fatal LogLevel = "fatal"
	Error LogLevel = "error"
	Warn  LogLevel = "warn"
	Info  LogLevel = "info"
	Debug LogLevel = "debug"
	Trace LogLevel = "trace"
)

func (l *LogLevel) String() string {
	return string(*l)
}

func (l *LogLevel) Levels() map[string]struct{} {
	return map[string]struct{}{
		string(Panic): {},
		string(Fatal): {},
		string(Error): {},
		string(Warn):  {},
		string(Info):  {},
		string(Debug): {},
		string(Trace): {},
	}
}

func (l *LogLevel) Set(s string) error {
	levels := l.Levels()
	if _, ok := levels[s]; ok {
		*l = LogLevel(s)
	} else {
		return fmt.Errorf("unknown field value: %s", s)
	}
	return nil
}

func (l *LogLevel) Type() string {
	return "enum"
}

func (l *LogLevel) LrLevel() logrus.Level {
	switch *l {
	case Panic:
		return logrus.PanicLevel
	case Fatal:
		return logrus.FatalLevel
	case Error:
		return logrus.ErrorLevel
	case Warn:
		return logrus.WarnLevel
	case Info:
		return logrus.InfoLevel
	case Debug:
		return logrus.DebugLevel
	case Trace:
		return logrus.TraceLevel
	default:
		return logrus.InfoLevel
	}
}
