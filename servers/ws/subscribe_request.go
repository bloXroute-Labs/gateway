package ws

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sourcegraph/jsonrpc2"
	"github.com/zhouzhuojie/conditions"

	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler/filter"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler/validator"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

var (
	availableFeeds = []types.FeedType{types.NewTxsFeed, types.NewBlocksFeed, types.BDNBlocksFeed, types.PendingTxsFeed,
		types.OnBlockFeed, types.TxReceiptsFeed, types.NewBeaconBlocksFeed, types.BDNBeaconBlocksFeed}

	availableFeedsMap = make(map[types.FeedType]struct{})
)

func init() {
	for _, feed := range availableFeeds {
		availableFeedsMap[feed] = struct{}{}
	}
}

func (h *handlerObj) createClientReq(req *jsonrpc2.Request, feed types.FeedType, rpcParams json.RawMessage) (*ClientReq, error) {
	request := subscriptionRequest{
		feed: feed,
	}

	err := json.Unmarshal(rpcParams, &request.options)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal options: %w", err)
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

	var expr conditions.Expr
	if request.options.Filters != "" {
		expr, err = filter.ValidateFilters(request.options.Filters, h.txFromFieldIncludable)
		if err != nil {
			h.log.Debugf("error when creating filters. request id: %v. method: %v. params: %s. remote address: %v account id: %v error - %v",
				req.ID, req.Method, *req.Params, h.remoteAddress, h.connectionAccount.AccountID, err.Error())
			return nil, fmt.Errorf("error creating Filters: %w", err)
		}
	}

	// check if valid Feed
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

	calls := make(map[string]*handler.RPCCall)
	if request.feed == types.OnBlockFeed {
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

	return &ClientReq{
		Includes: request.options.Include,
		Feed:     request.feed,
		Expr:     expr,
		calls:    &calls,
		MultiTxs: request.options.MultiTxs,
	}, nil
}

func (h *handlerObj) parseSubscriptionRequest(req *jsonrpc2.Request) (types.FeedType, json.RawMessage, error) {
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

	var feed types.FeedType
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
		(feed == types.OnBlockFeed || feed == types.TxReceiptsFeed) {
		err = fmt.Errorf("%v Feed is not available via cloud services. %v Feed is only supported on gateways", feed, feed)
		h.log.Errorf("%v. caller account ID: %v, node account ID: %v", err, h.connectionAccount.AccountID, h.serverAccountID)
		return "", nil, err
	}

	return feed, rpcParams[1], nil
}

func (h *handlerObj) validateFeed(feedName types.FeedType, feedStreaming sdnmessage.BDNFeedService, includes, filters []string) error {
	expireDateTime, err := time.Parse(bxgateway.TimeDateLayoutISO, feedStreaming.ExpireDate)
	if err != nil {
		return fmt.Errorf("failed to parse expire date %v: %w", feedStreaming.ExpireDate, err)
	}
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
