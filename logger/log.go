package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// initConfigMutex avoids race condition when somebody calls Init
var initConfigMutex sync.RWMutex

// Config represents logger options for where to write data and what data to write
type Config struct {
	AppName      string
	FileName     string
	FileLevel    Level
	ConsoleLevel Level
	MaxSize      int
	MaxBackups   int
	MaxAge       int
}

// FluentDConfig represents logger options for FluentD client
type FluentDConfig struct {
	FluentDHost string
	Level       Level
}

// Init initializes the logger with the given configuration
func Init(logConfig *Config, fluentDConfig *FluentDConfig, version string) error {
	fileLevel := toZeroLogLevel(logConfig.FileLevel)
	consoleLevel := toZeroLogLevel(logConfig.ConsoleLevel)

	writers := []io.Writer{
		stderrWriter(consoleLevel),
		fileWriter(logConfig.FileName, logConfig.MaxSize, logConfig.MaxBackups, logConfig.MaxAge, fileLevel),
	}

	if consoleLevel <= zerolog.InfoLevel {
		writers = append(writers, stdoutWriter(consoleLevel))
	}

	if fluentDConfig != nil {
		w, err := fluentDWriter(fluentDConfig.FluentDHost, toZeroLogLevel(fluentDConfig.Level))
		if err != nil {
			return fmt.Errorf("failed to create FluentD writer: %v", err)
		}
		writers = append(writers, w)
	}

	var logLevel zerolog.Level
	if consoleLevel <= fileLevel {
		logLevel = consoleLevel
	} else {
		logLevel = fileLevel
	}

	// lock configuration to avoid race conditions
	initConfigMutex.Lock()
	defer initConfigMutex.Unlock()

	zerolog.TimeFieldFormat = timestampFormat
	log.Logger = zerolog.New(zerolog.MultiLevelWriter(writers...)).Level(logLevel).With().Timestamp().Logger()

	log.Debug().Msg("log initiated")
	log.Info().Str("version", version).Msgf("%v is starting with arguments %v", logConfig.AppName, strings.Join(os.Args[1:], " "))

	return nil
}
