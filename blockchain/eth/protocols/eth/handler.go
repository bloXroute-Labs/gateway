package eth

import (
	"context"
	"time"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/core"
)

// Handler is a callback to invoke from an outside runner after the boilerplate
// exchanges have passed.
type Handler func(peer *Peer) error

// Backend defines the data retrieval methods to serve remote requests and the
// callback methods to invoke on remote deliveries.
type Backend interface {
	// Chain retrieves the blockchain object to serve data.
	Chain() *core.Chain

	// RunPeer is invoked when a peer joins on the `eth` protocol. The handler
	// should do any peer maintenance work, handshakes and validations. If all
	// is passed, control should be given back to the `handler` to process the
	// inbound messages going forward.
	RunPeer(peer *Peer, handler Handler) error

	// PeerInfo retrieves all known `eth` information about a peer.
	PeerInfo(id enode.ID) interface{}

	// Handle is a callback to be invoked when a data packet is received from
	// the remote peer. Only packets not consumed by the protocol handler will
	// be forwarded to the backend.
	Handle(peer *Peer, packet Packet) error

	// RequestTransactions retrieves the transactions for the given hashes.
	RequestTransactions(hashes []ethcommon.Hash) ([]rlp.RawValue, error)
}

// MakeProtocols constructs the P2P protocol definitions for `eth`.
func MakeProtocols(ctx context.Context, backend Backend, network uint64) []p2p.Protocol {
	netProtocols, ok := supportedProtocols[network]
	if !ok {
		return nil
	}

	log.Infof("creating eth protocols for network %d with versions: %v", network, netProtocols)

	protocols := make([]p2p.Protocol, 0, len(netProtocols))

	for _, version := range netProtocols {
		protocols = append(protocols, p2p.Protocol{
			Name:    ProtocolName,
			Version: version,
			Length:  protocolLengths[version],
			Run: func(p *p2p.Peer, rw p2p.MsgReadWriter) error {
				peer := NewPeer(ctx, p, rw, version, network)

				return backend.RunPeer(peer, func(peer *Peer) error {
					return handle(backend, peer)
				})
			},
			NodeInfo: func() interface{} {
				return nil
			},
			PeerInfo: func(id enode.ID) interface{} {
				return backend.PeerInfo(id)
			},
		})
	}
	return protocols
}

// handle is invoked whenever an `eth` connection is made that successfully passes
// the protocol handshake. This method will keep processing messages until the
// connection is torn down.
func handle(backend Backend, peer *Peer) error {
	for {
		if err := ReadAndHandle(backend, peer); err != nil {
			peer.Log().Errorf("message handling failed in `eth`: %v", err)
			return err
		}
	}
}

type msgHandler func(backend Backend, msg Decoder, peer *Peer) error

var eth66 = map[uint64]msgHandler{
	eth.NewBlockHashesMsg:             handleNewBlockHashes,
	eth.NewBlockMsg:                   handleNewBlockMsg,
	eth.TransactionsMsg:               handleTransactions,
	eth.NewPooledTransactionHashesMsg: handleNewPooledTransactionHashes,
	// eth66 messages have request-id
	eth.GetBlockHeadersMsg:       handleGetBlockHeaders,
	eth.BlockHeadersMsg:          handleBlockHeaders,
	eth.GetBlockBodiesMsg:        handleGetBlockBodies,
	eth.BlockBodiesMsg:           handleBlockBodies,
	GetNodeDataMsg:               handleUnimplemented,
	NodeDataMsg:                  handleUnimplemented,
	eth.GetReceiptsMsg:           handleUnimplemented,
	eth.ReceiptsMsg:              handleUnimplemented,
	eth.GetPooledTransactionsMsg: handleGetPooledTransactions,
	eth.PooledTransactionsMsg:    handlePooledTransactions,
}

var eth67 = map[uint64]msgHandler{
	eth.NewBlockHashesMsg:             handleNewBlockHashes,
	eth.NewBlockMsg:                   handleNewBlockMsg,
	eth.TransactionsMsg:               handleTransactions,
	eth.NewPooledTransactionHashesMsg: handleNewPooledTransactionHashes,
	eth.GetBlockHeadersMsg:            handleGetBlockHeaders,
	eth.BlockHeadersMsg:               handleBlockHeaders,
	eth.GetBlockBodiesMsg:             handleGetBlockBodies,
	eth.BlockBodiesMsg:                handleBlockBodies,
	eth.GetReceiptsMsg:                handleUnimplemented,
	eth.ReceiptsMsg:                   handleUnimplemented,
	eth.GetPooledTransactionsMsg:      handleGetPooledTransactions,
	eth.PooledTransactionsMsg:         handlePooledTransactions,
}

var eth68 = map[uint64]msgHandler{
	eth.NewBlockHashesMsg:             handleNewBlockHashes,
	eth.NewBlockMsg:                   handleNewBlockMsg,
	eth.TransactionsMsg:               handleTransactions,
	eth.NewPooledTransactionHashesMsg: handleNewPooledTransactionHashes68,
	eth.GetBlockHeadersMsg:            handleGetBlockHeaders,
	eth.BlockHeadersMsg:               handleBlockHeaders,
	eth.GetBlockBodiesMsg:             handleGetBlockBodies,
	eth.BlockBodiesMsg:                handleBlockBodies,
	eth.GetReceiptsMsg:                handleUnimplemented,
	eth.ReceiptsMsg:                   handleUnimplemented,
	eth.GetPooledTransactionsMsg:      handleGetPooledTransactions,
	eth.PooledTransactionsMsg:         handlePooledTransactions,
}

// ReadAndHandle is invoked whenever an inbound message is received from a remote peer
func ReadAndHandle(backend Backend, peer *Peer) error {
	msg, err := peer.rw.ReadMsg()
	if err != nil {
		return err
	}

	return HandleMassage(backend, msg, peer)
}

// HandleMassage is invoked whenever an inbound message is received from a remote peer
func HandleMassage(backend Backend, msg p2p.Msg, peer *Peer) error {
	startTime := time.Now()
	defer func() {
		_ = msg.Discard() //nolint:errcheck
		peer.log.Tracef("%v: handling message with code: %v took %v", peer, msg.Code, time.Since(startTime))
	}()

	handlers := eth66
	switch peer.version {
	case ETH67:
		handlers = eth67
	case eth.ETH68:
		handlers = eth68
	}

	handler, ok := handlers[msg.Code]
	if ok {
		return handler(backend, msg, peer)
	}

	log.Warnf("unknown message code %d from peer %s", msg.Code, peer.ID())

	return nil
}
