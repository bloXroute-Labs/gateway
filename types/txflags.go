package types

// TxFlags represents various flags controlling routing behavior of transactions
type TxFlags uint16

// flag constant values
const (
	TFStatusMonitoring TxFlags = 1 << iota
	TFPaidTx
	TFNonceMonitoring
	TFRePropagate
	TFEnterpriseSender
	TFLocalRegion
	TFPrivateTx
	TFEliteSender
	TFDeliverToNode
	TFValidatorsOnly
	TFReusedNonce

	TFStatusTrack = TFStatusMonitoring | TFPaidTx
	TFNonceTrack  = TFNonceMonitoring | TFStatusTrack
)

// IsPaid indicates whether the transaction is considered paid from the user and consumes quota
func (f TxFlags) IsPaid() bool {
	return f&TFPaidTx != 0
}

// ShouldDeliverToNode indicates whether the transaction should be forwarded to the blockchain node
func (f TxFlags) ShouldDeliverToNode() bool {
	return f&TFDeliverToNode != 0
}

// IsValidatorsOnly indicates whether the transaction should be forwarded to miner gateways only
func (f TxFlags) IsValidatorsOnly() bool {
	return f&TFValidatorsOnly != 0
}

// IsReuseSenderNonce indicates whether the transaction is reusing an existing nonce
func (f TxFlags) IsReuseSenderNonce() bool {
	return f&TFReusedNonce != 0
}
