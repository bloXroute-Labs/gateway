package blockchain

import (
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

// NoOpBxBridge is a placeholder bridge that still operates as a Converter
type NoOpBxBridge struct {
	Converter
}

// NewNoOpBridge is a placeholder bridge implementation for starting the node without any blockchain connections, so that there's no blocking on channels
func NewNoOpBridge(converter Converter) Bridge {
	return &NoOpBxBridge{
		Converter: converter,
	}
}

// ReceiveNetworkConfigUpdates is a no-op
func (n NoOpBxBridge) ReceiveNetworkConfigUpdates() <-chan network.EthConfig {
	return make(chan network.EthConfig)
}

// UpdateNetworkConfig is a no-op
func (n NoOpBxBridge) UpdateNetworkConfig(config network.EthConfig) error {
	return nil
}

// AnnounceTransactionHashes is a no-op
func (n NoOpBxBridge) AnnounceTransactionHashes(s string, list types.SHA256HashList, e types.NodeEndpoint) error {
	return nil
}

// SendTransactionsFromBDN is a no-op
func (n NoOpBxBridge) SendTransactionsFromBDN(transactions Transactions) error {
	return nil
}

// SendTransactionsToBDN is a no-op
func (n NoOpBxBridge) SendTransactionsToBDN(txs []*types.BxTransaction, peerEndpoint types.NodeEndpoint) error {
	return nil
}

// RequestTransactionsFromNode is a no-op
func (n NoOpBxBridge) RequestTransactionsFromNode(s string, list types.SHA256HashList) error {
	return nil
}

// RequestTransactionsFromBDN is no-op
func (n NoOpBxBridge) RequestTransactionsFromBDN(requestID string, hashes types.SHA256HashList) error {
	return nil
}

// ReceiveTransactionHashesRequestFromNode is no-op
func (n NoOpBxBridge) ReceiveTransactionHashesRequestFromNode() <-chan TransactionsRequest {
	return make(chan TransactionsRequest)
}

// SendRequestedTransactionsToNode is no-op
func (n NoOpBxBridge) SendRequestedTransactionsToNode(requestID string, transactions []*types.BxTransaction) error {
	return nil
}

// ReceiveRequestedTransactionsFromBDN is no-op
func (n NoOpBxBridge) ReceiveRequestedTransactionsFromBDN() <-chan TransactionsResponse {
	return make(chan TransactionsResponse)
}

// ReceiveNodeTransactions is a no-op
func (n NoOpBxBridge) ReceiveNodeTransactions() <-chan Transactions {
	return make(chan Transactions)
}

// ReceiveBDNTransactions is a no-op
func (n NoOpBxBridge) ReceiveBDNTransactions() <-chan Transactions {
	return make(chan Transactions)
}

// ReceiveTransactionHashesAnnouncement is a no-op
func (n NoOpBxBridge) ReceiveTransactionHashesAnnouncement() <-chan TransactionAnnouncement {
	return make(chan TransactionAnnouncement)
}

// ReceiveTransactionHashesRequest is a no-op
func (n NoOpBxBridge) ReceiveTransactionHashesRequest() <-chan TransactionAnnouncement {
	return make(chan TransactionAnnouncement)
}

// SendBlockToBDN is a no-op
func (n NoOpBxBridge) SendBlockToBDN(block *types.BxBlock, endpoint types.NodeEndpoint) error {
	return nil
}

// SendBlockToNode is a no-op
func (n NoOpBxBridge) SendBlockToNode(block *types.BxBlock) error {
	return nil
}

// ReceiveEthBlockFromBDN is a no-op
func (n NoOpBxBridge) ReceiveEthBlockFromBDN() <-chan *types.BxBlock {
	return make(chan *types.BxBlock)
}

// ReceiveBeaconBlockFromBDN is a no-op
func (n NoOpBxBridge) ReceiveBeaconBlockFromBDN() <-chan *types.BxBlock {
	return make(chan *types.BxBlock)
}

// ReceiveBlockFromNode is a no-op
func (n NoOpBxBridge) ReceiveBlockFromNode() <-chan BlockFromNode {
	return make(chan BlockFromNode)
}

// ReceiveConfirmedBlockFromNode is a no-op
func (n NoOpBxBridge) ReceiveConfirmedBlockFromNode() <-chan BlockFromNode {
	return nil
}

// SendBeaconMessageToBDN is a no-op
func (n NoOpBxBridge) SendBeaconMessageToBDN(*types.BxBeaconMessage, types.NodeEndpoint) error {
	return nil
}

// SendBeaconMessageToBlockchain is a no-op
func (n NoOpBxBridge) SendBeaconMessageToBlockchain(*types.BxBeaconMessage) error {
	return nil
}

// ReceiveBeaconMessageFromBDN is a no-op
func (n NoOpBxBridge) ReceiveBeaconMessageFromBDN() <-chan *types.BxBeaconMessage {
	return make(chan *types.BxBeaconMessage)
}

// ReceiveBeaconMessageFromNode is a no-op
func (n NoOpBxBridge) ReceiveBeaconMessageFromNode() <-chan BeaconMessageFromNode {
	return make(chan BeaconMessageFromNode)
}

// ReceiveNoActiveBlockchainPeersAlert is a no-op
func (n NoOpBxBridge) ReceiveNoActiveBlockchainPeersAlert() <-chan NoActiveBlockchainPeersAlert {
	return make(chan NoActiveBlockchainPeersAlert)
}

// SendNoActiveBlockchainPeersAlert is a no-op
func (n NoOpBxBridge) SendNoActiveBlockchainPeersAlert() error {
	return nil
}

// SendConfirmedBlockToGateway is a no-op
func (n NoOpBxBridge) SendConfirmedBlockToGateway(block *types.BxBlock, peerEndpoint types.NodeEndpoint) error {
	return nil
}

// SendBlockchainStatusRequest is a no-op
func (n NoOpBxBridge) SendBlockchainStatusRequest() error { return nil }

// SendTrustedPeerRequest is a no-op
func (n NoOpBxBridge) SendTrustedPeerRequest() error { return nil }

// ReceiveBlockchainStatusResponse is a no-op
func (n NoOpBxBridge) ReceiveBlockchainStatusResponse() ([]*types.NodeEndpoint, error) {
	return nil, nil
}

// ReceiveTrustedPeerResponse is a no-op
func (n NoOpBxBridge) ReceiveTrustedPeerResponse() ([]peer.ID, error) {
	return nil, nil
}

// SubscribeStatus is a no-op
func (n NoOpBxBridge) SubscribeStatus(_ bool) StatusSubscription { return nil }

// SendNodeConnectionCheckRequest is a no-op
func (n NoOpBxBridge) SendNodeConnectionCheckRequest() error { return nil }

// ReceiveNodeConnectionCheckRequest is a no-op
func (n NoOpBxBridge) ReceiveNodeConnectionCheckRequest() <-chan struct{} {
	return make(chan struct{})
}

// SendNodeConnectionCheckResponse is a no-op
func (n NoOpBxBridge) SendNodeConnectionCheckResponse(types.NodeEndpoint) error { return nil }

// ReceiveNodeConnectionCheckResponse is a no-op
func (n NoOpBxBridge) ReceiveNodeConnectionCheckResponse() <-chan types.NodeEndpoint {
	return make(chan types.NodeEndpoint)
}

// SendBlockchainConnectionStatus is a no-op
func (n NoOpBxBridge) SendBlockchainConnectionStatus(ConnectionStatus) error { return nil }

// ReceiveBlockchainConnectionStatus is a no-op
func (n NoOpBxBridge) ReceiveBlockchainConnectionStatus() <-chan ConnectionStatus {
	return make(chan ConnectionStatus)
}

// SendDisconnectEvent is a no-op
func (n NoOpBxBridge) SendDisconnectEvent(endpoint types.NodeEndpoint) error { return nil }

// ReceiveDisconnectEvent is a no-op
func (n NoOpBxBridge) ReceiveDisconnectEvent() <-chan types.NodeEndpoint {
	return make(chan types.NodeEndpoint)
}
