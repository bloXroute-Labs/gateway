package logger

import (
	"fmt"

	"github.com/rs/zerolog"
)

// Logger type
type Logger struct {
	zerolog.Logger
	close func() error
}

// Close closes the underlying file writer
func (l *Logger) Close() error {
	return l.close()
}

// CreateCustomLogger creates a new custom logger that only writes logs to a file
func CreateCustomLogger(appName string, port int, fileName string, maxSize int, maxBackups int, maxAge int, logFileLevel Level) (*Logger, error) {
	fw := fileWriter(fmt.Sprintf("logs/%v-%v-%v.log", fileName, appName, port), maxSize, maxBackups, maxAge, toZeroLogLevel(logFileLevel))

	return &Logger{Logger: zerolog.New(fw).With().Timestamp().Logger(), close: fw.Close}, nil
}
