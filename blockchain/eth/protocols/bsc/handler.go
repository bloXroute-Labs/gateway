package bsc

import (
	"context"
	"fmt"
	"time"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/core"
)

// MaxRequestRangeBlocksCount defines the maximum number of blocks that can be requested
const MaxRequestRangeBlocksCount = 64

// Decoder is an interface that defines the method to decode a message.
type Decoder interface {
	Decode(val interface{}) error
}

// Handler is a callback to invoke from an outside runner after the boilerplate
// exchanges have passed.
type Handler func(peer *Peer) error

// Backend defines the behavior of the `bsc` protocol handler
type Backend interface {
	// Chain retrieves the blockchain object to serve data.
	Chain() *core.Chain

	// RunPeer is invoked when a peer joins on the `bsc` protocol. The handler
	// should do any peer maintenance work, handshakes and validations. If all
	// is passed, control should be given back to the `handler` to process the
	// inbound messages going forward.
	RunPeer(peer *Peer, handler Handler) error

	// PeerInfo retrieves all known `bsc` information about a peer.
	PeerInfo(id enode.ID) interface{}
}

// MakeProtocols constructs the P2P protocol definitions for `bsc`.
func MakeProtocols(ctx context.Context, backend Backend) []p2p.Protocol {
	log.Infof("creating bsc protocols with versions: %v", ProtocolVersions)

	protocols := make([]p2p.Protocol, len(ProtocolVersions))
	for i, version := range ProtocolVersions {
		protocols[i] = p2p.Protocol{
			Name:    ProtocolName,
			Version: version,
			Length:  protocolLengths[version],
			Run: func(p *p2p.Peer, rw p2p.MsgReadWriter) error {
				peer := NewPeer(ctx, version, p, rw)

				return backend.RunPeer(peer, func(peer *Peer) error {
					return Handle(backend, peer)
				})
			},
			NodeInfo: func() interface{} {
				return nodeInfo()
			},
			PeerInfo: func(id enode.ID) interface{} {
				return backend.PeerInfo(id)
			},
			Attributes: []enr.Entry{&enrEntry{}},
		}
	}
	return protocols
}

// Handle is the callback invoked to manage the life cycle of a `bsc` peer.
// When this function terminates, the peer is disconnected.
func Handle(backend Backend, peer *Peer) error {
	for {
		if err := handleMessage(backend, peer); err != nil {
			peer.Log().Debugf("message handling failed in `bsc`: %v", err)
			return err
		}
	}
}

type msgHandler func(backend Backend, msg Decoder, peer *Peer) error

var handleBSC1 = map[uint64]msgHandler{
	VotesMsg: handleVotes,
}

var handleBSC2 = map[uint64]msgHandler{
	VotesMsg:            handleVotes,
	GetBlocksByRangeMsg: handleGetBlocksByRange,
	BlocksByRangeMsg:    handleBlocksByRange,
}

// handleMessage is invoked whenever an inbound message is received from a
// remote peer on the `bsc` protocol. The remote connection is torn down upon
// returning any error.
func handleMessage(backend Backend, peer *Peer) error {
	// Read the next message from the remote peer, and ensure it's fully consumed
	msg, err := peer.rw.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Size > maxMessageSize {
		return fmt.Errorf("%w: %v > %v", errMsgTooLarge, msg.Size, maxMessageSize)
	}

	startTime := time.Now()
	defer func() {
		_ = msg.Discard() //nolint:errcheck
		peer.log.Tracef("%v: handling message with code: %v took %v", peer, msg.Code, time.Since(startTime))
	}()

	var handlers = handleBSC1
	if peer.Version() >= Bsc2 {
		handlers = handleBSC2
	}

	if handler := handlers[msg.Code]; handler != nil {
		return handler(backend, msg, peer)
	}

	return fmt.Errorf("%w: %v", errInvalidMsgCode, msg.Code)
}

func handleVotes(Backend, Decoder, *Peer) error {
	return nil
}

func handleGetBlocksByRange(backend Backend, msg Decoder, p *Peer) error {
	req := new(GetBlocksByRangePacket)
	if err := msg.Decode(req); err != nil {
		return fmt.Errorf("msg %v, decode err: %v", GetBlocksByRangeMsg, err)
	}

	p.log.Debug("receive GetBlocksByRange request")

	// validate request parameters
	if req.Count == 0 || req.Count > MaxRequestRangeBlocksCount { // Limit maximum request count
		return fmt.Errorf("msg %v, invalid count: %v", GetBlocksByRangeMsg, req.Count)
	}

	var start eth.HashOrNumber

	if req.StartBlockHash != (common.Hash{}) {
		start = eth.HashOrNumber{Hash: req.StartBlockHash}
	} else {
		start = eth.HashOrNumber{Number: req.StartBlockHeight}
	}

	// start eth.HashOrNumber, count int, skip int, reverse bool
	blocks, err := backend.Chain().GetBlocks(start, int(req.Count), 0, true) //nolint:gosec
	if err != nil {
		return fmt.Errorf("msg %v, cannot get blocks: %w", GetBlocksByRangeMsg, err)
	}

	p.log.Debug("reply GetBlocksByRange msg")

	resp := make([]BlockData, len(blocks))
	for i := range blocks {
		resp[i] = BlockData{
			Header:      blocks[i].Header(),
			Txs:         blocks[i].Transactions(),
			Uncles:      blocks[i].Uncles(),
			Withdrawals: blocks[i].Withdrawals(),
			Sidecars:    blocks[i].Sidecars(),
		}
	}

	return p2p.Send(p.rw, BlocksByRangeMsg, &BlocksByRangePacket{
		RequestId: req.RequestId,
		Blocks:    resp,
	})
}

func handleBlocksByRange(_ Backend, msg Decoder, peer *Peer) error {
	res := new(BlocksByRangePacket)
	if err := msg.Decode(res); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}

	peer.log.Debug("receive BlocksByRange response")

	err := peer.dispatcher.dispatchResponse(&response{
		requestID: res.RequestId,
		data:      res,
		code:      BlocksByRangeMsg,
	})

	if err != nil {
		peer.Log().Errorf("failed to dispatch BlocksByRange response: %v", err)
	}

	return nil
}

// NodeInfo represents a short summary of the `bsc` sub-protocol metadata
// known about the host peer.
type NodeInfo struct{}

// nodeInfo retrieves some `bsc` protocol metadata about the running host node.
func nodeInfo() *NodeInfo {
	return &NodeInfo{}
}
