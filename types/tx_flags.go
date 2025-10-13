package types

// TxFlags represents various flags controlling routing behavior of transactions
type TxFlags uint16

// flag constant values
const (
	_ TxFlags = 1 << iota // Deprecated: TFStatusMonitoring not used
	TFPaidTx
	_ // Deprecated: TFNonceMonitoring not used
	_ // Deprecated: TFRePropagate not used
	_ // Deprecated: TFEnterpriseSender
	TFLocalRegion
	_ // Deprecated: TFPrivateTx not used
	_ // Deprecated: TFEliteSender
	TFDeliverToNode
	_ //  Deprecated: TFValidatorsOnly used for validators only txs
	TFReusedNonce
	_ //  Deprecated: TFNextValidator used for semi private tx
	_ // Deprecated: TFNextValidatorRebroadcast  used for semi private tx
	_ // Deprecated: TFFrontRunningProtection
	TFWithSidecar
	TFForBuilders // [NOTICE] last flag
)

// IsPaid indicates whether the transaction is considered paid from the user and consumes quota
func (f TxFlags) IsPaid() bool {
	return f&TFPaidTx != 0
}

// ShouldDeliverToNode indicates whether the transaction should be forwarded to the blockchain node
func (f TxFlags) ShouldDeliverToNode() bool {
	return f&TFDeliverToNode != 0
}

// IsReuseSenderNonce indicates whether the transaction is reusing an existing nonce
func (f TxFlags) IsReuseSenderNonce() bool {
	return f&TFReusedNonce != 0
}

// IsWithSidecar return true if transaction have blobs sidecar
func (f TxFlags) IsWithSidecar() bool {
	return f&TFWithSidecar != 0
}

// IsForBuilders return true if transaction is to be sent to builders
func (f TxFlags) IsForBuilders() bool {
	return f&TFForBuilders != 0
}
