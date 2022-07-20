package blockchain

import (
	"errors"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// NoActiveBlockchainPeersAlert is used to send an alert to the gateway on initial liveliness check if no active blockchain peers
type NoActiveBlockchainPeersAlert struct {
}

// TransactionAnnouncement represents an available transaction from a given peer that can be requested
type TransactionAnnouncement struct {
	Hashes types.SHA256HashList
	PeerID string
}

// Transactions is used to pass transactions between a node and the BDN
type Transactions struct {
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
	statusBacklog            = 10
)

// Bridge represents the application interface over which messages are passed between the blockchain node and the BDN
type Bridge interface {
	Converter

	ReceiveNetworkConfigUpdates() <-chan network.EthConfig
	UpdateNetworkConfig(network.EthConfig) error

	AnnounceTransactionHashes(string, types.SHA256HashList) error
	SendTransactionsFromBDN(transactions Transactions) error
	SendTransactionsToBDN(txs []*types.BxTransaction, peerEndpoint types.NodeEndpoint) error
	RequestTransactionsFromNode(string, types.SHA256HashList) error

	ReceiveNodeTransactions() <-chan Transactions
	ReceiveBDNTransactions() <-chan Transactions
	ReceiveTransactionHashesAnnouncement() <-chan TransactionAnnouncement
	ReceiveTransactionHashesRequest() <-chan TransactionAnnouncement

	SendBlockToBDN(*types.BxBlock, types.NodeEndpoint) error
	SendBlockToNode(*types.BxBlock) error
	SendConfirmedBlockToGateway(block *types.BxBlock, peerEndpoint types.NodeEndpoint) error

	ReceiveBlockFromBDN() <-chan *types.BxBlock
	ReceiveBlockFromNode() <-chan BlockFromNode
	ReceiveConfirmedBlockFromNode() <-chan BlockFromNode

	ReceiveNoActiveBlockchainPeersAlert() <-chan NoActiveBlockchainPeersAlert
	SendNoActiveBlockchainPeersAlert() error

	SendBlockchainStatusRequest() error
	ReceiveBlockchainStatusRequest() <-chan struct{}
	SendBlockchainStatusResponse([]*types.NodeEndpoint) error
	ReceiveBlockchainStatusResponse() <-chan []*types.NodeEndpoint
}

// ErrChannelFull is a special error for identifying overflowing channel buffers
var ErrChannelFull = errors.New("channel full")

// BxBridge is a channel based implementation of the Bridge interface
type BxBridge struct {
	Converter
	config                    chan network.EthConfig
	transactionsFromNode      chan Transactions
	transactionsFromBDN       chan Transactions
	transactionHashesFromNode chan TransactionAnnouncement
	transactionHashesRequests chan TransactionAnnouncement

	blocksFromNode         chan BlockFromNode
	blocksFromBDN          chan *types.BxBlock
	confirmedBlockFromNode chan BlockFromNode

	noActiveBlockchainPeers chan NoActiveBlockchainPeersAlert

	blockchainStatusRequest  chan struct{}
	blockchainStatusResponse chan []*types.NodeEndpoint
}

// NewBxBridge returns a BxBridge instance
func NewBxBridge(converter Converter) Bridge {
	return &BxBridge{
		config:                    make(chan network.EthConfig, 1),
		transactionsFromNode:      make(chan Transactions, transactionBacklog),
		transactionsFromBDN:       make(chan Transactions, transactionBacklog),
		transactionHashesFromNode: make(chan TransactionAnnouncement, transactionHashesBacklog),
		transactionHashesRequests: make(chan TransactionAnnouncement, transactionHashesBacklog),
		blocksFromNode:            make(chan BlockFromNode, blockBacklog),
		blocksFromBDN:             make(chan *types.BxBlock, blockBacklog),
		confirmedBlockFromNode:    make(chan BlockFromNode, blockBacklog),
		noActiveBlockchainPeers:   make(chan NoActiveBlockchainPeersAlert),
		blockchainStatusRequest:   make(chan struct{}, statusBacklog),
		blockchainStatusResponse:  make(chan []*types.NodeEndpoint, statusBacklog),
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
func (b BxBridge) SendTransactionsFromBDN(transactions Transactions) error {
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
	case b.transactionsFromNode <- Transactions{Transactions: txs, PeerEndpoint: peerEndpoint}:
		return nil
	default:
		return ErrChannelFull
	}
}

// SendConfirmedBlockToGateway sends a SHA256 of the block to be included in blockConfirm message
func (b BxBridge) SendConfirmedBlockToGateway(block *types.BxBlock, peerEndpoint types.NodeEndpoint) error {
	select {
	case b.confirmedBlockFromNode <- BlockFromNode{Block: block, PeerEndpoint: peerEndpoint}:
		return nil
	default:
		return ErrChannelFull
	}
}

// ReceiveNodeTransactions provides a channel that pushes transactions as they come in from nodes
func (b BxBridge) ReceiveNodeTransactions() <-chan Transactions {
	return b.transactionsFromNode
}

// ReceiveBDNTransactions provides a channel that pushes transactions as they arrive from the BDN
func (b BxBridge) ReceiveBDNTransactions() <-chan Transactions {
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

// ReceiveConfirmedBlockFromNode provides a channel that pushes confirmed blocks from nodes
func (b BxBridge) ReceiveConfirmedBlockFromNode() <-chan BlockFromNode {
	return b.confirmedBlockFromNode
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

// SendBlockchainStatusRequest sends a blockchain connection status request signal to a blockchain backend
func (b BxBridge) SendBlockchainStatusRequest() error {
	select {
	case b.blockchainStatusRequest <- struct{}{}:
		return nil
	default:
		return ErrChannelFull
	}
}

// ReceiveBlockchainStatusRequest handles SendBlockchainStatusRequest signal
func (b BxBridge) ReceiveBlockchainStatusRequest() <-chan struct{} {
	return b.blockchainStatusRequest
}

// SendBlockchainStatusResponse sends a response for blockchain connection status request
func (b BxBridge) SendBlockchainStatusResponse(endpoints []*types.NodeEndpoint) error {
	select {
	case b.blockchainStatusResponse <- endpoints:
		return nil
	default:
		return ErrChannelFull
	}
}

// ReceiveBlockchainStatusResponse handles blockchain connection status response from backend
func (b BxBridge) ReceiveBlockchainStatusResponse() <-chan []*types.NodeEndpoint {
	return b.blockchainStatusResponse
}
