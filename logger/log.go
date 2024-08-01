package logger

import (
	"fmt"
	"io"
	gLog "log"
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
func Init(logConfig *Config, fluentDConfig *FluentDConfig, version string) (close func(), err error) {
	fileLevel := toZeroLogLevel(logConfig.FileLevel)
	consoleLevel := toZeroLogLevel(logConfig.ConsoleLevel)

	var closeFuncs []func() error

	stderr := stderrWriter(consoleLevel)

	file := fileWriter(logConfig.FileName, logConfig.MaxSize, logConfig.MaxBackups, logConfig.MaxAge, fileLevel)
	closeFuncs = append(closeFuncs, file.Close)

	writers := []io.Writer{
		stderr,
		file,
	}

	if consoleLevel <= zerolog.InfoLevel {
		stdout := stdoutWriter(consoleLevel)
		closeFuncs = append(closeFuncs, stdout.Close)
		writers = append(writers, stdout)
	}

	if fluentDConfig != nil {
		w, err := fluentDWriter(fluentDConfig.FluentDHost, toZeroLogLevel(fluentDConfig.Level))
		if err != nil {
			return nil, fmt.Errorf("failed to create FluentD writer: %v", err)
		}
		closeFuncs = append(closeFuncs, w.Close)
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

	return func() {
		for _, f := range closeFuncs {
			if err := f(); err != nil {
				gLog.Printf("failed to close underlying log writer: %v", err)
			}
		}
	}, nil
}
