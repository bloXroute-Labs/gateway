package logger

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/fluent/fluent-logger-golang/fluent"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/diode"
)

const (
	fluentDTag         = "bx.go.log"
	defaultBufferLimit = 32 * 1024
)

var (
	once   sync.Once
	nodeID string
)

// SetNodeID sets the node ID for the fluentd writer
func SetNodeID(id string) {
	once.Do(func() {
		nodeID = id
	})
}

// fluentDWriter returns a writer that writes to fluentd
func fluentDWriter(fluentDHost string, level zerolog.Level) (*levelWriter, error) {
	fd, err := fluent.New(fluent.Config{
		FluentPort:    24224,
		FluentHost:    fluentDHost,
		BufferLimit:   defaultBufferLimit,
		Async:         true,
		MarshalAsJSON: true,
	})
	if err != nil {
		return nil, err
	}

	// set writer to discard logs
	w := newWriter(io.Discard, true)
	// use formatter to send logs to fluentd
	w.FormatPrepare = func(m map[string]interface{}) error {
		// FormatPrepare is called before FormatTimestamp, thus using standard time format to parse
		tm, err := time.Parse(zerolog.TimeFieldFormat, m["time"].(string))
		if err != nil {
			return fmt.Errorf("failed to parse time for fluend: %v", err)
		}
		m["level"] = strings.ToUpper(m["level"].(string))
		m["timestamp"] = m["time"].(string)
		if nodeID != "" {
			m["instance"] = nodeID
		}

		return fd.EncodeAndPostData(fluentDTag, tm, m)
	}

	return &levelWriter{
		WriteCloser: diode.NewWriter(w, backLog, 0, func(int) {}),
		minLevel:    zerolog.TraceLevel,
		maxLevel:    zerolog.PanicLevel,
		systemLevel: level,
	}, nil
}
