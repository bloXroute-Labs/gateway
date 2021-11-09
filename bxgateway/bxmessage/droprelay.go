package bxmessage

// DropRelay represents a message from relay to gateway requesting gateway disconnect and get new peers
type DropRelay struct {
	Header
}

func (m *DropRelay) size() uint32 {
	return m.Header.Size() + 1
}

// Pack serializes an DropRelay into a buffer for sending on the wire
func (m DropRelay) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.size()
	buf := make([]byte, bufLen)
	buf[bufLen-1] = ControlByte
	m.Header.Pack(&buf, DropRelayType)
	return buf, nil
}

// Unpack deserializes an DropRelay from a buffer
func (m *DropRelay) Unpack(buf []byte, protocol Protocol) error {
	return nil
}
