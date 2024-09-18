package ws

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/sourcegraph/jsonrpc2"
	"github.com/zhouzhuojie/conditions"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler/filter"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler/validator"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/utils/intent"
)

var (
	availableFeeds = []types.FeedType{types.NewTxsFeed, types.NewBlocksFeed, types.BDNBlocksFeed, types.PendingTxsFeed,
		types.OnBlockFeed, types.TxReceiptsFeed, types.NewBeaconBlocksFeed, types.BDNBeaconBlocksFeed, types.UserIntentsFeed,
		types.UserIntentSolutionsFeed, types.QuotesFeed}

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

func (h *handlerObj) createQuoteClientReq(rpcParams json.RawMessage) (*ClientReq, func(), error) {
	var params subscribeQuotesParams
	var expr conditions.Expr
	err := json.Unmarshal(rpcParams, &params)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal subscription intent options: %w", err)
	}
	if !common.IsHexAddress(params.DappAddress) {
		return nil, nil, intent.ErrInvalidAddress
	}

	subscriptions := h.intentsManager.AddQuotesSubscription(params.DappAddress)
	sub := bxmessage.NewQuotesSubscription(params.DappAddress)

	includes := []string{params.DappAddress}

	if subscriptions == 1 {
		err = h.node.HandleMsg(sub, nil, connections.RunBackground)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to send subscription message for feed quotes: %w", err)
		}
	}
	postRun := func() {
		subscriptions, err = h.intentsManager.RmQuotesSubscription(params.DappAddress)
		// send quotesUnsubscription to Relay if there are no more subscriptions to a specific address
		if err == nil && subscriptions == 0 {
			unsub := bxmessage.NewQuotesUnsubscription(params.DappAddress)
			err = h.node.HandleMsg(unsub, nil, connections.RunBackground)
			if err != nil {
				h.log.Errorf("failed to handle unsubscription message for dapp address %v and feed quotes: %v", params.DappAddress, err)
			}
		}
	}

	return &ClientReq{Feed: types.QuotesFeed, Includes: includes, Expr: expr}, postRun, nil
}

func (h *handlerObj) createIntentClientReq(req *jsonrpc2.Request, feed types.FeedType, rpcParams json.RawMessage) (*ClientReq, func(), error) {
	var params subscriptionIntentParams
	err := json.Unmarshal(rpcParams, &params)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal subscription intent options: %w", err)
	}

	switch feed {
	case types.UserIntentsFeed:
		err = intent.ValidateHashAndSignature(params.SolverAddress, params.Hash, params.Signature, []byte(params.SolverAddress))
	case types.UserIntentSolutionsFeed:
		err = intent.ValidateHashAndSignature(params.DappAddress, params.Hash, params.Signature, []byte(params.DappAddress))
	default:
		return nil, nil, fmt.Errorf("invalid intentoin feed type %v", feed)
	}
	if err != nil {
		h.log.Debugf("error when validating signature. request id: %v. method: %v. params: %s. remote address: %v account id: %v error - %v",
			req.ID, req.Method, *req.Params, h.remoteAddress, h.connectionAccount.AccountID, err)
		return nil, nil, err
	}

	var sub bxmessage.Message
	var postRun func()
	var includes []string
	var expr conditions.Expr

	switch feed {
	case types.UserIntentsFeed:
		if h.intentsManager.IntentsSubscriptionExists(params.SolverAddress) {
			return nil, nil, fmt.Errorf("intent subscription already exists for solver address %v", params.SolverAddress)
		}
		h.intentsManager.AddIntentsSubscription(params.SolverAddress, params.Hash, params.Signature)
		sub = bxmessage.NewIntentsSubscription(params.SolverAddress, params.Hash, params.Signature)

		if params.Filters != "" {
			expr, err = filter.ValidateIntentsFilters(params.Filters)
			if err != nil {
				h.log.Debugf("error when creating filters. request id: %v. method: %v. params: %s. remote address: %v account id: %v error - %v",
					req.ID, req.Method, *req.Params, h.remoteAddress, h.connectionAccount.AccountID, err.Error())
				return nil, nil, fmt.Errorf("error creating Filters: %w", err)
			}
		}

		postRun = func() {
			h.intentsManager.RmIntentsSubscription(params.SolverAddress)
			unsub := bxmessage.NewIntentsUnsubscription(params.SolverAddress)
			err = h.node.HandleMsg(unsub, nil, connections.RunBackground)
			if err != nil {
				h.log.Errorf("failed to handle unsubscription message for solver address %v and feed %v: %v", params.SolverAddress, feed, err)
			}
		}
	case types.UserIntentSolutionsFeed:
		if h.intentsManager.SolutionsSubscriptionExists(params.DappAddress) {
			return nil, nil, fmt.Errorf("intent solutions subscription already exists for dapp address %v", params.DappAddress)
		}
		h.intentsManager.AddSolutionsSubscription(params.DappAddress, params.Hash, params.Signature)
		sub = bxmessage.NewSolutionsSubscription(params.DappAddress, params.Hash, params.Signature)

		includes = []string{params.DappAddress}

		postRun = func() {
			h.intentsManager.RmSolutionsSubscription(params.DappAddress)
			unsub := bxmessage.NewSolutionsUnsubscription(params.DappAddress)
			err = h.node.HandleMsg(unsub, nil, connections.RunBackground)
			if err != nil {
				h.log.Errorf("failed to handle unsubscription message for dapp address %v and feed %v: %v", params.DappAddress, feed, err)
			}
		}
	default:
		return nil, nil, fmt.Errorf("invalid intentoin feed type %v", feed)
	}

	// send subscription to Relay
	err = h.node.HandleMsg(sub, nil, connections.RunBackground)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to send subscription message for feed %v: %w", feed, err)
	}

	return &ClientReq{Feed: feed, Includes: includes, Expr: expr}, postRun, nil
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
