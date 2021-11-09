package bxmessage

// RefreshBlockchainNetwork acknowledges a received header message
type RefreshBlockchainNetwork struct {
	Header
}

func (m *RefreshBlockchainNetwork) size() uint32 {
	return m.Header.Size() + 1
}

// Pack serializes an RefreshBlockchainNetwork into a buffer for sending on the wire
func (m RefreshBlockchainNetwork) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.size()
	buf := make([]byte, bufLen)
	buf[bufLen-1] = 0x01
	m.Header.Pack(&buf, RefreshBlockchainNetworkType)
	return buf, nil
}

// Unpack deserializes an RefreshBlockchainNetwork from a buffer
func (m *RefreshBlockchainNetwork) Unpack(buf []byte, protocol Protocol) error {
	return nil
}
