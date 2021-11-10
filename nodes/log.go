package nodes

import (
	"fmt"
	"github.com/bloXroute-Labs/gateway/bxgateway/config"
	"github.com/orandin/lumberjackrus"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"os"
	"strings"
)

// writerHook is a hook that writes logs of specified logLevels to specified writer
// This hook is used to separate stdout and stderr log output, as this is not supported natively by logrus
type writerHook struct {
	writer    io.Writer
	logLevels []log.Level
}

func stdoutWriter(level log.Level) *writerHook {
	var levels []log.Level

	// stdout should never write WARN, ERROR, etc. logs, since that's handled by stderr
	// (increasing log levels = more verbose logs)
	if level >= log.InfoLevel {
		levels = log.AllLevels[log.InfoLevel : level+1]
	}
	return &writerHook{
		writer:    os.Stdout,
		logLevels: levels,
	}
}

func stderrWriter(level log.Level) *writerHook {
	// stderr should never write INFO or more verbose logs, cap the levels represented
	if level >= log.InfoLevel {
		level = log.WarnLevel
	}
	return &writerHook{
		writer:    os.Stderr,
		logLevels: log.AllLevels[:level+1],
	}
}

// Fire will be called when some logging function is called with current hook
// It will format log entry to string and write it to appropriate writer
func (hook *writerHook) Fire(entry *log.Entry) error {
	line, err := entry.Bytes()
	if err != nil {
		return err
	}
	_, err = hook.writer.Write(line)
	return err
}

// Levels define on which log levels this hook would trigger
func (hook *writerHook) Levels() []log.Level {
	return hook.logLevels
}

// InitLogs - initialise logging
func InitLogs(logConfig *config.Log, version string) error {
	fileHook, formatter, err := createLogFileHook(logConfig.FileName, logConfig.MaxSize, logConfig.MaxBackups, logConfig.MaxAge, logConfig.FileLevel)
	if err != nil {
		return err
	}

	log.SetFormatter(formatter)
	log.SetLevel(log.TraceLevel)

	// send logs to nowhere by default, use hooks for separate stdout/stderr
	log.SetOutput(ioutil.Discard)

	log.AddHook(stdoutWriter(logConfig.ConsoleLevel))
	log.AddHook(stderrWriter(logConfig.ConsoleLevel))
	log.AddHook(fileHook)

	log.Debugf("log initiated.")
	log.Infof("%v (%v) is starting with arguments %v", logConfig.AppName, version, strings.Join(os.Args[1:], " "))
	return nil
}

// CreateCustomLogger creates a new custom logrus instance
func CreateCustomLogger(appName string, port int, fileName string, maxSize int, maxBackups int, maxAge int, logFileLevel log.Level) (*log.Logger, error) {
	customLogger := logrus.New()

	fileHook, formatter, err := createLogFileHook(fmt.Sprintf("logs/%v-%v-%v.log", fileName, appName, port), maxSize, maxBackups, maxAge, logFileLevel)
	if err != nil {
		return nil, err
	}

	customLogger.SetFormatter(formatter)
	customLogger.SetLevel(logFileLevel)

	// send logs to nowhere by default, use hook for redirection to file
	customLogger.SetOutput(ioutil.Discard)
	customLogger.AddHook(fileHook)

	return customLogger, nil
}

func createLogFileHook(fileName string, maxSize int, maxBackups int, maxAge int, logFileLevel log.Level) (*lumberjackrus.Hook, *log.TextFormatter, error) {
	formatter := new(log.TextFormatter)
	formatter.TimestampFormat = "2006-01-02T15:04:05.000000"
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
