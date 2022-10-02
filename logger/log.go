package logger

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/orandin/lumberjackrus"
	"github.com/sirupsen/logrus"
)

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

// writerHook is a hook that writes logs of specified logLevels to specified writer
// This hook is used to separate stdout and stderr log output, as this is not supported natively by logrus
type writerHook struct {
	writer    io.Writer
	logLevels []logrus.Level
}

func stdoutWriter(level logrus.Level) *writerHook {
	var levels []logrus.Level

	// stdout should never write WARN, ERROR, etc. logs, since that's handled by stderr
	// (increasing log levels = more verbose logs)
	if level >= logrus.InfoLevel {
		levels = logrus.AllLevels[logrus.InfoLevel : level+1]
	}
	return &writerHook{
		writer:    os.Stdout,
		logLevels: levels,
	}
}

func stderrWriter(level logrus.Level) *writerHook {
	// stderr should never write INFO or more verbose logs, cap the levels represented
	if level >= logrus.InfoLevel {
		level = logrus.WarnLevel
	}
	return &writerHook{
		writer:    os.Stderr,
		logLevels: logrus.AllLevels[:level+1],
	}
}

// Fire will be called when some logging function is called with current hook
// It will format log entry to string and write it to appropriate writer
func (hook *writerHook) Fire(entry *logrus.Entry) error {
	line, err := entry.Bytes()
	if err != nil {
		return err
	}
	_, err = hook.writer.Write(line)
	return err
}

// Levels define on which log levels this hook would trigger
func (hook *writerHook) Levels() []logrus.Level {
	return hook.logLevels
}

// Init - initialise logging
func Init(logConfig *Config, version string) error {
	fileHook, formatter, err := createLogFileHook(logConfig.FileName, logConfig.MaxSize, logConfig.MaxBackups, logConfig.MaxAge, logrus.Level(logConfig.FileLevel))
	if err != nil {
		return err
	}

	logrus.SetFormatter(formatter)
	logrus.SetLevel(logrus.TraceLevel)

	// send logs to nowhere by default, use hooks for separate stdout/stderr
	logrus.SetOutput(ioutil.Discard)

	// ignoring prysm logs
	filterPrysmLogs := func(entry *logrus.Entry) bool {
		if _, ok := entry.Data["prefix"]; ok {
			return true
		}

		if _, ok := entry.Data["function"]; ok {
			return true
		}

		return false
	}

	logrus.AddHook(newFilterHook(
		stdoutWriter(logrus.Level(logConfig.ConsoleLevel)),
		filterPrysmLogs,
	))
	logrus.AddHook(newFilterHook(
		stderrWriter(logrus.Level(logConfig.ConsoleLevel)),
		filterPrysmLogs,
	))
	logrus.AddHook(newFilterHook(fileHook, filterPrysmLogs))

	logrus.Debugf("log initiated.")
	logrus.Infof("%v (%v) is starting with arguments %v", logConfig.AppName, version, strings.Join(os.Args[1:], " "))
	return nil
}

// CreateCustomLogger creates a new custom logrus instance
func CreateCustomLogger(appName string, port int, fileName string, maxSize int, maxBackups int, maxAge int, logFileLevel Level) (*Logger, error) {
	customLogger := logrus.New()

	fileHook, formatter, err := createLogFileHook(fmt.Sprintf("logs/%v-%v-%v.log", fileName, appName, port), maxSize, maxBackups, maxAge, logrus.Level(logFileLevel))
	if err != nil {
		return nil, err
	}

	customLogger.SetFormatter(formatter)
	customLogger.SetLevel(logrus.Level(logFileLevel))

	// send logs to nowhere by default, use hook for redirection to file
	customLogger.SetOutput(ioutil.Discard)
	customLogger.AddHook(fileHook)

	return &Logger{Logger: customLogger}, nil
}

func createLogFileHook(fileName string, maxSize int, maxBackups int, maxAge int, logFileLevel logrus.Level) (*lumberjackrus.Hook, *logrus.TextFormatter, error) {
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = timestampFormat
	formatter.FullTimestamp = true
	formatter.DisableColors = true

	fileHook, err := lumberjackrus.NewHook(
		&lumberjackrus.LogFile{
			Filename:   fileName,
			MaxSize:    maxSize,
			MaxBackups: maxBackups,
			MaxAge:     maxAge,
			Compress:   false,
			LocalTime:  false,
		},
		logFileLevel,
		formatter,
		&lumberjackrus.LogFileOpts{},
	)

	return fileHook, formatter, err
}

type filterHook struct {
	logrus.Hook
	filters []func(*logrus.Entry) bool
}

func newFilterHook(hook logrus.Hook, filters ...func(*logrus.Entry) bool) *filterHook {
	return &filterHook{
		Hook:    hook,
		filters: filters,
	}
}

// Fire will be called when some logging function is called with current hook
func (hook *filterHook) Fire(entry *logrus.Entry) error {
	for _, filter := range hook.filters {
		if filter(entry) {
			return nil
		}
	}

	return hook.Hook.Fire(entry)
}
