package blockchain

import (
	"github.com/bloXroute-Labs/gateway/blockchain/network"
	"github.com/bloXroute-Labs/gateway/types"
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
func (n NoOpBxBridge) SendTransactionsFromBDN(transactions []*types.BxTransaction) error {
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
func (n NoOpBxBridge) ReceiveNodeTransactions() <-chan TransactionsFromNode {
	return make(chan TransactionsFromNode)
}

// ReceiveBDNTransactions is a no-op
func (n NoOpBxBridge) ReceiveBDNTransactions() <-chan []*types.BxTransaction {
	return make(chan []*types.BxTransaction)
}

// ReceiveTransactionHashesAnnouncement is a no-op
func (n NoOpBxBridge) ReceiveTransactionHashesAnnouncement() <-chan TransactionAnnouncement {
	return make(chan TransactionAnnouncement)
}

// ReceiveTransactionHashesRequest is a no-op
func (n NoOpBxBridge) ReceiveTransactionHashesRequest() <-chan TransactionAnnouncement {
	return make(chan TransactionAnnouncement)
}

// AnnounceBlockHashes is a no-op
func (n NoOpBxBridge) AnnounceBlockHashes(peerID string, peerEndpoint types.NodeEndpoint, hashes types.SHA256HashList) error {
	return nil
}

// SendBlockToBDN is a no-op
func (n NoOpBxBridge) SendBlockToBDN(block *types.BxBlock, endpoint types.NodeEndpoint) error {
	return nil
}

// SendBlockToNode is a no-op
func (n NoOpBxBridge) SendBlockToNode(block *types.BxBlock) error {
	return nil
}

// RequestBlockHashFromNode is a no-op
func (n NoOpBxBridge) RequestBlockHashFromNode(peerID string, hash types.SHA256Hash) error {
	return nil
}

// ReceiveBlockFromBDN is a no-op
func (n NoOpBxBridge) ReceiveBlockFromBDN() <-chan *types.BxBlock {
	return make(chan *types.BxBlock)
}

// ReceiveBlockFromNode is a no-op
func (n NoOpBxBridge) ReceiveBlockFromNode() <-chan BlockFromNode {
	return make(chan BlockFromNode)
}

// ReceiveBlockAnnouncement is a no-op
func (n NoOpBxBridge) ReceiveBlockAnnouncement() <-chan BlockAnnouncement {
	return make(chan BlockAnnouncement)
}

// ReceiveBlockRequest is a no-op
func (n NoOpBxBridge) ReceiveBlockRequest() <-chan BlockAnnouncement {
	return make(chan BlockAnnouncement)
}

// ReceiveNoActiveBlockchainPeersAlert is a no-op
func (n NoOpBxBridge) ReceiveNoActiveBlockchainPeersAlert() <-chan NoActiveBlockchainPeersAlert {
	return make(chan NoActiveBlockchainPeersAlert)
}

// SendNoActiveBlockchainPeersAlert is a no-op
func (n NoOpBxBridge) SendNoActiveBlockchainPeersAlert() error {
	return nil
}
