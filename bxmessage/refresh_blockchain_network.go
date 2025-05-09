package bxmessage

import (
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// RefreshBlockchainNetwork acknowledges a received header message
type RefreshBlockchainNetwork struct {
	Header
}

func (m *RefreshBlockchainNetwork) size() uint32 {
	return m.Header.Size()
}

// GetNetworkNum gets the message network number
func (m *RefreshBlockchainNetwork) GetNetworkNum() bxtypes.NetworkNum {
	return types.AllNetworkNum
}

// Pack serializes an RefreshBlockchainNetwork into a buffer for sending on the wire
func (m RefreshBlockchainNetwork) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.size()
	buf := make([]byte, bufLen)
	m.Header.Pack(&buf, RefreshBlockchainNetworkType)
	return buf, nil
}
