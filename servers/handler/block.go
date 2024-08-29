package handler

import (
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// HandleEthOnBlock handle eth on block
func HandleEthOnBlock(wsManager blockchain.WSManager, block *types.EthBlockNotification, calls map[string]*RPCCall, sendNotification func(notification *types.OnBlockNotification) error) error {
	if len(block.Transactions) > 0 {
		nodeWS, ok := wsManager.GetSyncedWSProvider(block.Source())
		if !ok {
			log.Errorf("failed to get synced ws provider")
			return fmt.Errorf("node ws connection is not available")
		}
		blockHeightStr := block.Header.Number
		hashStr := block.BlockHash.String()

		var wg sync.WaitGroup
		for _, c := range calls {
			wg.Add(1)
			go func(call *RPCCall) {
				defer wg.Done()
				if !call.active {
					return
				}
				tag := hexutil.EncodeUint64(block.Header.GetNumber() + uint64(call.blockOffset))
				payload, err := wsManager.ConstructRPCCallPayload(call.commandMethod, call.callPayload, tag)
				if err != nil {
					return
				}
				response, err := nodeWS.CallRPC(call.commandMethod, payload, blockchain.RPCOptions{RetryAttempts: bxgateway.MaxEthOnBlockCallRetries, RetryInterval: bxgateway.EthOnBlockCallRetrySleepInterval})
				if err != nil {
					log.Debugf("disabling failed onBlock call %v: %v", call.callName, err)
					call.active = false
					taskDisabledNotification := types.NewOnBlockNotification(bxgateway.TaskDisabledEvent, call.string(), blockHeightStr, tag, hashStr)
					err = sendNotification(taskDisabledNotification)
					if err != nil {
						log.Errorf("failed to send TaskDisabledNotification for %v", call.callName)
					}
					return
				}
				onBlockNotification := types.NewOnBlockNotification(call.callName, response.(string), blockHeightStr, tag, hashStr)
				err = sendNotification(onBlockNotification)
				if err != nil {
					return
				}
			}(c)
		}
		wg.Wait()
		taskCompletedNotification := types.NewOnBlockNotification(bxgateway.TaskCompletedEvent, "", blockHeightStr, blockHeightStr, hashStr)
		err := sendNotification(taskCompletedNotification)
		if err != nil {
			log.Errorf("failed to send TaskCompletedEvent on block %v", blockHeightStr)
			return nil
		}
	}
	log.Debugf("finished executing onBlock for block %v, %v", block.BlockHash, block.Header.Number)
	return nil
}
