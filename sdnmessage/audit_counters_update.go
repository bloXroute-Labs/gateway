package sdnmessage

// AccountAuditCountersUpdate represents an audit counter update for an account
type AccountAuditCountersUpdate struct {
	AccountID     string `json:"account_id"`
	TxPaidCounter int    `json:"tx_paid_counter"`
}

// AuditCountersUpdate represents an audit counter phase update for accounts
type AuditCountersUpdate struct {
	AccountUpdates []AccountAuditCountersUpdate `json:"account_updates"`
	PhaseKey       int                          `json:"phase_key"`
}

// AuditCountersUpdateRequest represents a request from the SDN for an AuditCountersUpdate
type AuditCountersUpdateRequest struct {
	PhaseKey int `json:"phase_key"`
}
