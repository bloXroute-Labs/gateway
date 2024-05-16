package logger

import "github.com/rs/zerolog"

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

func toZeroLogLevel(level Level) zerolog.Level {
	switch level {
	case PanicLevel:
		return zerolog.PanicLevel
	case FatalLevel:
		return zerolog.FatalLevel
	case ErrorLevel:
		return zerolog.ErrorLevel
	case WarnLevel:
		return zerolog.WarnLevel
	case InfoLevel:
		return zerolog.InfoLevel
	case DebugLevel:
		return zerolog.DebugLevel
	case TraceLevel:
		return zerolog.TraceLevel
	default:
		return zerolog.NoLevel
	}
}

func fromZeroLogLevel(level zerolog.Level) Level {
	switch level {
	case zerolog.PanicLevel:
		return PanicLevel
	case zerolog.FatalLevel:
		return FatalLevel
	case zerolog.ErrorLevel:
		return ErrorLevel
	case zerolog.WarnLevel:
		return WarnLevel
	case zerolog.InfoLevel:
		return InfoLevel
	case zerolog.DebugLevel:
		return DebugLevel
	case zerolog.TraceLevel:
		return TraceLevel
	default:
		return PanicLevel
	}
}
