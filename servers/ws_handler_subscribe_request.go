package servers

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/sourcegraph/jsonrpc2"
	"github.com/zhouzhuojie/conditions"
)

var (
	availableFeeds = []types.FeedType{types.NewTxsFeed, types.NewBlocksFeed, types.BDNBlocksFeed, types.PendingTxsFeed,
		types.OnBlockFeed, types.TxReceiptsFeed, types.NewBeaconBlocksFeed, types.BDNBeaconBlocksFeed}

	txContentFields = []string{"tx_contents.nonce", "tx_contents.tx_hash",
		"tx_contents.gas_price", "tx_contents.gas", "tx_contents.to", "tx_contents.value", "tx_contents.input",
		"tx_contents.v", "tx_contents.r", "tx_contents.s", "tx_contents.type", "tx_contents.access_list",
		"tx_contents.chain_id", "tx_contents.max_priority_fee_per_gas", "tx_contents.max_fee_per_gas", "tx_contents.max_fee_per_blob_gas", "tx_contents.blob_versioned_hashes", "tx_contents.y_parity"}

	defaultTxParams = append(txContentFields, "tx_hash", "local_region", "time")

	txContentFieldsWithFrom = append(txContentFields, "tx_contents.from")

	validTxParams        = append(txContentFields, "tx_contents", "tx_contents.from", "tx_hash", "local_region", "time", "raw_tx")
	validBlockParams     = append(txContentFields, "tx_contents.from", "hash", "header", "transactions", "uncles", "future_validator_info", "withdrawals")
	validTxReceiptParams = []string{"block_hash", "block_number", "contract_address",
		"cumulative_gas_used", "effective_gas_price", "from", "gas_used", "logs", "logs_bloom",
		"status", "to", "transaction_hash", "transaction_index", "type", "txs_count", "blob_gas_used", "blob_gas_price"}
	validOnBlockParams     = []string{"name", "response", "block_height", "tag"}
	validBeaconBlockParams = []string{"hash", "header", "slot", "body"}

	availableFeedsMap = make(map[types.FeedType]struct{})
	validParamsMap    = make(map[types.FeedType]map[string]struct{})
)

func init() {
	// populate the maps
	for _, feed := range availableFeeds {
		availableFeedsMap[feed] = struct{}{}
	}

	validParamsMap = map[types.FeedType]map[string]struct{}{
		types.NewTxsFeed:          stringSliceToSet(validTxParams),
		types.PendingTxsFeed:      stringSliceToSet(validTxParams),
		types.BDNBlocksFeed:       stringSliceToSet(validBlockParams),
		types.NewBlocksFeed:       stringSliceToSet(validBlockParams),
		types.OnBlockFeed:         stringSliceToSet(validOnBlockParams),
		types.TxReceiptsFeed:      stringSliceToSet(validTxReceiptParams),
		types.NewBeaconBlocksFeed: stringSliceToSet(validBeaconBlockParams),
		types.BDNBeaconBlocksFeed: stringSliceToSet(validBeaconBlockParams),
	}
}

