package logger

import (
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestLog_Exit(t *testing.T) {
	SetLevel(DebugLevel)
	hook := test.NewGlobal()

	numLogRecords := 1000
	for i := 0; i < numLogRecords; i++ {
		Infof("log record %v", i)
	}

	assert.Greater(t, numLogRecords, len(hook.Entries))
	NonBlocking.Exit()
	assert.Equal(t, numLogRecords, len(hook.Entries))
}
