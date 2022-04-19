package eth

import (
	"context"
	log "github.com/bloXroute-Labs/gateway/logger"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"time"
)

// SupportedProtocols is the list of all Ethereum devp2p protocols supported by this client
var SupportedProtocols = []uint{
	eth.ETH65, eth.ETH66,
}

// ProtocolLengths is a mapping of each supported devp2p protocol to its message version length
var ProtocolLengths = map[uint]uint64{eth.ETH65: 17, eth.ETH66: 17}

// MakeProtocols generates the set of supported protocols structs for the p2p server
func MakeProtocols(ctx context.Context, backend Backend) []p2p.Protocol {
	protocols := make([]p2p.Protocol, 0, len(SupportedProtocols))
	for _, version := range SupportedProtocols {
		protocols = append(protocols, makeProtocol(ctx, backend, version, ProtocolLengths[version]))
	}
	return protocols
}

func makeProtocol(ctx context.Context, backend Backend, version uint, versionLength uint64) p2p.Protocol {
	return p2p.Protocol{
		Name:    "eth",
		Version: version,
		Length:  versionLength,
		Run: func(p *p2p.Peer, rw p2p.MsgReadWriter) error {
			ep := NewPeer(ctx, p, rw, version)

			config := backend.NetworkConfig()
			peerStatus, err := ep.Handshake(uint32(version), config.Network, config.TotalDifficulty, config.Head, config.Genesis)
			if err != nil {
				return err
			}

			// process status message on backend to set initial total difficulty
			_ = backend.Handle(ep, peerStatus)

			return backend.RunPeer(ep, func(peer *Peer) error {
				for {
					if err = handleMessage(backend, ep); err != nil {
						return err
					}
				}
			})
		},
		NodeInfo: func() interface{} {
			return nil
		},
		PeerInfo: func(id enode.ID) interface{} {
			return nil
		},
	}
}

type msgHandler func(backend Backend, msg Decoder, peer *Peer) error

// Decoder represents any struct that can be decoded into an Ethereum message type
type Decoder interface {
	Decode(val interface{}) error
}

var eth65 = map[uint64]msgHandler{
	eth.GetBlockHeadersMsg:            handleGetBlockHeaders,
	eth.BlockHeadersMsg:               handleBlockHeaders,
	eth.GetBlockBodiesMsg:             handleGetBlockBodies,
	eth.BlockBodiesMsg:                handleBlockBodies,
	eth.GetNodeDataMsg:                handleUnimplemented,
	eth.NodeDataMsg:                   handleUnimplemented,
	eth.GetReceiptsMsg:                handleUnimplemented,
	eth.ReceiptsMsg:                   handleUnimplemented,
	eth.NewBlockHashesMsg:             handleNewBlockHashes,
	eth.NewBlockMsg:                   handleNewBlockMsg,
	eth.TransactionsMsg:               handleTransactions,
	eth.NewPooledTransactionHashesMsg: handleNewPooledTransactionHashes,
	eth.GetPooledTransactionsMsg:      handleUnimplemented,
	eth.PooledTransactionsMsg:         handlePooledTransactions,
}

var eth66 = map[uint64]msgHandler{
	eth.NewBlockHashesMsg:             handleNewBlockHashes,
	eth.NewBlockMsg:                   handleNewBlockMsg,
	eth.TransactionsMsg:               handleTransactions,
	eth.NewPooledTransactionHashesMsg: handleNewPooledTransactionHashes,
	// eth66 messages have request-id
	eth.GetBlockHeadersMsg:       handleGetBlockHeaders66,
	eth.BlockHeadersMsg:          handleBlockHeaders66,
	eth.GetBlockBodiesMsg:        handleGetBlockBodies66,
	eth.BlockBodiesMsg:           handleBlockBodies66,
	eth.GetNodeDataMsg:           handleUnimplemented,
	eth.NodeDataMsg:              handleUnimplemented,
	eth.GetReceiptsMsg:           handleUnimplemented,
	eth.ReceiptsMsg:              handleUnimplemented,
	eth.GetPooledTransactionsMsg: handleUnimplemented,
	eth.PooledTransactionsMsg:    handlePooledTransactions66,
}

func handleMessage(backend Backend, peer *Peer) error {
	msg, err := peer.rw.ReadMsg()
	if err != nil {
		return err
	}

	startTime := time.Now()
	defer func() {
		_ = msg.Discard()
		log.Tracef("%v: handling message with code: %v took %v", peer, msg.Code, time.Since(startTime))
	}()

	handlers := eth65
	if peer.version >= eth.ETH66 {
		handlers = eth66
	}
	handler, ok := handlers[msg.Code]

	if ok {
		return handler(backend, msg, peer)
	}
	return nil
}
