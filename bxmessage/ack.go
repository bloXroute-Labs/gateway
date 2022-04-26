package bxmessage

// Ack acknowledges a received header message
type Ack struct {
	Header
}

// Pack serializes an Ack into a buffer for sending on the wire
func (m Ack) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.Header.Size()
	buf := make([]byte, bufLen)
	m.Header.Pack(&buf, AckType)
	return buf, nil
}
