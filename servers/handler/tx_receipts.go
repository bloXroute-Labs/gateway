package handler

import (
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// HandleTxReceipts - fetches transaction receipts for transactions in block and sends them to the client
func HandleTxReceipts(wsManager blockchain.WSManager, block *types.EthBlockNotification) ([]*types.TxReceipt, error) {
	nodeWS, ok := wsManager.GetSyncedWSProvider(block.Source())
	if !ok {
		return nil, fmt.Errorf("node ws connection is not available")
	}

	var result []*types.TxReceipt
	var mu sync.Mutex
	g := new(errgroup.Group)
	rpcOptions := blockchain.RPCOptions{RetryAttempts: bxgateway.MaxEthTxReceiptCallRetries, RetryInterval: bxgateway.EthTxReceiptCallRetrySleepInterval}
	txsCountHex := fmt.Sprintf("0x%x", len(block.GetTransactions()))
	txs := block.GetTransactions()

	for _, t := range txs {
		tx := t
		g.Go(func() error {
			hash := tx["hash"]
			responseTxReceipt, err := nodeWS.FetchTransactionReceipt([]interface{}{hash}, rpcOptions)
			if err != nil || responseTxReceipt == nil {
				log.Debugf("failed to fetch transaction receipt for %v in block %v: %v", hash, block.BlockHash, err)
				return err
			}

			receipt := types.NewTxReceipt(responseTxReceipt.(map[string]interface{}), txsCountHex)

			mu.Lock()
			result = append(result, receipt)
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	log.Debugf("finished fetching transaction receipts for block %v, %v", block.BlockHash, block.Header.Number)
	return result, nil
}
