package types

import (
	"fmt"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"sync"
)

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
	BlockchainTransaction
	validationStatus TxValidationStatus
	// lock is used to prevent parallel extract of sender address
	// while not locking the other unrelated go routines.
	lock *sync.Mutex
}

// CreateNewTransactionNotification -  creates NewTransactionNotification object which contains bxTransaction and local region
func CreateNewTransactionNotification(bxTx *BxTransaction) *NewTransactionNotification {
	return &NewTransactionNotification{
		bxTx,
		nil,
		TxPendingValidation,
		&sync.Mutex{},
	}
}

// MakeBlockchainTransaction creates blockchain transaction
func (newTransactionNotification *NewTransactionNotification) MakeBlockchainTransaction() error {
	var err error
	newTransactionNotification.lock.Lock()
	defer newTransactionNotification.lock.Unlock()
	if newTransactionNotification.validationStatus == TxPendingValidation {
		newTransactionNotification.BlockchainTransaction, err =
			newTransactionNotification.BxTransaction.BlockchainTransaction(newTransactionNotification.Sender())
		if err != nil {
			newTransactionNotification.validationStatus = TxInvalid
			err = fmt.Errorf("invalid tx with hash %v. error %v", newTransactionNotification.BxTransaction.Hash(), err)
			log.Errorf("failed in MakeBlockchainTransaction - %v", err)
			return err
		}
		newTransactionNotification.validationStatus = TxValid
	}
	if newTransactionNotification.validationStatus == TxInvalid {
		return fmt.Errorf("invalid tx")
	}
	return nil
}

//Filters - creates BlockchainTransaction if needs and returns a map of requested fields and their value for evaluation
func (newTransactionNotification *NewTransactionNotification) Filters(filters []string) map[string]interface{} {
	err := newTransactionNotification.MakeBlockchainTransaction()
	if err != nil {
		return nil
	}
	return newTransactionNotification.BlockchainTransaction.Filters(filters)
}

// Fields - creates BlockchainTransaction if needs and returns the value of requested fields of the transaction
func (newTransactionNotification *NewTransactionNotification) Fields(fields []string) map[string]interface{} {
	err := newTransactionNotification.MakeBlockchainTransaction()
	if err != nil {
		return nil
	}
	ethTx := newTransactionNotification.BlockchainTransaction.(*EthTransaction)
	return ethTx.Fields(fields)
}

// WithFields - creates BlockchainTransaction if needs and returns the value of requested fields of the transaction
func (newTransactionNotification *NewTransactionNotification) WithFields(fields []string) Notification {
	return nil
}

// LocalRegion - returns the local region of the ethereum transaction
func (newTransactionNotification *NewTransactionNotification) LocalRegion() bool {
	return TFLocalRegion&newTransactionNotification.BxTransaction.Flags() != 0
}

// GetHash - returns tha hash of BlockchainTransaction
func (newTransactionNotification *NewTransactionNotification) GetHash() string {
	return newTransactionNotification.BxTransaction.hash.Format(true)
}

// RawTx - returns the tx raw content
// the tx bytes returned can be used directly to submit to RPC endpoint
// rlp.DecodeBytes is used for the wire protocol, while `MarshalBinary`/`UnmarshalBinary` is used for RPC interface
func (newTransactionNotification *NewTransactionNotification) RawTx() []byte {
	var rawTx ethtypes.Transaction
	err := rlp.DecodeBytes(newTransactionNotification.BxTransaction.content, &rawTx)
	if err != nil {
		log.Infof("invalid tx content %v with hash %v. error %v", newTransactionNotification.BxTransaction.content, newTransactionNotification.BxTransaction.Hash(), err)
	}
	marshalledTxBytes, err := rawTx.MarshalBinary()
	if err != nil {
		log.Infof("invalid raw eth tx %v error %v", newTransactionNotification.BxTransaction.Hash(), err)
	}
	return marshalledTxBytes
}

// NotificationType - returns the feed name notification
func (newTransactionNotification *NewTransactionNotification) NotificationType() FeedType {
	return NewTxsFeed
}
