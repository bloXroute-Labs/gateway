package blockchain

import (
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/types"
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

// TransactionBlockchainToBDN is a no-op
func (n NoOpBxBridge) TransactionBlockchainToBDN(i interface{}) (*types.BxTransaction, error) {
	return nil, nil
}

// TransactionBDNToBlockchain is a no-op
func (n NoOpBxBridge) TransactionBDNToBlockchain(transaction *types.BxTransaction) (interface{}, error) {
	return nil, nil
}

// BlockBlockchainToBDN is a no-op
func (n NoOpBxBridge) BlockBlockchainToBDN(i interface{}) (*types.BxBlock, error) {
	return nil, nil
}

// BlockBDNtoBlockchain is a no-op
func (n NoOpBxBridge) BlockBDNtoBlockchain(block *types.BxBlock) (interface{}, error) {
	return nil, nil
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
func (n NoOpBxBridge) AnnounceTransactionHashes(s string, list types.SHA256HashList) error {
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

// ReceiveBlockchainStatusRequest is a no-op
func (n NoOpBxBridge) ReceiveBlockchainStatusRequest() <-chan struct{} { return make(chan struct{}) }

// SendBlockchainStatusResponse is a no-op
func (n NoOpBxBridge) SendBlockchainStatusResponse([]*types.NodeEndpoint) error { return nil }

// ReceiveBlockchainStatusResponse is a no-op
func (n NoOpBxBridge) ReceiveBlockchainStatusResponse() <-chan []*types.NodeEndpoint {
	return make(chan []*types.NodeEndpoint)
}
