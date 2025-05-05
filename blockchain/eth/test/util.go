package test

import (
	"fmt"
	"math/rand"

	"github.com/ethereum/go-ethereum/p2p/enode"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

// GenerateEnodeID randomly creates an enode for testing purposes
func GenerateEnodeID() enode.ID {
	var enodeID enode.ID
	id := make([]byte, 32)
	_, _ = rand.Read(id)

	copy(enodeID[:], id)
	return enodeID
}

// GenerateBlockchainPeersInfo returns generated lists of NodeEndpoint and PeerInfo
func GenerateBlockchainPeersInfo(numPeers int) ([]types.NodeEndpoint, []network.PeerInfo) {
	var blockchainPeers []types.NodeEndpoint
	var blockchainPeersInfo []network.PeerInfo
	ip := "123.45.6.78"
	port := 1234
	wsPortOffset := 100
	for i := 0; i < numPeers; i++ {
		blockchainPeers = append(blockchainPeers, types.NodeEndpoint{IP: ip, Port: port + i})
		enode := utils.GenerateValidEnode(ip, port+i, port+i)
		blockchainPeersInfo = append(blockchainPeersInfo, network.PeerInfo{
			Enode:    enode,
			EthWSURI: fmt.Sprintf("ws://%v:%v", ip, port+i+wsPortOffset),
		})
	}
	return blockchainPeers, blockchainPeersInfo
}
