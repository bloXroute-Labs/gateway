package blockchain

import (
	"errors"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

// NoActiveBlockchainPeersAlert is used to send an alert to the gateway on initial liveliness check if no active blockchain peers
type NoActiveBlockchainPeersAlert struct{}

// TransactionAnnouncement represents an available transaction from a given peer that can be requested
type TransactionAnnouncement struct {
	Hashes       types.SHA256HashList
	PeerID       string
	PeerEndpoint types.NodeEndpoint
}

// TransactionsRequest is used to request transactions from the gateway
type TransactionsRequest struct {
	Hashes    types.SHA256HashList
	RequestID string
}

// TransactionsResponse is used to pass transactions between a node and the BDN
type TransactionsResponse struct {
	Transactions []*types.BxTransaction
	RequestID    string
}

// Transactions is used to pass transactions between a node and the BDN
type Transactions struct {
	Transactions   []*types.BxTransaction
	PeerEndpoint   types.NodeEndpoint
	ConnectionType utils.NodeType
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

// ConnectionStatus represents blockchain connection status
type ConnectionStatus struct {
	PeerEndpoint types.NodeEndpoint
	IsConnected  bool
	IsDynamic    bool
}

// BeaconMessageFromNode is used to pass beacon messages from a node to the BDN
type BeaconMessageFromNode struct {
	Message      *types.BxBeaconMessage
	PeerEndpoint types.NodeEndpoint
}

// Converter defines an interface for converting between blockchain and BDN transactions
type Converter interface {
	TransactionBlockchainToBDN(interface{}) (*types.BxTransaction, error)
	TransactionBDNToBlockchain(*types.BxTransaction) (interface{}, error)
	BlockBlockchainToBDN(interface{}) (*types.BxBlock, error)
	BlockBDNtoBlockchain(block *types.BxBlock) (interface{}, error)

	BeaconMessageToBDN(interface{}) (*types.BxBeaconMessage, error)
	BeaconMessageBDNToBlockchain(*types.BxBeaconMessage) (interface{}, error)
}

// constants for transaction channel buffer sizes
const (
	transactionBacklog       = 2000
	transactionHashesBacklog = 1000
	blockBacklog             = 100
	statusBacklog            = 10
)

// Bridge represents the application interface over which messages are passed between the blockchain node and the BDN
type Bridge interface {
	Converter

	ReceiveNetworkConfigUpdates() <-chan network.EthConfig
	UpdateNetworkConfig(network.EthConfig) error

	AnnounceTransactionHashes(string, types.SHA256HashList, types.NodeEndpoint) error
	SendTransactionsFromBDN(transactions Transactions) error
	SendTransactionsToBDN(txs []*types.BxTransaction, peerEndpoint types.NodeEndpoint) error
	RequestTransactionsFromNode(string, types.SHA256HashList) error

	ReceiveNodeTransactions() <-chan Transactions
	ReceiveBDNTransactions() <-chan Transactions
	ReceiveTransactionHashesAnnouncement() <-chan TransactionAnnouncement
	ReceiveTransactionHashesRequest() <-chan TransactionAnnouncement

	// pooled transactions request
	RequestTransactionsFromBDN(string, types.SHA256HashList) error
	ReceiveTransactionHashesRequestFromNode() <-chan TransactionsRequest

	// pooled transactions response
	SendRequestedTransactionsToNode(string, []*types.BxTransaction) error
	ReceiveRequestedTransactionsFromBDN() <-chan TransactionsResponse

	SendBlockToBDN(*types.BxBlock, types.NodeEndpoint) error
	SendBlockToNode(*types.BxBlock) error
	SendConfirmedBlockToGateway(block *types.BxBlock, peerEndpoint types.NodeEndpoint) error

	ReceiveEthBlockFromBDN() <-chan *types.BxBlock
	ReceiveBeaconBlockFromBDN() <-chan *types.BxBlock
	ReceiveBlockFromNode() <-chan BlockFromNode
	ReceiveConfirmedBlockFromNode() <-chan BlockFromNode

	SendBeaconMessageToBDN(*types.BxBeaconMessage, types.NodeEndpoint) error
	SendBeaconMessageToBlockchain(*types.BxBeaconMessage) error
	ReceiveBeaconMessageFromBDN() <-chan *types.BxBeaconMessage
	ReceiveBeaconMessageFromNode() <-chan BeaconMessageFromNode

	ReceiveNoActiveBlockchainPeersAlert() <-chan NoActiveBlockchainPeersAlert
	SendNoActiveBlockchainPeersAlert() error

	SendBlockchainStatusRequest() error
	ReceiveBlockchainStatusRequest() <-chan struct{}
	SendBlockchainStatusResponse([]*types.NodeEndpoint) error
	ReceiveBlockchainStatusResponse() <-chan []*types.NodeEndpoint
	SendBlockchainConnectionStatus(ConnectionStatus) error
	ReceiveBlockchainConnectionStatus() <-chan ConnectionStatus

	SendNodeConnectionCheckRequest() error
	ReceiveNodeConnectionCheckRequest() <-chan struct{}
	SendNodeConnectionCheckResponse(types.NodeEndpoint) error
	ReceiveNodeConnectionCheckResponse() <-chan types.NodeEndpoint

	SendDisconnectEvent(endpoint types.NodeEndpoint) error
	ReceiveDisconnectEvent() <-chan types.NodeEndpoint

	SendConnectedEvent(endpoint types.NodeEndpoint) error
	ReceiveConnectEvent() <-chan types.NodeEndpoint
}

// Errors
var (
	ErrChannelFull = errors.New("channel full") // ErrChannelFull is a special error for identifying overflowing channel buffers
)

// BxBridge is a channel based implementation of the Bridge interface
type BxBridge struct {
	Converter
	config                           chan network.EthConfig
	transactionsFromNode             chan Transactions
	transactionsFromBDN              chan Transactions
	transactionHashesFromNode        chan TransactionAnnouncement
	transactionHashesRequests        chan TransactionAnnouncement
	transactionHashesRequestsFromBDN chan TransactionsRequest
	requestedTransactions            chan TransactionsResponse

	withBeacon bool

	blocksFromNode      chan BlockFromNode
	ethBlocksFromBDN    chan *types.BxBlock
	beaconBlocksFromBDN chan *types.BxBlock

	beaconMessageFromNode chan BeaconMessageFromNode
	beaconMessageFromBDN  chan *types.BxBeaconMessage

	confirmedBlockFromNode chan BlockFromNode

	noActiveBlockchainPeers chan NoActiveBlockchainPeersAlert

	blockchainStatusRequest     chan struct{}
	blockchainStatusResponse    chan []*types.NodeEndpoint
	nodeConnectionCheckRequest  chan struct{}
	nodeConnectionCheckResponse chan types.NodeEndpoint
	blockchainConnectionStatus  chan ConnectionStatus
	disconnectEvent             chan types.NodeEndpoint
	connectEvent                chan types.NodeEndpoint
}

// NewBxBridge returns a BxBridge instance
func NewBxBridge(converter Converter, withBeacon bool) Bridge {
	return &BxBridge{
		config:                           make(chan network.EthConfig, 1),
		transactionsFromNode:             make(chan Transactions, transactionBacklog),
		transactionsFromBDN:              make(chan Transactions, transactionBacklog),
		transactionHashesFromNode:        make(chan TransactionAnnouncement, transactionHashesBacklog),
		transactionHashesRequests:        make(chan TransactionAnnouncement, transactionHashesBacklog),
		transactionHashesRequestsFromBDN: make(chan TransactionsRequest, transactionHashesBacklog),
		requestedTransactions:            make(chan TransactionsResponse, transactionBacklog),
		withBeacon:                       withBeacon,
		blocksFromNode:                   make(chan BlockFromNode, blockBacklog),
		ethBlocksFromBDN:                 make(chan *types.BxBlock, blockBacklog),
		beaconBlocksFromBDN:              make(chan *types.BxBlock, blockBacklog),
		beaconMessageFromNode:            make(chan BeaconMessageFromNode, blockBacklog),
		beaconMessageFromBDN:             make(chan *types.BxBeaconMessage, blockBacklog),
		confirmedBlockFromNode:           make(chan BlockFromNode, blockBacklog),
		noActiveBlockchainPeers:          make(chan NoActiveBlockchainPeersAlert),
		blockchainStatusRequest:          make(chan struct{}, statusBacklog),
		blockchainStatusResponse:         make(chan []*types.NodeEndpoint, statusBacklog),
		nodeConnectionCheckRequest:       make(chan struct{}, statusBacklog),
		nodeConnectionCheckResponse:      make(chan types.NodeEndpoint, statusBacklog),
		blockchainConnectionStatus:       make(chan ConnectionStatus, transactionBacklog),
		disconnectEvent:                  make(chan types.NodeEndpoint, statusBacklog),
		connectEvent:                     make(chan types.NodeEndpoint, statusBacklog),
		Converter:                        converter,
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
func (b BxBridge) AnnounceTransactionHashes(peerID string, hashes types.SHA256HashList, endpoint types.NodeEndpoint) error {
	select {
	case b.transactionHashesFromNode <- TransactionAnnouncement{Hashes: hashes, PeerID: peerID, PeerEndpoint: endpoint}:
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

// RequestTransactionsFromBDN requests a series of transactions from the BDN
func (b BxBridge) RequestTransactionsFromBDN(requestID string, hashes types.SHA256HashList) error {
	select {
	case b.transactionHashesRequestsFromBDN <- TransactionsRequest{Hashes: hashes, RequestID: requestID}:
		return nil
	default:
		return ErrChannelFull
	}
}

// ReceiveTransactionHashesRequestFromNode provides a channel that pushes requests for transaction hashes from a node
func (b BxBridge) ReceiveTransactionHashesRequestFromNode() <-chan TransactionsRequest {
	return b.transactionHashesRequestsFromBDN
}

// SendRequestedTransactionsToNode sends a set of requested transaction to the node
func (b BxBridge) SendRequestedTransactionsToNode(requestID string, transactions []*types.BxTransaction) error {
	select {
	case b.requestedTransactions <- TransactionsResponse{Transactions: transactions, RequestID: requestID}:
		return nil
	default:
		return ErrChannelFull
	}
}

// ReceiveRequestedTransactionsFromBDN provides a channel that pushes requested transactions from the BDN
func (b BxBridge) ReceiveRequestedTransactionsFromBDN() <-chan TransactionsResponse {
	return b.requestedTransactions
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
	switch block.Type {
	case types.BxBlockTypeEth:
		select {
		case b.ethBlocksFromBDN <- block:
		default:
			return ErrChannelFull
		}
	case types.BxBlockTypeBeaconPhase0, types.BxBlockTypeBeaconAltair, types.BxBlockTypeBeaconBellatrix, types.BxBlockTypeBeaconCapella, types.BxBlockTypeBeaconDeneb:
		// No listener, `b.withBeacon` is true if the gateway started with a beacon P2P node or Beacon API
		if !b.withBeacon {
			return nil
		}

		select {
		case b.beaconBlocksFromBDN <- block:
		default:
			return ErrChannelFull
		}
	default:
		return fmt.Errorf("could not send block %v with type %v", block.Hash(), block.Type)
	}

	return nil
}

// ReceiveBlockFromNode provides a channel that pushes blocks as they come in from nodes
func (b BxBridge) ReceiveBlockFromNode() <-chan BlockFromNode {
	return b.blocksFromNode
}

// ReceiveEthBlockFromBDN provides a channel that pushes new eth blocks from the BDN
func (b BxBridge) ReceiveEthBlockFromBDN() <-chan *types.BxBlock {
	return b.ethBlocksFromBDN
}

// ReceiveBeaconBlockFromBDN provides a channel that pushes new beacon blocks from the BDN
func (b BxBridge) ReceiveBeaconBlockFromBDN() <-chan *types.BxBlock {
	return b.beaconBlocksFromBDN
}

// ReceiveConfirmedBlockFromNode provides a channel that pushes confirmed blocks from nodes
func (b BxBridge) ReceiveConfirmedBlockFromNode() <-chan BlockFromNode {
	return b.confirmedBlockFromNode
}

// SendBeaconMessageToBDN sends a beacon message from a node to the BDN
func (b BxBridge) SendBeaconMessageToBDN(msg *types.BxBeaconMessage, nodeEnpoint types.NodeEndpoint) error {
	select {
	case b.beaconMessageFromNode <- BeaconMessageFromNode{Message: msg, PeerEndpoint: nodeEnpoint}:
		return nil
	default:
		return ErrChannelFull
	}
}

// SendBeaconMessageToBlockchain sends a beacon message from the BDN to the blockchain node
func (b BxBridge) SendBeaconMessageToBlockchain(msg *types.BxBeaconMessage) error {
	// No listener, `b.withBeacon` is true if the gateway started with a beacon P2P node or Beacon API
	if !b.withBeacon {
		return nil
	}

	select {
	case b.beaconMessageFromBDN <- msg:
		return nil
	default:
		return ErrChannelFull
	}
}

// ReceiveBeaconMessageFromBDN provides a channel that pushes beacon messages from the BDN
func (b BxBridge) ReceiveBeaconMessageFromBDN() <-chan *types.BxBeaconMessage {
	return b.beaconMessageFromBDN
}

// ReceiveBeaconMessageFromNode provides a channel that pushes beacon messages from nodes
func (b BxBridge) ReceiveBeaconMessageFromNode() <-chan BeaconMessageFromNode {
	return b.beaconMessageFromNode
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

// SendNodeConnectionCheckRequest sends a request for node connection check request
func (b BxBridge) SendNodeConnectionCheckRequest() error {
	select {
	case b.nodeConnectionCheckRequest <- struct{}{}:
		return nil
	default:
		return ErrChannelFull
	}
}

// ReceiveNodeConnectionCheckRequest handles node connection check request from backend
func (b BxBridge) ReceiveNodeConnectionCheckRequest() <-chan struct{} {
	return b.nodeConnectionCheckRequest
}

// SendNodeConnectionCheckResponse sends a response for node connection check request
func (b BxBridge) SendNodeConnectionCheckResponse(endpoints types.NodeEndpoint) error {
	select {
	case b.nodeConnectionCheckResponse <- endpoints:
		return nil
	default:
		return ErrChannelFull
	}
}

// ReceiveNodeConnectionCheckResponse handles node connection check response from backend
func (b BxBridge) ReceiveNodeConnectionCheckResponse() <-chan types.NodeEndpoint {
	return b.nodeConnectionCheckResponse
}

// SendBlockchainConnectionStatus sends blockchain connection status
func (b BxBridge) SendBlockchainConnectionStatus(connStatus ConnectionStatus) error {
	select {
	case b.blockchainConnectionStatus <- connStatus:
		return nil
	default:
		return ErrChannelFull
	}
}

// ReceiveBlockchainConnectionStatus handles blockchain connection status
func (b BxBridge) ReceiveBlockchainConnectionStatus() <-chan ConnectionStatus {
	return b.blockchainConnectionStatus
}

// SendDisconnectEvent send disconnect event
func (b BxBridge) SendDisconnectEvent(endpoint types.NodeEndpoint) error {
	select {
	case b.disconnectEvent <- endpoint:
		return nil
	default:
		return ErrChannelFull
	}
}

// ReceiveDisconnectEvent handles disconnect event
func (b BxBridge) ReceiveDisconnectEvent() <-chan types.NodeEndpoint {
	return b.disconnectEvent
}

// SendConnectedEvent send connected event
func (b BxBridge) SendConnectedEvent(endpoint types.NodeEndpoint) error {
	select {
	case b.connectEvent <- endpoint:
		return nil
	default:
		return ErrChannelFull
	}
}

// ReceiveConnectEvent receive connected event
func (b BxBridge) ReceiveConnectEvent() <-chan types.NodeEndpoint {
	return b.connectEvent
}
