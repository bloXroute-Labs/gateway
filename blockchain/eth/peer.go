package eth

import (
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth/protocols/bsc"
	eth2 "github.com/bloXroute-Labs/gateway/v2/blockchain/eth/protocols/eth"
)

// ethPeerInfo represents a short summary of the `eth` sub-protocol metadata known
// about a connected peer.
type ethPeerInfo struct {
	Version uint `json:"version"` // Ethereum protocol version negotiated
}

// ethPeer is a wrapper around eth.Peer to maintain a few extra metadata.
type ethPeer struct {
	*eth2.Peer
	bscExt *bscPeer // Satellite `bsc` connection
}

// info gathers and returns some `eth` protocol metadata known about a peer.
func (p *ethPeer) info() *ethPeerInfo {
	return &ethPeerInfo{
		Version: p.Version(),
	}
}

// bscPeerInfo represents a short summary of the `bsc` sub-protocol metadata known
// about a connected peer.
type bscPeerInfo struct {
	Version uint `json:"version"` // bsc protocol version negotiated
}

// bscPeer is a wrapper around bsc.Peer to maintain a few extra metadata.
type bscPeer struct {
	*bsc.Peer
}

// info gathers and returns some `bsc` protocol metadata known about a peer.
func (p *bscPeer) info() *bscPeerInfo {
	return &bscPeerInfo{
		Version: p.Version(),
	}
}
