package logger

import (
	"fmt"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Entry type
type Entry struct {
	ee zerolog.Logger
}

// Fields type
type Fields map[string]interface{}

// WithFields creates an entry with fields
func WithFields(fields Fields) *Entry {
	return &Entry{ee: log.With().Fields(map[string]interface{}(fields)).Logger()}
}

// WithField creates an entry with field
func WithField(key string, value interface{}) *Entry {
	return &Entry{ee: log.With().Any(key, value).Logger()}
}

// WithField adds a single field to the Entry.
func (entry *Entry) WithField(key string, value interface{}) *Entry {
	return &Entry{ee: entry.ee.With().Any(key, value).Logger()}
}

// WithFields adds fields to the Entry.
func (entry *Entry) WithFields(fields Fields) *Entry {
	return &Entry{ee: entry.ee.With().Fields(map[string]interface{}(fields)).Logger()}
}

// Logf logs using level and format
func (entry *Entry) Logf(level Level, format string, args ...interface{}) {
	if entry == nil {
		return
	}

	entry.ee.WithLevel(toZeroLogLevel(level)).Msgf(format, args...)
}

// Log logs using level
func (entry *Entry) Log(level Level, args ...interface{}) {
	if entry == nil {
		return
	}

	entry.ee.WithLevel(toZeroLogLevel(level)).Msg(fmt.Sprint(args...))
}

// Tracef logs trace level with format
func (entry *Entry) Tracef(format string, args ...interface{}) {
	entry.Logf(TraceLevel, format, args...)
}

// Debugf logs debug level with format
func (entry *Entry) Debugf(format string, args ...interface{}) {
	entry.Logf(DebugLevel, format, args...)
}

// Infof logs info level with format
func (entry *Entry) Infof(format string, args ...interface{}) {
	entry.Logf(InfoLevel, format, args...)
}

// Warnf logs warn level with format
func (entry *Entry) Warnf(format string, args ...interface{}) {
	entry.Logf(WarnLevel, format, args...)
}

// Errorf logs error level with format
func (entry *Entry) Errorf(format string, args ...interface{}) {
	entry.Logf(ErrorLevel, format, args...)
}

// Panicf logs panic level with format
func (entry *Entry) Panicf(format string, args ...interface{}) {
	entry.Logf(PanicLevel, format, args...)
}

// Trace logs trace level
func (entry *Entry) Trace(args ...interface{}) {
	entry.Log(TraceLevel, args...)
}

// Debug logs debug level
func (entry *Entry) Debug(args ...interface{}) {
	entry.Log(DebugLevel, args...)
}

// Info logs info level
func (entry *Entry) Info(args ...interface{}) {
	entry.Log(InfoLevel, args...)
}

// Warn logs warn level
func (entry *Entry) Warn(args ...interface{}) {
	entry.Log(WarnLevel, args...)
}

// Error logs error level
func (entry *Entry) Error(args ...interface{}) {
	entry.Log(ErrorLevel, args...)
}

// Fatalf logs fatal level
func (entry *Entry) Fatalf(format string, args ...interface{}) {
	entry.ee.Fatal().Msgf(format, args...)
}
