package servers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"golang.org/x/sync/errgroup"
)

// RPCCall represents customer call executed for onBlock feed
type RPCCall struct {
	commandMethod string
	blockOffset   int
	callName      string
	callPayload   map[string]string
	active        bool
}

func newCall(name string) *RPCCall {
	return &RPCCall{
		callName:    name,
		callPayload: make(map[string]string),
		active:      true,
	}
}

func (c *RPCCall) constructCall(callParams map[string]string, nodeWSManager blockchain.WSManager) error {
	for param, value := range callParams {
		switch param {
		case "method":
			isValidMethod := utils.Exists(value, nodeWSManager.ValidRPCCallMethods())
			if !isValidMethod {
				return fmt.Errorf("invalid method %v provided. Supported methods: %v", value, nodeWSManager.ValidRPCCallMethods())
			}
			c.commandMethod = value
		case "tag":
			if value == "latest" {
				c.blockOffset = 0
				break
			}
			blockOffset, err := strconv.Atoi(value)
			if err != nil || blockOffset > 0 {
				return fmt.Errorf("invalid value %v provided for tag. Supported values: latest, 0 or a negative number", value)
			}
			c.blockOffset = blockOffset
		case "name":
			c.callName = value
		default:
			isValidPayloadField := utils.Exists(param, nodeWSManager.ValidRPCCallPayloadFields())
			if !isValidPayloadField {
				return fmt.Errorf("invalid payload field %v provided. Supported fields: %v", param, nodeWSManager.ValidRPCCallPayloadFields())
			}
			c.callPayload[param] = value
		}
	}
	requiredFields, ok := nodeWSManager.RequiredPayloadFieldsForRPCMethod(c.commandMethod)
	if !ok {
		return fmt.Errorf("unexpectedly, unable to find required fields for method %v", c.commandMethod)
	}
	err := c.validatePayload(c.commandMethod, requiredFields)
	if err != nil {
		return err
	}
	return nil
}

func (c *RPCCall) validatePayload(method string, requiredFields []string) error {
	for _, field := range requiredFields {
		_, ok := c.callPayload[field]
		if !ok {
			return fmt.Errorf("expected %v element in request payload for %v", field, method)
		}
	}
	return nil
}

func (c *RPCCall) string() string {
	payloadBytes, err := json.Marshal(c.callPayload)
	if err != nil {
		log.Errorf("failed to convert eth call to string: %v", err)
		return c.callName
	}

	return fmt.Sprintf("%+v", struct {
		commandMethod string
		blockOffset   int
		callName      string
		callPayload   string
		active        bool
	}{
		commandMethod: c.commandMethod,
		blockOffset:   c.blockOffset,
		callName:      c.callName,
		callPayload:   string(payloadBytes),
		active:        c.active,
	})
}

func fillCalls(feedManager *FeedManager, calls map[string]*RPCCall, callIdx int, callParams map[string]string) error {
	call := newCall(strconv.Itoa(callIdx))
	err := call.constructCall(callParams, feedManager.nodeWSManager)
	if err != nil {
		return err
	}
	_, nameExists := calls[call.callName]
	if nameExists {
		return fmt.Errorf("unique name must be provided for each call: call %v already exists", call.callName)
	}
	calls[call.callName] = call
	return err
}

func handleEthOnBlock(feedManager *FeedManager, block *types.EthBlockNotification, calls map[string]*RPCCall, sendNotification func(notification *types.OnBlockNotification) error) error {
	if len(block.Transactions) > 0 {
		nodeWS, ok := feedManager.getSyncedWSProvider(block.Source())
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
				payload, err := feedManager.nodeWSManager.ConstructRPCCallPayload(call.commandMethod, call.callPayload, tag)
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

func handleTxReceipts(feedManager *FeedManager, block *types.EthBlockNotification, sendNotification func(notification *types.TxReceiptNotification) error) error {
	nodeWS, ok := feedManager.getSyncedWSProvider(block.Source())
	if !ok {
		log.Errorf("node ws connection is not available")
		return fmt.Errorf("node ws connection is not available")
	}

	g := new(errgroup.Group)
	for _, t := range block.Transactions {
		tx := t
		g.Go(func() error {
			hash := tx["hash"]
			responseTxReceipt, err := nodeWS.FetchTransactionReceipt([]interface{}{hash}, blockchain.RPCOptions{RetryAttempts: bxgateway.MaxEthTxReceiptCallRetries, RetryInterval: bxgateway.EthTxReceiptCallRetrySleepInterval})
			if err != nil || responseTxReceipt == nil {
				log.Debugf("failed to fetch transaction receipt for %v in block %v: %v", hash, block.BlockHash, err)
				return err
			}
			responseBlock, err := nodeWS.FetchBlock([]interface{}{responseTxReceipt.(map[string]interface{})["blockNumber"], false}, blockchain.RPCOptions{RetryAttempts: bxgateway.MaxEthOnBlockCallRetries, RetryInterval: bxgateway.EthOnBlockCallRetrySleepInterval})
			var txsCount int
			if err == nil && responseBlock != nil {
				transactions, exist := responseBlock.(map[string]interface{})["transactions"]
				txsCount = len(transactions.([]interface{}))
				if !exist {
					return fmt.Errorf("transactions field doesn't exist when query previous epoch block")
				}
			}

			txReceiptNotification := types.NewTxReceiptNotification(responseTxReceipt.(map[string]interface{}), fmt.Sprintf("0x%x", txsCount))
			if err = sendNotification(txReceiptNotification); err != nil {
				log.Errorf("failed to send tx receipt for %v err %v", hash, err)
				return err
			}
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil
	}
	log.Debugf("finished fetching transaction receipts for block %v, %v", block.BlockHash, block.Header.Number)
	return nil
}
