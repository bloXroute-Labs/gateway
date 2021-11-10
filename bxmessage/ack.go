package bxmessage

// Ack acknowledges a received header message
type Ack struct {
	Header
}

func (m *Ack) size() uint32 {
	return m.Header.Size() + 1
}

// Pack serializes an Ack into a buffer for sending on the wire
func (m Ack) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.size()
	buf := make([]byte, bufLen)
	buf[bufLen-1] = 0x01
	m.Header.Pack(&buf, AckType)
	return buf, nil
}

// Unpack deserializes an Ack from a buffer
func (m *Ack) Unpack(buf []byte, protocol Protocol) error {
	return nil
}
