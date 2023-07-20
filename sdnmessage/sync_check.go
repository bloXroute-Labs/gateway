package sdnmessage

// TransactionSyncCheck is a check to determine whether the relay or proxy is synced from
// its perspective
type TransactionSyncCheck struct {
	RequestID string `json:"request_id"`
	IsSynced  bool   `json:"is_tx_service_synced"`
}
