package bxmessage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorNotificationPackUnpack(t *testing.T) {
	errorNotification := ErrorNotification{}
	errorNotification.Code = 12
	errorNotification.Reason = "failed for error"
	e, err := errorNotification.Pack(0)
	assert.NoError(t, err)

	var decodedErrorNotification ErrorNotification
	err = decodedErrorNotification.Unpack(e, 0)
	assert.NoError(t, err)

	assert.Equal(t, 12, int(decodedErrorNotification.Code))
	assert.Equal(t, "failed for error", decodedErrorNotification.Reason)
}
