package bxmock

import (
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/sdnmessage"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/types"
)

// MockNetwork creates a mock network
func MockNetwork(networkNum types.NetworkNum, protocol string, network string,
	percentToLogByHash float64) *sdnmessage.BlockchainNetwork {
	return &sdnmessage.BlockchainNetwork{
		Network:              network,
		NetworkNum:           networkNum,
		Protocol:             protocol,
		TxPercentToLogByHash: percentToLogByHash,
	}
}
