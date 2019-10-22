package testutils

import (
	"fmt"

	"gopkg.in/src-d/go-log.v1"
)

type LoggerMock struct {
	out []string
}

func (l *LoggerMock) Next() string {
	if len(l.out) == 0 {
		return ""
	}
	first := l.out[0]
	l.out[0] = ""
	l.out = l.out[1:]
	return first
}

func (l *LoggerMock) Debugf(format string, args ...interface{}) {
	l.out = append(l.out, fmt.Sprintf(format, args...))
	log.Debugf(format, args...)
}

func (l *LoggerMock) Errorf(err error, format string, args ...interface{}) {
	arguments := append([]interface{}{err}, args)
	errorFormat := fmt.Sprintf("Error %s; %s", err, format)
	l.out = append(l.out, fmt.Sprintf(errorFormat, arguments...))
	log.Errorf(err, format, args...)
}

func (l *LoggerMock) Infof(format string, args ...interface{}) {
	l.out = append(l.out, fmt.Sprintf(format, args...))
	log.Infof(format, args...)
}

func (l *LoggerMock) Warningf(format string, args ...interface{}) {
	l.out = append(l.out, fmt.Sprintf(format, args...))
	log.Warningf(format, args...)
}

func (l *LoggerMock) New(fields log.Fields) log.Logger {
	return l
}

func (l *LoggerMock) With(fields log.Fields) log.Logger {
	return l
}
