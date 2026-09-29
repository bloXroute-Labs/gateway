package ws

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/v2/sdnsdk/message"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler/filter"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler/validator"
)

var (
	availableFeeds = []bxtypes.FeedType{
		bxtypes.NewTxsFeed, bxtypes.NewBlocksFeed, bxtypes.BDNBlocksFeed, bxtypes.PendingTxsFeed,
		bxtypes.OnBlockFeed, bxtypes.TxReceiptsFeed, bxtypes.NewBeaconBlocksFeed, bxtypes.BDNBeaconBlocksFeed,
	}

	availableFeedsMap = make(map[bxtypes.FeedType]struct{})
)

func init() {
	for _, feed := range availableFeeds {
		availableFeedsMap[feed] = struct{}{}
	}
}

func (h *handlerObj) createClientReq(req Request, feed bxtypes.FeedType, rpcParams json.RawMessage) (*ClientReq, error) {
	request := subscriptionRequest{
		feed: feed,
	}

	err := json.Unmarshal(rpcParams, &request.options)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal options: %w", err)
	}

	// Set default value for ParsedTxs if not provided
	if request.options.ParsedTxs == nil {
		request.options.ParsedTxs = new(true)
	}
	if request.options.Include == nil {
		h.log.Debugf("invalid param from request id: %v. method: %v. params: %s. remote address: %v account id: %v.",
			req.ID, req.Method, *req.Params, h.remoteAddress, h.connectionAccount.AccountID)
		return nil, fmt.Errorf("got unsupported params: %v", string(rpcParams))
	}

	requestedFields, err := validator.ValidateIncludeParam(request.feed, request.options.Include, h.txFromFieldIncludable)
	if err != nil {
		return nil, err
	}

	request.options.Include = requestedFields

	var expr *filter.Expression
	if request.options.Filters != "" {
		expr, err = filter.NewDefaultExpression(request.options.Filters, h.txFromFieldIncludable)
		if err != nil {
			h.log.Debugf("error when creating filters. request id: %v. method: %v. params: %s. remote address: %v account id: %v error - %v",
				req.ID, req.Method, *req.Params, h.remoteAddress, h.connectionAccount.AccountID, err.Error())
			return nil, fmt.Errorf("error creating Filters: %w", err)
		}
	}

	// HACK: feed validation should not execute on ESE GWs to avoid feed expiration validation
	// See https://bloxroute.atlassian.net/browse/BP-3340
	if h.connectionAccount.AccountID != h.serverAccountID {
		feedStreaming := sdnmessage.BDNQuotaService{}

		switch request.feed {
		case bxtypes.NewTxsFeed, bxtypes.PendingTxsFeed:
			switch h.networkNum {
			case bxtypes.MainnetNum:
				feedStreaming = h.connectionAccount.EthMempoolStreaming
			case bxtypes.BSCMainnetNum:
				feedStreaming = h.connectionAccount.BscMempoolStreaming
			}
		case bxtypes.BDNBlocksFeed, bxtypes.NewBlocksFeed, bxtypes.NewBeaconBlocksFeed, bxtypes.BDNBeaconBlocksFeed, bxtypes.OnBlockFeed:
			switch h.networkNum {
			case bxtypes.MainnetNum:
				feedStreaming = h.connectionAccount.EthBlocksStreaming
			case bxtypes.BSCMainnetNum:
				feedStreaming = h.connectionAccount.BscBlocksStreaming
			}
		case bxtypes.TxReceiptsFeed:
			switch h.networkNum {
			case bxtypes.MainnetNum:
				feedStreaming = h.connectionAccount.EthTxReceiptsStreaming
			case bxtypes.BSCMainnetNum:
				feedStreaming = h.connectionAccount.BscTxReceiptsStreaming
			}
		}

		if !feedStreaming.IsActive() {
			return nil, fmt.Errorf("%v is not allowed", request.feed)
		}
	}

	calls := make(map[string]*handler.RPCCall)
	if request.feed == bxtypes.OnBlockFeed {
		for idx, callParams := range request.options.CallParams {
			if callParams == nil {
				return nil, errors.New("call-params cannot be nil")
			}
			err = handler.FillCalls(h.nodeWSManager, calls, idx, callParams)
			if err != nil {
				return nil, err
			}
		}
	}

	// pre-normalize "transactions" include for block feeds once at subscription time,
	// so sendNotification pays zero cost per notification (mirrors gRPC handleBlocks).
	includes := request.options.Include
	switch request.feed {
	case bxtypes.NewBlocksFeed, bxtypes.BDNBlocksFeed, bxtypes.NewBeaconBlocksFeed, bxtypes.BDNBeaconBlocksFeed:
		if i := slices.Index(includes, "transactions"); i >= 0 {
			if !*request.options.ParsedTxs {
				includes = slices.Replace(includes, i, i+1, "raw_transactions")
			} else {
				includes = slices.Replace(includes, i, i+1, "transactions_without_sender")
			}
		}
	}

	return &ClientReq{
		Includes:  includes,
		Feed:      request.feed,
		Expr:      expr,
		calls:     &calls,
		MultiTxs:  request.options.MultiTxs,
		ParsedTxs: *request.options.ParsedTxs,
	}, nil
}

func (h *handlerObj) parseSubscriptionRequest(req Request) (bxtypes.FeedType, json.RawMessage, error) {
	if req.Params == nil {
		return "", nil, errors.New(errParamsValueIsMissing)
	}

	var rpcParams []json.RawMessage
	err := json.Unmarshal(*req.Params, &rpcParams)
	if err != nil {
		return "", nil, fmt.Errorf("failed to unmarshal params: %w", err)
	}
	if len(rpcParams) < 2 {
		h.log.Debugf("invalid param from request id: %v. method: %v. params: %s. remote address: %v account id: %v.",
			req.ID, req.Method, *req.Params, h.remoteAddress, h.connectionAccount.AccountID)
		return "", nil, fmt.Errorf("received invalid number of params: expected 2, got %d, params %s", len(rpcParams), string(*req.Params))
	}

	var feed bxtypes.FeedType
	err = json.Unmarshal(rpcParams[0], &feed)
	if err != nil {
		return "", nil, fmt.Errorf("failed to unmarshal Feed name: %w", err)
	}

	if _, ok := availableFeedsMap[feed]; !ok {
		h.log.Debugf("invalid request Feed param from request id: %v, method: %v, params: %s. remote address: %v account id: %v.",
			req.ID, req.Method, *req.Params, h.remoteAddress, h.connectionAccount.AccountID)
		return "", nil, fmt.Errorf("got unsupported Feed name %v, possible feeds are: %v", feed, availableFeeds)
	}

	if h.connectionAccount.AccountID != h.serverAccountID &&
		(feed == bxtypes.OnBlockFeed || feed == bxtypes.TxReceiptsFeed) {
		err = fmt.Errorf("%v Feed is not available via cloud services. %v Feed is only supported on gateways", feed, feed)
		h.log.Errorf("%v. caller account ID: %v, node account ID: %v", err, h.connectionAccount.AccountID, h.serverAccountID)
		return "", nil, err
	}

	return feed, rpcParams[1], nil
}
