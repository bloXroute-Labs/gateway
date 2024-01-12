package logger

import (
	"testing"

	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

func TestLog_Exit(t *testing.T) {
	SetLevel(TraceLevel)
	hook := test.NewGlobal()

	numLogRecords := 100
	for i := 0; i < numLogRecords; i++ {
		Infof("log record %v", i)
	}

	assert.Greater(t, numLogRecords, len(hook.AllEntries()))
	NonBlocking.Exit()
	assert.Equal(t, numLogRecords, len(hook.AllEntries()))
}
