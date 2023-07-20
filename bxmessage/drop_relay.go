package bxmessage

// DropRelay represents a message from relay to gateway requesting gateway disconnect and get new peers
type DropRelay struct {
	Header
}

// Pack serializes an DropRelay into a buffer for sending on the wire
func (m DropRelay) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.Header.Size()
	buf := make([]byte, bufLen)
	m.Header.Pack(&buf, DropRelayType)
	return buf, nil
}
