package logger

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/diode"
	"gopkg.in/natefinch/lumberjack.v2"
)

const backLog = 100000

// levelWriter is a writer that only writes logs of a certain level
type levelWriter struct {
	io.WriteCloser
	minLevel, maxLevel, systemLevel zerolog.Level
}

// WriteLevel writes p to the writer if the level is enabled
func (lw *levelWriter) WriteLevel(l zerolog.Level, p []byte) (n int, err error) {
	if l >= lw.minLevel && l <= lw.maxLevel && l >= lw.systemLevel {
		return lw.Write(p)
	}

	return len(p), nil
}

// stdoutWriter returns a writer that writes to stdout;
// it should never write WARN, ERROR, etc. logs, since that's handled by stderr
func stdoutWriter(level zerolog.Level) *levelWriter {
	return &levelWriter{
		WriteCloser: diode.NewWriter(newWriter(os.Stdout, true), backLog, 0, logOverflowAlerter),
		minLevel:    zerolog.TraceLevel,
		maxLevel:    zerolog.InfoLevel,
		systemLevel: level,
	}
}

// stderrWriter returns a writer that writes to stderr;
// it should never write INFO or more verbose logs
func stderrWriter(level zerolog.Level) *levelWriter {
	return &levelWriter{
		WriteCloser: diode.NewWriter(newWriter(os.Stderr, true), backLog, 0, logOverflowAlerter),
		minLevel:    zerolog.WarnLevel,
		maxLevel:    zerolog.PanicLevel,
		systemLevel: level,
	}
}

// fileWriter returns a writer that writes to a file;
func fileWriter(fileName string, maxSize, maxBackups, maxAge int, level zerolog.Level) *levelWriter {
	w := &lumberjack.Logger{
		Filename:   fileName,
		MaxSize:    maxSize,
		MaxAge:     maxAge,
		MaxBackups: maxBackups,
		LocalTime:  false,
		Compress:   false,
	}

	return &levelWriter{
		WriteCloser: diode.NewWriter(newWriter(w, true), backLog, 100*time.Millisecond, logOverflowAlerter),
		minLevel:    zerolog.TraceLevel,
		maxLevel:    zerolog.PanicLevel,
		systemLevel: level,
	}
}

func newWriter(out io.Writer, noColor bool) zerolog.ConsoleWriter {
	return zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.Out = out
		w.NoColor = noColor
		w.FormatTimestamp = formatTimestamp()
		w.FormatLevel = formatLevel(noColor)
		w.FormatMessage = func(i interface{}) string {
			return fmt.Sprintf("msg=\"%s\"", i)
		}
	})
}

func logOverflowAlerter(i int) {
	fmt.Fprintf(os.Stderr, "diode dropped %d messages\n", i)
}
