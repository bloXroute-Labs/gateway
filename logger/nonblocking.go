package logger

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

const backLog = 100000

type line struct {
	data  *Fields
	msg   string
	level Level
	time  time.Time
}

type nonBlocking struct {
	lines        chan line
	avoidChannel bool
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NonBlocking logs via channel to reduce contention
var NonBlocking *nonBlocking

func init() {
	NonBlocking = newNonBlockingLogger()
}

func logOneLine(oneline line) {
	entry := logrus.NewEntry(logrus.StandardLogger())
	entry.Time = oneline.time
	if oneline.data != nil {
		entry = entry.WithFields(logrus.Fields(*oneline.data))
	}
	entry.Log(logrus.Level(oneline.level), oneline.msg)
}

func newNonBlockingLogger() *nonBlocking {
	lines := make(chan line, backLog)
	ctx, cancel := context.WithCancel(context.Background())
	l := &nonBlocking{lines: lines, ctx: ctx, cancel: cancel}
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		for {
			select {
			case oneLine, ok := <-lines:
				// if we got an error on channel we have noting more to do
				if !ok {
					logrus.Errorf("non blocking log channel unexpectdly closed. nonBlockingLogger stopped")
					return
				}
				logOneLine(oneLine)
			case <-ctx.Done():
				for {
					select {
					case oneLine := <-lines:
						// if we got an error on channel we have noting more to do
						logOneLine(oneLine)
					default:
						return
					}
				}
			}
		}
	}()
	return l
}

// AvoidChannel should be used in tests only
func (l *nonBlocking) AvoidChannel() {
	l.avoidChannel = true
}

// Logf - main log function
func (l nonBlocking) Logf(level Level, fields *Fields, format string, args ...interface{}) {
	if !IsLevelEnabled(level) {
		return
	}

	oneLine := line{
		msg:   fmt.Sprintf(format, args...),
		data:  fields,
		level: level,
		time:  time.Now(),
	}

	if l.avoidChannel {
		logOneLine(oneLine)
		return
	}

	select {
	case l.lines <- oneLine:
	default:
		logrus.Errorf("Unable to use FastLogger properly")
	}
}

func (l nonBlocking) Log(level Level, fields *Fields, a ...interface{}) {
	if !IsLevelEnabled(level) {
		return
	}
	oneLine := line{
		msg:   fmt.Sprint(a...),
		data:  fields,
		level: level,
		time:  time.Now(),
	}
	if l.avoidChannel {
		logOneLine(oneLine)
		return
	}

	select {
	case l.lines <- oneLine:
	default:
		logrus.Errorf("Unable to use FastLogger properly")
	}
}

// Exit verifies all log records in the channel are written before exiting
func (l *nonBlocking) Exit() {
	l.AvoidChannel()
	// Flushes all log records
	l.cancel()
	l.wg.Wait()
}
