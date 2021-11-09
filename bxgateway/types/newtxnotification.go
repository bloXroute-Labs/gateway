package types

import (
	"fmt"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	log "github.com/sirupsen/logrus"
)

// NewTransactionNotification - contains BxTransaction which contains the local region of the ethereum transaction and all its fields.
type NewTransactionNotification struct {
	*BxTransaction
	BlockchainTransaction
}

// CreateNewTransactionNotification -  creates NewTransactionNotification object which contains bxTransaction and local region
func CreateNewTransactionNotification(bxTx *BxTransaction) Notification {
	return &NewTransactionNotification{
		bxTx,
		nil,
	}
}

func (newTransactionNotification *NewTransactionNotification) makeBlockchainTransaction() error {
	var err error
	newTransactionNotification.BxTransaction.m.Lock()
	defer newTransactionNotification.BxTransaction.m.Unlock()
	if newTransactionNotification.BlockchainTransaction == nil {
		newTransactionNotification.BlockchainTransaction, err = newTransactionNotification.BxTransaction.BlockchainTransaction()
		if err != nil {
			err = fmt.Errorf("invalid tx with hash %v. error %v", newTransactionNotification.BxTransaction.Hash(), err)
			log.Errorf("failed in makeBlockchainTransaction - %v", err)
			return err
		}
	}
	return nil
}

//Filters - creates BlockchainTransaction if needs and returns a map of requested fields and their value for evaluation
func (newTransactionNotification *NewTransactionNotification) Filters(filters []string) map[string]interface{} {
	err := newTransactionNotification.makeBlockchainTransaction()
	if err != nil {
		return nil
	}
	return newTransactionNotification.BlockchainTransaction.Filters(filters)
}

// WithFields - creates BlockchainTransaction if needs and returns the value of requested fields of the transaction
func (newTransactionNotification *NewTransactionNotification) WithFields(fields []string) Notification {
	err := newTransactionNotification.makeBlockchainTransaction()
	if err != nil {
		return &NewTransactionNotification{
			nil,
			nil,
		}
	}

	newBlockchainTransaction := newTransactionNotification.BlockchainTransaction.WithFields(fields)
	return &NewTransactionNotification{
		nil,
		newBlockchainTransaction,
	}
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
