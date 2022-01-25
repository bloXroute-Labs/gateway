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
