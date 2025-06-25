package eth

import (
	"github.com/ethereum/go-ethereum/p2p/enode"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/core"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth/protocols/bsc"
)

type bscHandler handler

func (h *bscHandler) Chain() *core.Chain {
	return h.chain
}

func (h *bscHandler) RunPeer(peer *bsc.Peer, hand bsc.Handler) error {
	return (*handler)(h).runBscExtension(peer, hand)
}

func (h *bscHandler) PeerInfo(id enode.ID) interface{} {
	peer, ok := h.peers.get(id.String())
	if ok && peer.bscExt != nil {
		return peer.bscExt.info()
	}

	return nil
}
