package logger

import (
	"github.com/sirupsen/logrus"
	"io"
	"os"
)

const (
	// PanicLevel level, highest level of severity. Logs and then calls panic with the
	// message passed to Debug, Info, ...
	PanicLevel Level = iota
	// FatalLevel level. Logs and then calls `logger.Exit(1)`. It will exit even if the
	// logging level is set to Panic.
	FatalLevel
	// ErrorLevel level. Logs. Used for errors that should definitely be noted.
	// Commonly used for hooks to send errors to an error tracking service.
	ErrorLevel
	// WarnLevel level. Non-critical entries that deserve eyes.
	WarnLevel
	// InfoLevel level. General operational entries about what's going on inside the
	// application.
	InfoLevel
	// DebugLevel level. Usually only enabled when debugging. Very verbose logging.
	DebugLevel
	// TraceLevel level. Designates finer-grained informational events than the Debug.
	TraceLevel
)

// Level type
type Level uint32

// Fields type
type Fields map[string]interface{}

// WithFields creates an entry with fields
func WithFields(fields Fields) *Entry {
	e := logrus.WithFields(logrus.Fields(fields))
	return &Entry{e: e}
}

// Logger type
type Logger struct {
	*logrus.Logger
}

// Tracef logs a message at level Trace on the standard logger.
func Tracef(format string, args ...interface{}) {
	NonBlocking.Logf(TraceLevel, nil, format, args...)
}

// Debugf logs a message at level Debug on the standard logger.
func Debugf(format string, args ...interface{}) {
	NonBlocking.Logf(DebugLevel, nil, format, args...)
}

// Printf logs a message at level Info on the standard logger.
func Printf(format string, args ...interface{}) {
	logrus.Printf(format, args...)
}

// Infof logs a message at level Info on the standard logger.
func Infof(format string, args ...interface{}) {
	NonBlocking.Logf(InfoLevel, nil, format, args...)
}

// Warnf logs a message at level Warn on the standard logger.
func Warnf(format string, args ...interface{}) {
	NonBlocking.Logf(WarnLevel, nil, format, args...)
}

// Warningf logs a message at level Warn on the standard logger.
func Warningf(format string, args ...interface{}) {
	NonBlocking.Logf(WarnLevel, nil, format, args...)
}

// Errorf logs a message at level Error on the standard logger.
func Errorf(format string, args ...interface{}) {
	NonBlocking.Logf(ErrorLevel, nil, format, args...)
}

// Panicf logs a message at level Panic on the standard logger.
func Panicf(format string, args ...interface{}) {
	NonBlocking.Logf(PanicLevel, nil, format, args...)
}

// Fatalf logs a message at level Fatal on the standard logger then the process will exit with status set to 1.
func Fatalf(format string, args ...interface{}) {
	NonBlocking.Logf(FatalLevel, nil, format, args...)
}

// Trace logs a message at level Trace on the standard logger.
func Trace(args ...interface{}) {
	NonBlocking.Log(TraceLevel, nil, args...)
}

// Debug logs a message at level Debug on the standard logger.
func Debug(args ...interface{}) {
	NonBlocking.Log(DebugLevel, nil, args...)
}

// Info logs a message at level Info on the standard logger.
func Info(args ...interface{}) {
	NonBlocking.Log(InfoLevel, nil, args...)
}

// Warn logs a message at level Warn on the standard logger.
func Warn(args ...interface{}) {
	NonBlocking.Log(WarnLevel, nil, args...)
}

// Error logs a message at level Error on the standard logger.
func Error(args ...interface{}) {
	NonBlocking.Log(ErrorLevel, nil, args...)
}

// Panic logs a message at level Panic on the standard logger.
func Panic(args ...interface{}) {
	NonBlocking.Exit()
	logrus.Panic(args...)
}

// Fatal logs a message at level Fatal on the standard logger then the process will exit with status set to 1.
func Fatal(args ...interface{}) {
	NonBlocking.Exit()
	logrus.Fatal(args...)
}

// GetLevel returns the standard logger level.
func GetLevel() Level {
	return Level(logrus.GetLevel())
}

// SetLevel sets the standard logger level.
func SetLevel(level Level) {
	logrus.SetLevel(logrus.Level(level))
}

// SetOutput sets the standard logger output.
func SetOutput(out io.Writer) {
	logrus.SetOutput(out)
}

// WithField creates an entry from the standard logger and adds a field to
// it. If you want multiple fields, use `WithFields`.
//
// Note that it doesn't log until you call Debug, Print, Info, Warn, Fatal
// or Panic on the Entry it returns.
func WithField(key string, value interface{}) *Entry {
	e := logrus.WithField(key, value)
	return &Entry{e: e}
}

// ParseLevel takes a string level and returns the Logrus log level constant.
func ParseLevel(lvl string) (Level, error) {
	level, err := logrus.ParseLevel(lvl)
	return Level(level), err
}

// IsLevelEnabled checks if the log level of the standard logger is greater than the level param
func IsLevelEnabled(level Level) bool {
	return logrus.IsLevelEnabled(logrus.Level(level))
}

// Exit change the logger to be blocking, flush all pending records and perform os.Exit
func Exit(rc int) {
	NonBlocking.Exit()
	os.Exit(rc)
}
