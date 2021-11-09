package blockchain

import (
	"errors"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/blockchain/network"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/types"
)

// NoActiveBlockchainPeersAlert is used to send an alert to the gateway on initial liveliness check if no active blockchain peers
type NoActiveBlockchainPeersAlert struct {
}

// TransactionAnnouncement represents an available transaction from a given peer that can be requested
type TransactionAnnouncement struct {
	Hashes types.SHA256HashList
	PeerID string
}

// TransactionsFromNode is used to pass transactions from a node to the BDN
type TransactionsFromNode struct {
	Transactions []*types.BxTransaction
	PeerEndpoint types.NodeEndpoint
}

// BlockFromNode is used to pass blocks from a node to the BDN
type BlockFromNode struct {
	Block        *types.BxBlock
	PeerEndpoint types.NodeEndpoint
}

// BlockAnnouncement represents an available block from a given peer that can be requested
type BlockAnnouncement struct {
	Hash         types.SHA256Hash
	PeerID       string
	PeerEndpoint types.NodeEndpoint
}

// Converter defines an interface for converting between blockchain and BDN transactions
type Converter interface {
	TransactionBlockchainToBDN(interface{}) (*types.BxTransaction, error)
	TransactionBDNToBlockchain(*types.BxTransaction) (interface{}, error)
	BlockBlockchainToBDN(interface{}) (*types.BxBlock, error)
	BlockBDNtoBlockchain(block *types.BxBlock) (interface{}, error)
	BxBlockToCanonicFormat(*types.BxBlock) (*types.BlockNotification, error)
}

// constants for transaction channel buffer sizes
const (
	transactionBacklog       = 500
	transactionHashesBacklog = 1000
	blockBacklog             = 100
)

// Bridge represents the application interface over which messages are passed between the blockchain node and the BDN
type Bridge interface {
	Converter

	ReceiveNetworkConfigUpdates() <-chan network.EthConfig
	UpdateNetworkConfig(network.EthConfig) error

	AnnounceTransactionHashes(string, types.SHA256HashList) error
	SendTransactionsFromBDN([]*types.BxTransaction) error
	SendTransactionsToBDN(txs []*types.BxTransaction, peerEndpoint types.NodeEndpoint) error
	RequestTransactionsFromNode(string, types.SHA256HashList) error

	ReceiveNodeTransactions() <-chan TransactionsFromNode
	ReceiveBDNTransactions() <-chan []*types.BxTransaction
	ReceiveTransactionHashesAnnouncement() <-chan TransactionAnnouncement
	ReceiveTransactionHashesRequest() <-chan TransactionAnnouncement

	AnnounceBlockHashes(string, types.NodeEndpoint, types.SHA256HashList) error
	SendBlockToBDN(*types.BxBlock, types.NodeEndpoint) error
	SendBlockToNode(*types.BxBlock) error
	RequestBlockHashFromNode(string, types.SHA256Hash) error

	ReceiveBlockFromBDN() <-chan *types.BxBlock
	ReceiveBlockFromNode() <-chan BlockFromNode
	ReceiveBlockAnnouncement() <-chan BlockAnnouncement
	ReceiveBlockRequest() <-chan BlockAnnouncement

	ReceiveNoActiveBlockchainPeersAlert() <-chan NoActiveBlockchainPeersAlert
	SendNoActiveBlockchainPeersAlert() error
}

// ErrChannelFull is a special error for identifying overflowing channel buffers
var ErrChannelFull = errors.New("channel full")

// BxBridge is a channel based implementation of the Bridge interface
type BxBridge struct {
	Converter
	config                    chan network.EthConfig
	transactionsFromNode      chan TransactionsFromNode
	transactionsFromBDN       chan []*types.BxTransaction
	transactionHashesFromNode chan TransactionAnnouncement
	transactionHashesRequests chan TransactionAnnouncement

	blocksFromNode              chan BlockFromNode
	blocksFromBDN               chan *types.BxBlock
	blockHashFromNode           chan BlockAnnouncement
	blockHashRequests           chan BlockAnnouncement
	missingBlockRequestsFromBDN chan int

	noActiveBlockchainPeers chan NoActiveBlockchainPeersAlert
}

// NewBxBridge returns a BxBridge instance
func NewBxBridge(converter Converter) Bridge {
	return &BxBridge{
		config:                    make(chan network.EthConfig),
		transactionsFromNode:      make(chan TransactionsFromNode, transactionBacklog),
		transactionsFromBDN:       make(chan []*types.BxTransaction, transactionBacklog),
		transactionHashesFromNode: make(chan TransactionAnnouncement, transactionHashesBacklog),
		transactionHashesRequests: make(chan TransactionAnnouncement, transactionHashesBacklog),
		blocksFromNode:            make(chan BlockFromNode, blockBacklog),
		blocksFromBDN:             make(chan *types.BxBlock, blockBacklog),
		blockHashFromNode:         make(chan BlockAnnouncement, blockBacklog),
		blockHashRequests:         make(chan BlockAnnouncement, blockBacklog),
		noActiveBlockchainPeers:   make(chan NoActiveBlockchainPeersAlert),
		Converter:                 converter,
	}
}

// ReceiveNetworkConfigUpdates provides a channel with network config updates
func (b *BxBridge) ReceiveNetworkConfigUpdates() <-chan network.EthConfig {
	return b.config
}

