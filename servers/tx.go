package servers

import (
	"strings"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils/orderedmap"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/zhouzhuojie/conditions"
)

// HandleSingleTransaction handles a single tx, returns txHash, a boolean value indicating if it was successfully or not and an error only if we need to send it back to the caller
func HandleSingleTransaction(
	feedManager *FeedManager,
	transaction string,
	txSender []byte,
	conn connections.Conn,
	validatorsOnly,
	nextValidator,
	nodeValidationRequested,
	frontRunningProtection bool,
	fallback uint16,
	nextValidatorMap *orderedmap.OrderedMap,
	validatorStatusMap *syncmap.SyncMap[string, bool],
) (string, bool, error) {

	feedManager.LockPendingNextValidatorTxs()

	txContent, err := types.DecodeHex(transaction)
	if err != nil {
		return "", false, err
	}
	tx, pendingReevaluation, err := validateTxFromExternalSource(transaction, txContent, validatorsOnly, feedManager.chainID, nextValidator, fallback, nextValidatorMap, validatorStatusMap, feedManager.networkNum, conn.GetAccountID(), nodeValidationRequested, feedManager.nodeWSManager, conn, feedManager.pendingBSCNextValidatorTxHashToInfo, frontRunningProtection)
	feedManager.UnlockPendingNextValidatorTxs()
	if err != nil {
		return "", false, err
	}

	// This is an option to assign the sender of the tx manually in order to save time from tx processing
	if txSender != nil {
		var sender types.Sender
		copy(sender[:], txSender[:])
		tx.SetSender(sender)
	}

	if !pendingReevaluation {
		// call the Handler. Don't invoke in a go routine
		err = feedManager.node.HandleMsg(tx, conn, connections.RunForeground)
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
			feedManager.LockPendingNextValidatorTxs()
			defer feedManager.UnlockPendingNextValidatorTxs()
			if _, exists := feedManager.pendingBSCNextValidatorTxHashToInfo[tx.Hash().String()]; exists {
				delete(feedManager.pendingBSCNextValidatorTxHashToInfo, tx.Hash().String())
				log.Infof("sending next validator tx %v because fallback time reached", tx.Hash().String())

				tx.RemoveFlags(types.TFNextValidator)
				tx.SetFallback(0)
				err = feedManager.node.HandleMsg(tx, conn, connections.RunForeground)
				if err != nil {
					log.Errorf("failed to send pending next validator tx %v at fallback time: %v", tx.Hash().String(), err)
				}
			}
		})
	}

	return tx.Hash().String(), true, nil
}

func shouldSendTx(clientReq *clientReq, tx *types.NewTransactionNotification, remoteAddress string, accountID types.AccountID) bool {
	if clientReq.expr == nil {
		return true
	}

	filters := clientReq.expr.Args()
	txFilters := tx.Filters(filters)

	// should be done after tx.Filters() to avoid nil pointer dereference
	txType := tx.BlockchainTransaction.(*types.EthTransaction).Type()

	if !isFiltersSupportedByTxType(txType, filters) {
		return false
	}

	// Evaluate if we should send the tx
	shouldSend, err := conditions.Evaluate(clientReq.expr, txFilters)
	if err != nil {
		log.Errorf("error evaluate Filters. feed: %v. filters: %s. remote address: %v. account id: %v error - %v tx: %v",
			clientReq.feed, clientReq.expr, remoteAddress, accountID, err.Error(), txFilters)
		return false
	}

	return shouldSend
}

func includeTx(clientReq *clientReq, tx *types.NewTransactionNotification) *TxResult {
	hasTxContent := false
	var response TxResult
	for _, param := range clientReq.includes {
		switch param {
		case "tx_hash":
			txHash := tx.GetHash()
			response.TxHash = &txHash
		case "time":
			timeNow := time.Now().Format(bxgateway.MicroSecTimeFormat)
			response.Time = &timeNow
		case "local_region":
			localRegion := tx.LocalRegion()
			response.LocalRegion = &localRegion
		case "raw_tx":
			rawTx := hexutil.Encode(tx.RawTx())
			response.RawTx = &rawTx
		default:
			if !hasTxContent && strings.HasPrefix(param, "tx_contents.") {
				hasTxContent = true
			}
		}
	}

	if hasTxContent {
		fields := tx.Fields(clientReq.includes)
		if fields == nil {
			log.Errorf("Got nil from tx.Fields - need to be checked")
			return nil
		}
		response.TxContents = fields
	}

	return &response
}

func filterAndIncludeTx(clientReq *clientReq, tx *types.NewTransactionNotification, remoteAddress string, accountID types.AccountID) *TxResult {
	if !shouldSendTx(clientReq, tx, remoteAddress, accountID) {
		return nil
	}

	return includeTx(clientReq, tx)
}
