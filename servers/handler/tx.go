package handler

import (
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// HandleSingleTransaction handles a single tx, returns txHash, a boolean value indicating if it was successfully or not and an error only if we need to send it back to the caller
func HandleSingleTransaction(
	node connections.BxListener,
	nodeWSManager blockchain.WSManager,
	transaction string,
	txSender []byte,
	conn connections.Conn,
	validatorsOnly,
	nodeValidationRequested,
	frontRunningProtection bool,
	gatewayChainID bxtypes.NetworkID,
) (string, bool, error) {

	txContent, err := types.DecodeHex(transaction)
	if err != nil {
		return "", false, err
	}
	tx, pendingReevaluation, err := validateTxFromExternalSource(transaction, txContent, validatorsOnly, gatewayChainID, nodeValidationRequested, nodeWSManager, conn, frontRunningProtection)
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
	}

	return tx.Hash().String(), true, nil
}

// validateTxFromExternalSource validate transaction from external source (ws / grpc), return bool indicates if tx is pending reevaluation
func validateTxFromExternalSource(transaction string, txBytes []byte, validatorsOnly bool, gatewayChainID bxtypes.NetworkID, nodeValidationRequested bool, wsManager blockchain.WSManager, source connections.Conn, frontRunningProtection bool) (*bxmessage.Tx, bool, error) {
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

	if ethTx.ChainId().Int64() != 0 && gatewayChainID != 0 && bxtypes.NetworkID(ethTx.ChainId().Int64()) != gatewayChainID {
		log.Debugf("chainID mismatch for hash %v - tx chainID %v, gateway networkNum %v networkChainID %v", ethTx.Hash().String(), ethTx.ChainId().Int64(), networkNum, gatewayChainID)
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

	if nodeValidationRequested && !tx.Flags().IsValidatorsOnly() {
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
