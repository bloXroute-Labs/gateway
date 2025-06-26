package types

import (
	"fmt"
	"sync"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// ErrInvalidTx is returned when the transaction is invalid
var ErrInvalidTx = fmt.Errorf("invalid tx")

// TxValidationStatus indicates the validation status of transaction notifications
type TxValidationStatus int

// TxValidationStatus types enumeration
const (
	TxPendingValidation TxValidationStatus = 0
	TxInvalid           TxValidationStatus = 1
	TxValid             TxValidationStatus = 2
)

// NewTransactionNotification - contains BxTransaction which contains the local region of the ethereum transaction and all its fields.
type NewTransactionNotification struct {
	*BxTransaction
	*EthTransaction
	validationStatus TxValidationStatus
	// lock is used to prevent parallel extract of sender address
	// while not locking the other unrelated go routines.
	lock *sync.Mutex
}

// CreateNewTransactionNotification -  creates NewTransactionNotification object which contains bxTransaction and local region
func CreateNewTransactionNotification(bxTx *BxTransaction) *NewTransactionNotification {
	return &NewTransactionNotification{
		BxTransaction:    bxTx,
		validationStatus: TxPendingValidation,
		lock:             &sync.Mutex{},
	}
}

// MakeEthTransaction creates blockchain transaction
func (n *NewTransactionNotification) MakeEthTransaction() error {
	n.lock.Lock()
	defer n.lock.Unlock()

	switch n.validationStatus {
	case TxPendingValidation:
		var err error
		n.EthTransaction, err = n.BxTransaction.MakeAndSetEthTransaction(n.BxTransaction.sender)
		if err != nil {
			n.validationStatus = TxInvalid
			err = fmt.Errorf("invalid tx with hash %v: %v", n.BxTransaction.Hash(), err)
			log.Errorf("failed in MakeEthTransaction: %v", err)
			return err
		}
		n.validationStatus = TxValid

		return nil
	case TxInvalid:
		return ErrInvalidTx
	default:
		return nil
	}
}

// Filters - creates EthTransaction if needs and returns a map of requested fields and their value for evaluation
func (n *NewTransactionNotification) Filters(filters []string) map[string]interface{} {
	err := n.MakeEthTransaction()
	if err != nil {
		return nil
	}

	return n.EthTransaction.Filters(filters)
}

// Fields - creates EthTransaction if needs and returns the value of requested fields of the transaction
func (n *NewTransactionNotification) Fields(fields []string) map[string]interface{} {
	err := n.MakeEthTransaction()
	if err != nil {
		return nil
	}

	return n.EthTransaction.Fields(fields)
}

// WithFields - creates EthTransaction if needs and returns the value of requested fields of the transaction
func (n *NewTransactionNotification) WithFields([]string) Notification {
	return nil
}

// LocalRegion - returns the local region of the ethereum transaction
func (n *NewTransactionNotification) LocalRegion() bool {
	return TFLocalRegion&n.BxTransaction.Flags() != 0
}

// GetHash - returns tha hash of EthTransaction
func (n *NewTransactionNotification) GetHash() string {
	return n.BxTransaction.hash.Format(true)
}

// RawTx - returns the tx raw content
// the tx bytes returned can be used directly to submit to RPC endpoint
// rlp.DecodeBytes is used for the wire protocol, while `MarshalBinary`/`UnmarshalBinary` is used for RPC interface
func (n *NewTransactionNotification) RawTx() []byte {
	if n.EthTransaction != nil {
		return n.EthTransaction.RawTx()
	}

	var rawTx ethtypes.Transaction
	err := rlp.DecodeBytes(n.BxTransaction.content, &rawTx)
	if err != nil {
		log.Infof("invalid tx content %v with hash %v. error %v", n.BxTransaction.content, n.BxTransaction.Hash(), err)
	}
	marshalledTxBytes, err := rawTx.MarshalBinary()
	if err != nil {
		log.Infof("invalid raw eth tx %v error %v", n.BxTransaction.Hash(), err)
	}
	return marshalledTxBytes
}

// NotificationType - returns the feed name notification
func (n *NewTransactionNotification) NotificationType() FeedType {
	return NewTxsFeed
}
