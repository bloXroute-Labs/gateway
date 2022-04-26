package bxmock

import (
	"github.com/bloXroute-Labs/gateway/sdnmessage"
	"github.com/bloXroute-Labs/gateway/types"
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
