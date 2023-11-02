package servers

import (
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils/orderedmap"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
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
