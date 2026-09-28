package bxmock

import (
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/v2/sdnsdk/message"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/v2/types"
)

// MockNetwork creates a mock network
func MockNetwork(networkNum bxtypes.NetworkNum, protocol string, network string,
	percentToLogByHash float64) *sdnmessage.BlockchainNetwork {
	return &sdnmessage.BlockchainNetwork{
		Network:              network,
		NetworkNum:           networkNum,
		Protocol:             protocol,
		TxPercentToLogByHash: percentToLogByHash,
	}
}
