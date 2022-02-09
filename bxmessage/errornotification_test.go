package bxmessage

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestErrorNotificationPackUnpack(t *testing.T) {
	errorNotification := ErrorNotification{}
	errorNotification.Code = 12
	errorNotification.Reason = "failed for error"
	e, err := errorNotification.Pack(0)
	assert.Nil(t, err)

	var decodedErrorNotification ErrorNotification
	err = decodedErrorNotification.Unpack(e, 0)
	assert.Nil(t, err)

	assert.Equal(t, 12, int(decodedErrorNotification.Code))
	assert.Equal(t, "failed for error", decodedErrorNotification.Reason)
}
