package handler

import (
	"errors"
	"fmt"
	"strings"
	"time"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/services/validator"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// HandleSingleTransaction handles a single tx, returns txHash, a boolean value indicating if it was successfully or not and an error only if we need to send it back to the caller
func HandleSingleTransaction(
	node connections.BxListener,
	nodeWSManager blockchain.WSManager,
	validatorsManager *validator.Manager,
	transaction string,
	txSender []byte,
	conn connections.Conn,
	validatorsOnly,
	nextValidator,
	nodeValidationRequested,
	frontRunningProtection bool,
	fallback uint16,
	gatewayChainID types.NetworkID,
) (string, bool, error) {

	txContent, err := types.DecodeHex(transaction)
	if err != nil {
		return "", false, err
	}
	tx, pendingReevaluation, err := validateTxFromExternalSource(validatorsManager, transaction, txContent, validatorsOnly, gatewayChainID, nextValidator, fallback, nodeValidationRequested, nodeWSManager, conn, frontRunningProtection)
	if err != nil {
		return "", false, err
	}

	// This is an option to assign the sender of the tx manually in order to save time from tx processing
	if txSender != nil {
		var sender types.Sender
		copy(sender[:], txSender)
		tx.SetSender(sender)
	}

	if !pendingReevaluation {
		// call the Handler. Don't invoke in a go routine
		err = node.HandleMsg(tx, conn, connections.RunForeground)
		if err != nil {
			// TODO in this case validation fails but we are not returning any error back (so we are not sending anything to the sender)
			log.Errorf("failed to handle single transaction: %v", err)
			return "", false, nil
		}
	} else if fallback != 0 {
		// BSC first validator was not accessible and fallback > BSCBlockTime
		// in case fallback time is up before next validator is evaluated, send tx as normal tx at fallback time
		// (tx with fallback less than BSCBlockTime are not marked as pending)
		time.AfterFunc(time.Duration(uint64(fallback)*bxgateway.MillisecondsToNanosecondsMultiplier), func() {
			validatorsManager.Lock()
			defer validatorsManager.Unlock()

			pendingNextValidatorTxsMap := validatorsManager.GetPendingNextValidatorTxs()

			if _, exists := pendingNextValidatorTxsMap[tx.Hash().String()]; exists {
				delete(pendingNextValidatorTxsMap, tx.Hash().String())
				log.Infof("sending next validator tx %v because fallback time reached", tx.Hash().String())

				tx.RemoveFlags(types.TFNextValidator)
				tx.SetFallback(0)
				err = node.HandleMsg(tx, conn, connections.RunForeground)
				if err != nil {
					log.Errorf("failed to send pending next validator tx %v at fallback time: %v", tx.Hash().String(), err)
				}
			}
		})
	}

	return tx.Hash().String(), true, nil
}

// validateTxFromExternalSource validate transaction from external source (ws / grpc), return bool indicates if tx is pending reevaluation
func validateTxFromExternalSource(validatorsManager *validator.Manager, transaction string, txBytes []byte, validatorsOnly bool, gatewayChainID types.NetworkID, nextValidator bool, fallback uint16, nodeValidationRequested bool, wsManager blockchain.WSManager, source connections.Conn, frontRunningProtection bool) (*bxmessage.Tx, bool, error) {
	// Ethereum's transactions encoding for RPC interfaces is slightly different from the RLP encoded format, so decode + re-encode the transaction for consistency.
	// Specifically, note `UnmarshalBinary` should be used for RPC interfaces, and rlp.DecodeBytes should be used for the wire protocol.
	var ethTx ethtypes.Transaction
	err := ethTx.UnmarshalBinary(txBytes)
	if err != nil {
		// If UnmarshalBinary failed, we will try RLP in case user made mistake
		e := rlp.DecodeBytes(txBytes, &ethTx)
		if e != nil {
			return nil, false, fmt.Errorf("failed to unmarshal tx: %w", err)
		}
		log.Warnf("Ethereum transaction was in RLP format instead of binary," +
			" transaction has been processed anyway, but it'd be best to use the Ethereum binary standard encoding")
	}

	networkNum := source.GetNetworkNum()
	accountID := source.GetAccountID()

	if ethTx.ChainId().Int64() != 0 && gatewayChainID != 0 && types.NetworkID(ethTx.ChainId().Int64()) != gatewayChainID {
		log.Debugf("chainID mismatch for hash %v - tx chainID %v , gateway networkNum %v networkChainID %v", ethTx.Hash().String(), ethTx.ChainId().Int64(), networkNum, gatewayChainID)
		return nil, false, fmt.Errorf("chainID mismatch for hash %v, expect %v got %v, make sure the tx is sent with the right blockchain network", ethTx.Hash().String(), gatewayChainID, ethTx.ChainId().Int64())
	}

	txContent, err := rlp.EncodeToBytes(&ethTx)

	if err != nil {
		return nil, false, err
	}

	var txFlags = types.TFPaidTx | types.TFLocalRegion
	switch {
	case validatorsOnly:
		txFlags |= types.TFValidatorsOnly
	case nextValidator:
		txFlags |= types.TFNextValidator
	default:
		txFlags |= types.TFDeliverToNode
	}

	if frontRunningProtection {
		txFlags |= types.TFFrontRunningProtection
	}

	var hash types.SHA256Hash
	copy(hash[:], ethTx.Hash().Bytes())

	// should set the account of the sender, not the account of the gateway itself
	tx := bxmessage.NewTx(hash, txContent, networkNum, txFlags, accountID)
	if nextValidator {
		txPendingReevaluation, err := validatorsManager.ProcessNextValidatorTx(tx, fallback, networkNum, source)
		if err != nil {
			return nil, false, err
		}
		if txPendingReevaluation {
			return tx, true, nil
		}
	}

	if nodeValidationRequested && !tx.Flags().IsNextValidator() && !tx.Flags().IsValidatorsOnly() {
		syncedWS, ok := wsManager.SyncedProvider()
		if ok {
			_, err := syncedWS.SendTransaction(
				fmt.Sprintf("%v%v", "0x", transaction),
				blockchain.RPCOptions{
					RetryAttempts: 1,
					RetryInterval: 10 * time.Millisecond,
				},
			)
			if err != nil {
				if !strings.Contains(err.Error(), "already known") { // gateway propagates tx to node before doing this check
					errMsg := fmt.Sprintf("tx (%v) failed node validation with error: %v", tx.Hash(), err.Error())
					return nil, false, errors.New(errMsg)
				}
			}
		} else {
			return nil, false, fmt.Errorf("failed to validate tx (%v) via node: no synced WS provider available", tx.Hash())
		}
	}
	return tx, false, nil
}
