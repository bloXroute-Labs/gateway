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
