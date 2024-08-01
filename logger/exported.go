package logger

import (
	"fmt"
	"io"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Tracef logs a message at level Trace on the standard logger.
func Tracef(format string, args ...interface{}) {
	log.Trace().Msgf(format, args...)
}

// Debugf logs a message at level Debug on the standard logger.
func Debugf(format string, args ...interface{}) {
	log.Debug().Msgf(format, args...)
}

// Printf logs a message at level Info on the standard logger.
func Printf(format string, args ...interface{}) {
	log.Printf(format, args...)
}

// Infof logs a message at level Info on the standard logger.
func Infof(format string, args ...interface{}) {
	log.Info().Msgf(format, args...)
}

// Warnf logs a message at level Warn on the standard logger.
func Warnf(format string, args ...interface{}) {
	log.Warn().Msgf(format, args...)
}

// Errorf logs a message at level Error on the standard logger.
func Errorf(format string, args ...interface{}) {
	log.Error().Msgf(format, args...)
}

// Panicf logs a message at level Panic on the standard logger.
func Panicf(format string, args ...interface{}) {
	log.Panic().Msgf(format, args...)
}

// Fatalf logs a message at level Fatal on the standard logger then the process will exit with status set to 1.
func Fatalf(format string, args ...interface{}) {
	log.Fatal().Msgf(format, args...)
}

// Trace logs a message at level Trace on the standard logger.
func Trace(args ...interface{}) {
	log.Trace().Msg(fmt.Sprint(args...))
}

// Debug logs a message at level Debug on the standard logger.
func Debug(args ...interface{}) {
	log.Debug().Msg(fmt.Sprint(args...))
}

// Info logs a message at level Info on the standard logger.
func Info(args ...interface{}) {
	log.Info().Msg(fmt.Sprint(args...))
}

// Warn logs a message at level Warn on the standard logger.
func Warn(args ...interface{}) {
	log.Warn().Msg(fmt.Sprint(args...))
}

// Error logs a message at level Error on the standard logger.
func Error(args ...interface{}) {
	log.Error().Msg(fmt.Sprint(args...))
}

// Panic logs a message at level Panic on the standard logger.
func Panic(args ...interface{}) {
	log.Panic().Msg(fmt.Sprint(args...))
}

// Fatal logs a message at level Fatal on the standard logger then the process will exit with status set to 1.
func Fatal(args ...interface{}) {
	log.Fatal().Msg(fmt.Sprint(args...))
}

// GetLevel returns the standard logger level.
func GetLevel() Level {
	return fromZeroLogLevel(log.Logger.GetLevel())
}

// SetLevel sets the standard logger level.
func SetLevel(level Level) {
	// Lock configuration to avoid race conditions
	initConfigMutex.Lock()
	defer initConfigMutex.Unlock()

	log.Logger = log.Level(toZeroLogLevel(level))
}

// SetOutput sets the standard logger output.
func SetOutput(out io.Writer) {
	// Lock configuration to avoid race conditions
	initConfigMutex.Lock()
	defer initConfigMutex.Unlock()

	log.Logger = log.Output(out)
}

// ParseLevel takes a string level and returns the log level constant.
func ParseLevel(lvl string) (Level, error) {
	level, err := zerolog.ParseLevel(lvl)
	return fromZeroLogLevel(level), err
}

// IsLevelEnabled checks if the log level of the standard logger is greater than the level param
func IsLevelEnabled(level Level) bool {
	return fromZeroLogLevel(log.Logger.GetLevel()) >= level
}
