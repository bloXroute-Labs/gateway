package logger

import (
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

// Entry type
type Entry struct {
	e *logrus.Entry
}

// TestEntry - Not being used currently
func TestEntry() *Entry {
	logger, _ := test.NewNullLogger()
	return &Entry{
		e: logrus.NewEntry(logger),
	}
}

// WithField adds a single field to the Entry.
func (entry *Entry) WithField(key string, value interface{}) *Entry {
	e := entry.e.WithField(key, value)
	return &Entry{e: e}
}

// WithFields adds fields to the Entry.
func (entry *Entry) WithFields(fields Fields) *Entry {
	e := entry.e.WithFields(logrus.Fields(fields))
	return &Entry{e: e}
}

// Logf logs using level and format
func (entry *Entry) Logf(level Level, format string, args ...interface{}) {
	if !IsLevelEnabled(level) {
		return
	}
	fields := Fields(entry.e.Data)
	NonBlocking.Logf(level, &fields, format, args...)
}

// Log logs using level
func (entry *Entry) Log(level Level, args ...interface{}) {
	if !IsLevelEnabled(level) {
		return
	}
	fields := Fields(entry.e.Data)
	NonBlocking.Log(level, &fields, args...)
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

// Warningf is like Warnf
func (entry *Entry) Warningf(format string, args ...interface{}) {
	entry.Warnf(format, args...)
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

// Error logs error level
func (entry *Entry) Error(args ...interface{}) {
	entry.Log(ErrorLevel, args...)
}
