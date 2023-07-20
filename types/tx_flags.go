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
	TFNextValidator
	TFNextValidatorRebroadcast
	TFFrontRunningProtection
	_
	_ // [NOTICE] last flag

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

// IsPaidTx indicates is transaction is paid transaction
func (f TxFlags) IsPaidTx() bool {
	return f&TFPaidTx != 0
}

// IsDeliverToNode return true if transaction marked to deliver to node
func (f TxFlags) IsDeliverToNode() bool {
	return f&TFDeliverToNode != 0
}

// IsNextValidator return true if transaction marked for next validator
func (f TxFlags) IsNextValidator() bool {
	return f&TFNextValidator != 0
}

// IsNextValidatorRebroadcast return true if transaction is fallback tx of next validator tx
func (f TxFlags) IsNextValidatorRebroadcast() bool {
	return f&TFNextValidatorRebroadcast != 0
}

// IsFrontRunningProtection return true if transaction is enabled with front runnig protection
func (f TxFlags) IsFrontRunningProtection() bool {
	return f&TFFrontRunningProtection != 0
}
