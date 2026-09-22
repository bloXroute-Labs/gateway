package types

// NewTransactionNotification represents a transaction notification with lazily evaluated fields.
type NewTransactionNotification struct {
	*EthTransaction
	hash        SHA256Hash
	localRegion bool
}

// CreateNewTransactionNotification creates NewTransactionNotification.
// ethTx is the shared parsed transaction; it must not be nil.
func CreateNewTransactionNotification(hash SHA256Hash, flags TxFlags, ethTx *EthTransaction) *NewTransactionNotification {
	return &NewTransactionNotification{
		EthTransaction: ethTx,
		hash:           hash,
		localRegion:    TFLocalRegion&flags != 0,
	}
}

// Filters returns a map of requested fields and their value for evaluation
func (n *NewTransactionNotification) Filters() (map[string]interface{}, error) {
	return n.EthTransaction.Filters()
}

// Fields returns the value of requested fields of the transaction
func (n *NewTransactionNotification) Fields(fields []string) (map[string]interface{}, error) {
	return n.EthTransaction.Fields(fields)
}

// WithFields returns the value of requested fields of the transaction
func (n *NewTransactionNotification) WithFields([]string) Notification {
	return nil
}

// LocalRegion - returns the local region of the ethereum transaction
func (n *NewTransactionNotification) LocalRegion() bool {
	return n.localRegion
}

// GetHash - returns the hash of the transaction
func (n *NewTransactionNotification) GetHash() string {
	return n.hash.Format(true)
}

// RawTx returns the tx raw content (lazy computed in EthTransaction).
func (n *NewTransactionNotification) RawTx() ([]byte, error) {
	return n.EthTransaction.RawTx()
}

// NotificationType - returns the feed name notification
func (n *NewTransactionNotification) NotificationType() FeedType {
	return NewTxsFeed
}