func (h *handlerObj) createClientReq(req *jsonrpc2.Request) (*clientReq, error) {
	if req.Params == nil {
		return nil, errors.New(errParamsValueIsMissing)
	}

	var rpcParams []json.RawMessage
	err := json.Unmarshal(*req.Params, &rpcParams)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal params: %w", err)
	}
	if len(rpcParams) < 2 {
		h.log.Debugf("invalid param from request id: %v. method: %v. params: %s. remote address: %v account id: %v.",
			req.ID, req.Method, *req.Params, h.remoteAddress, h.connectionAccount.AccountID)
		return nil, fmt.Errorf("received invalid number of params: expected 2, got %d, params %s", len(rpcParams), string(*req.Params))
	}

	request := subscriptionRequest{}
	err = json.Unmarshal(rpcParams[0], &request.feed)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal feed name: %w", err)
	}
	if _, ok := availableFeedsMap[request.feed]; !ok {
		h.log.Debugf("invalid request feed param from request id: %v, method: %v, params: %s. remote address: %v account id: %v.",
			req.ID, req.Method, *req.Params, h.remoteAddress, h.connectionAccount.AccountID)
		return nil, fmt.Errorf("got unsupported feed name %v, possible feeds are: %v", request.feed, availableFeeds)
	}
	if h.connectionAccount.AccountID != h.FeedManager.accountModel.AccountID &&
		(request.feed == types.OnBlockFeed || request.feed == types.TxReceiptsFeed) {
		err = fmt.Errorf("%v feed is not available via cloud services. %v feed is only supported on gateways", request.feed, request.feed)
		h.log.Errorf("%v. caller account ID: %v, node account ID: %v", err, h.connectionAccount.AccountID, h.FeedManager.accountModel.AccountID)
		return nil, err
	}

	err = json.Unmarshal(rpcParams[1], &request.options)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal options: %w", err)
	}
	if request.options.Include == nil {
		h.log.Debugf("invalid param from request id: %v. method: %v. params: %s. remote address: %v account id: %v.",
			req.ID, req.Method, *req.Params, h.remoteAddress, h.connectionAccount.AccountID)
		return nil, fmt.Errorf("got unsupported params: %v", string(rpcParams[1]))
	}

	requestedFields, err := validateIncludeParam(request.feed, request.options.Include, h.txFromFieldIncludable)
	if err != nil {
		return nil, err
	}

	request.options.Include = requestedFields

	var expr conditions.Expr
	if request.options.Filters != "" {
		expr, err = validateFilters(request.options.Filters, h.txFromFieldIncludable)
		if err != nil {
			h.log.Debugf("error when creating filters. request id: %v. method: %v. params: %s. remote address: %v account id: %v error - %v",
				req.ID, req.Method, *req.Params, h.remoteAddress, h.connectionAccount.AccountID, err.Error())
			return nil, fmt.Errorf("error creating Filters: %w", err)
		}
	}

	// check if valid feed
	var filters []string
	if expr != nil {
		filters = expr.Args()
	}

	feedStreaming := sdnmessage.BDNFeedService{}
	switch request.feed {
	case types.NewTxsFeed:
		feedStreaming = h.connectionAccount.NewTransactionStreaming
	case types.PendingTxsFeed:
		feedStreaming = h.connectionAccount.PendingTransactionStreaming
	case types.BDNBlocksFeed, types.NewBlocksFeed, types.NewBeaconBlocksFeed, types.BDNBeaconBlocksFeed:
		feedStreaming = h.connectionAccount.NewBlockStreaming
	case types.OnBlockFeed:
		feedStreaming = h.connectionAccount.OnBlockFeed
	case types.TxReceiptsFeed:
		feedStreaming = h.connectionAccount.TransactionReceiptFeed
	}

	err = h.validateFeed(request.feed, feedStreaming, request.options.Include, filters)
	if err != nil {
		return nil, err
	}

	calls := make(map[string]*RPCCall)
	if request.feed == types.OnBlockFeed {
		for idx, callParams := range request.options.CallParams {
			if callParams == nil {
				return nil, errors.New("call-params cannot be nil")
			}
			err = fillCalls(h.FeedManager.nodeWSManager, calls, idx, callParams)
			if err != nil {
				return nil, err
			}
		}
	}

	return &clientReq{
		includes: request.options.Include,
		feed:     request.feed,
		expr:     expr,
		calls:    &calls,
		MultiTxs: request.options.MultiTxs,
	}, nil
}

func (h *handlerObj) validateFeed(feedName types.FeedType, feedStreaming sdnmessage.BDNFeedService, includes, filters []string) error {
	expireDateTime, _ := time.Parse(bxgateway.TimeDateLayoutISO, feedStreaming.ExpireDate)
	if time.Now().UTC().After(expireDateTime) {
		return fmt.Errorf("%v is not allowed or date has been expired", feedName)
	}
	if feedStreaming.Feed.AllowFiltering && utils.Exists("all", feedStreaming.Feed.AvailableFields) {
		return nil
	}
	for _, include := range includes {
		if !utils.Exists(include, feedStreaming.Feed.AvailableFields) {
			return fmt.Errorf("including %v field in %v is not allowed", include, feedName)
		}
	}
	if !feedStreaming.Feed.AllowFiltering && len(filters) > 0 {
		return fmt.Errorf("filtering in %v is not allowed", feedName)
	}

	return nil
}

func stringSliceToSet(s []string) map[string]struct{} {
	m := make(map[string]struct{})
	for i := range s {
		m[s[i]] = struct{}{}
	}
	return m
}
