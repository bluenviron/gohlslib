package gohlslib

import (
	"log"
)

// LogLevel is a log level.
type LogLevel int

// Log levels.
const (
	LogLevelDebug LogLevel = iota + 1
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// LogFunc is the prototype of the log function.
type LogFunc func(level LogLevel, format string, args ...interface{})

func defaultLog(_ LogLevel, format string, args ...interface{}) {
	log.Printf(format, args...)
}