// UpdateNetworkConfig pushes a new Ethereum configuration update
func (b *BxBridge) UpdateNetworkConfig(config network.EthConfig) error {
	b.config <- config
	return nil
}

// AnnounceTransactionHashes pushes a series of transaction announcements onto the announcements channel
func (b BxBridge) AnnounceTransactionHashes(peerID string, hashes types.SHA256HashList) error {
	select {
	case b.transactionHashesFromNode <- TransactionAnnouncement{Hashes: hashes, PeerID: peerID}:
		return nil
	default:
		return ErrChannelFull
	}
}

// RequestTransactionsFromNode requests a series of transactions that a peer node has announced
func (b BxBridge) RequestTransactionsFromNode(peerID string, hashes types.SHA256HashList) error {
	select {
	case b.transactionHashesRequests <- TransactionAnnouncement{Hashes: hashes, PeerID: peerID}:
		return nil
	default:
		return ErrChannelFull
	}
}

// SendTransactionsFromBDN sends a set of transactions from the BDN for distribution to nodes
func (b BxBridge) SendTransactionsFromBDN(transactions []*types.BxTransaction) error {
	select {
	case b.transactionsFromBDN <- transactions:
		return nil
	default:
		return ErrChannelFull
	}
}

// SendTransactionsToBDN sends a set of transactions from a node to the BDN for propagation
func (b BxBridge) SendTransactionsToBDN(txs []*types.BxTransaction, peerEndpoint types.NodeEndpoint) error {
	select {
	case b.transactionsFromNode <- TransactionsFromNode{Transactions: txs, PeerEndpoint: peerEndpoint}:
		return nil
	default:
		return ErrChannelFull
	}
}

// ReceiveNodeTransactions provides a channel that pushes transactions as they come in from nodes
func (b BxBridge) ReceiveNodeTransactions() <-chan TransactionsFromNode {
	return b.transactionsFromNode
}

// ReceiveBDNTransactions provides a channel that pushes transactions as they arrive from the BDN
func (b BxBridge) ReceiveBDNTransactions() <-chan []*types.BxTransaction {
	return b.transactionsFromBDN
}

// ReceiveTransactionHashesAnnouncement provides a channel that pushes announcements as nodes announce them
func (b BxBridge) ReceiveTransactionHashesAnnouncement() <-chan TransactionAnnouncement {
	return b.transactionHashesFromNode
}

// ReceiveTransactionHashesRequest provides a channel that pushes requests for transaction hashes from the BDN
func (b BxBridge) ReceiveTransactionHashesRequest() <-chan TransactionAnnouncement {
	return b.transactionHashesRequests
}

// SendBlockToBDN sends a block from a node to the BDN
func (b BxBridge) SendBlockToBDN(block *types.BxBlock, peerEndpoint types.NodeEndpoint) error {
	select {
	case b.blocksFromNode <- BlockFromNode{Block: block, PeerEndpoint: peerEndpoint}:
		return nil
	default:
		return ErrChannelFull
	}
}

// SendBlockToNode sends a block from the BDN for distribution to nodes
func (b BxBridge) SendBlockToNode(block *types.BxBlock) error {
	select {
	case b.blocksFromBDN <- block:
		return nil
	default:
		return ErrChannelFull
	}
}

// ReceiveBlockFromNode provides a channel that pushes blocks as they come in from nodes
func (b BxBridge) ReceiveBlockFromNode() <-chan BlockFromNode {
	return b.blocksFromNode
}

// ReceiveBlockFromBDN provides a channel that pushes new blocks from the BDN
func (b BxBridge) ReceiveBlockFromBDN() <-chan *types.BxBlock {
	return b.blocksFromBDN
}

// AnnounceBlockHashes pushes a series of block announcements onto the announcements channel
func (b *BxBridge) AnnounceBlockHashes(peerID string, peerEndpoint types.NodeEndpoint, hashes types.SHA256HashList) error {
	for _, hash := range hashes {
		select {
		case b.blockHashFromNode <- BlockAnnouncement{hash, peerID, peerEndpoint}:
		default:
			return ErrChannelFull
		}
	}
	return nil
}

// RequestBlockHashFromNode requests a block hash that has been announced and should be available for fetching
func (b *BxBridge) RequestBlockHashFromNode(peerID string, hash types.SHA256Hash) error {
	select {
	case b.blockHashRequests <- BlockAnnouncement{Hash: hash, PeerID: peerID}:
		return nil
	default:
		return ErrChannelFull
	}
}

// ReceiveBlockAnnouncement provides a channel that pushes available blocks from nodes
func (b *BxBridge) ReceiveBlockAnnouncement() <-chan BlockAnnouncement {
	return b.blockHashFromNode
}

// ReceiveBlockRequest provides a channel for the BDN to push requests for blocks
func (b *BxBridge) ReceiveBlockRequest() <-chan BlockAnnouncement {
	return b.blockHashRequests
}

// SendNoActiveBlockchainPeersAlert sends alerts to the BDN when there is no active blockchain peer
func (b BxBridge) SendNoActiveBlockchainPeersAlert() error {
	select {
	case b.noActiveBlockchainPeers <- NoActiveBlockchainPeersAlert{}:
		return nil
	default:
		return ErrChannelFull
	}
}

// ReceiveNoActiveBlockchainPeersAlert provides a channel that pushes no active blockchain peer alerts
func (b BxBridge) ReceiveNoActiveBlockchainPeersAlert() <-chan NoActiveBlockchainPeersAlert {
	return b.noActiveBlockchainPeers
}
