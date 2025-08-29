package bsc

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/p2p"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
)

// Peer is a collection of relevant information we have about a `bsc` peer.
type Peer struct {
	*p2p.Peer
	id      string
	rw      p2p.MsgReadWriter
	version uint
	log     *log.Entry
	ctx     context.Context
	cancel  context.CancelFunc

	dispatcher *dispatcher // message request-response dispatcher
}

// NewPeer create a wrapper for a network connection and negotiated protocol
// version.
func NewPeer(parent context.Context, version uint, p *p2p.Peer, rw p2p.MsgReadWriter) *Peer {
	ctx, cancel := context.WithCancel(parent)

	peer := &Peer{
		Peer:    p,
		id:      p.ID().String(),
		rw:      rw,
		version: version,
		log: log.WithFields(log.Fields{
			"connType":   "BSC",
			"remoteAddr": p.RemoteAddr().String(),
			"id":         fmt.Sprintf("%x", p.ID().String()[:8]),
		}),
		ctx:    ctx,
		cancel: cancel,
	}

	peer.dispatcher = newDispatcher(peer)

	return peer
}

// ID retrieves the peer's unique identifier.
func (p *Peer) ID() string {
	return p.id
}

// Version retrieves the peer's negotiated `bsc` protocol version.
func (p *Peer) Version() uint {
	return p.version
}

// Log returns logger entry with pre-set fields
func (p *Peer) Log() *log.Entry {
	return p.log
}

// Stop stops the peer's context, which will terminate any ongoing operations
func (p *Peer) Stop() {
	p.cancel()
}

// RequestBlocksByRange send GetBlocksByRangeMsg by request start block hash
func (p *Peer) RequestBlocksByRange(startHeight uint64, startHash common.Hash, count uint64) ([]BlockData, error) {
	requestID := genRequestID()
	res, err := p.dispatcher.dispatchRequest(&request{
		code:      GetBlocksByRangeMsg,
		want:      BlocksByRangeMsg,
		requestID: requestID,
		data: &GetBlocksByRangePacket{
			RequestId:        requestID,
			StartBlockHeight: startHeight,
			StartBlockHash:   startHash,
			Count:            count,
		},
		timeout: time.Second,
	})
	if err != nil {
		return nil, err
	}

	// type assertion to get the response object
	ret, ok := res.(*BlocksByRangePacket)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: expected *BlocksByRangePacket, got %T", res)
	}

	return ret.Blocks, nil
}
