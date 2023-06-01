package bxmessage

import (
	"encoding/binary"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// ErrorNotification represents an error msg the relay sends to the gateway
type ErrorNotification struct {
	Header
	Code   types.ErrorNotificationCode
	Reason string
}

func (m *ErrorNotification) size() uint32 {
	return m.Header.Size() + uint32(types.ErrorNotificationCodeLen+len(m.Reason))
}

// Pack serializes an ErrorNotification into the buffer for sending
func (m *ErrorNotification) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.size()
	buf := make([]byte, bufLen)
	m.Header.Pack(&buf, ErrorNotificationType)
	offset := HeaderLen
	binary.LittleEndian.PutUint32(buf[offset:], uint32(m.Code))
	offset += types.ErrorNotificationCodeLen
	copy(buf[offset:], m.Reason)
	return buf, nil
}

// Unpack deserializes an ErrorNotification from a buffer
func (m *ErrorNotification) Unpack(buf []byte, protocol Protocol) error {
	offset := HeaderLen
	m.Code = types.ErrorNotificationCode(binary.LittleEndian.Uint32(buf[offset:]))
	offset += types.ErrorNotificationCodeLen
	m.Reason = string(buf[offset : len(buf)-ControlByteLen])
	return m.Header.Unpack(buf, protocol)
}
