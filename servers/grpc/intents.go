package grpc

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/zhouzhuojie/conditions"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/bloXroute-Labs/gateway/v2/servers/handler/filter"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/utils/intent"
)

//go:generate mockgen -destination ../test/mock/gw_intents_manager_mock.go -package mock . IntentsManager
//go:generate mockgen -destination ../test/mock/mock_grpc_feed_manager.go -package mock . GRPCFeedManager

func (g *server) SubmitQuote(ctx context.Context, req *pb.SubmitQuoteRequest) (*pb.SubmitQuoteReply, error) {
	authHeader := retrieveAuthHeader(ctx, "")
	_, err := g.validateIntentAuthHeader(authHeader, true, true, getPeerAddr(ctx))
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	g.log.Infof("received SubmitQuote request, dAppAddress: %s, solverAddress: %s", req.DappAddress, req.SolverAddress)

	if !common.IsHexAddress(req.DappAddress) {
		return nil, status.Errorf(codes.InvalidArgument, "DappAddress is invalid")
	}

	if len(req.Quote) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Quote is required")
	}

	err = intent.ValidateHashAndSignature(req.SolverAddress, req.Hash, req.Signature, req.Quote)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	quoteNotification, err := g.convertQuoteRequest(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	quoteMsg := bxmessage.NewQuote(quoteNotification.ID, quoteNotification.DappAddress, quoteNotification.SolverAddress, quoteNotification.Hash, quoteNotification.Signature, quoteNotification.Quote, quoteNotification.Timestamp)
	g.params.connector.Broadcast(quoteMsg, nil, utils.RelayProxy)
	g.notify(quoteNotification)

	g.params.intentsManager.IncQuoteSubmissions()

	return &pb.SubmitQuoteReply{QuoteId: quoteNotification.ID}, nil
}

// SubmitIntent submit intent
func (g *server) SubmitIntent(ctx context.Context, req *pb.SubmitIntentRequest) (*pb.SubmitIntentReply, error) {
	authHeader := retrieveAuthHeader(ctx, "")
	_, err := g.validateIntentAuthHeader(authHeader, true, true, getPeerAddr(ctx))
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	g.log.Infof("received SubmitIntent request, dAppAddress: %s, senderAddress: %s", req.DappAddress, req.SenderAddress)

	if !common.IsHexAddress(req.DappAddress) {
		return nil, status.Errorf(codes.InvalidArgument, "DAppAddress is invalid")
	}

	if (req.DappAddress != req.SenderAddress) && !common.IsHexAddress(req.SenderAddress) {
		return nil, status.Errorf(codes.InvalidArgument, "SenderAddress is invalid")
	}

	if len(req.Intent) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Intent is required")
	}

	err = intent.ValidateHashAndSignature(req.SenderAddress, req.Hash, req.Signature, req.Intent)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	intentNotification, err := g.convertIntentRequest(req)
	if err != nil {
		return nil, err
	}

	// send intent to connected Relays
	intentMsg := bxmessage.NewIntent(intentNotification.ID, intentNotification.DappAddress, intentNotification.SenderAddress, intentNotification.Hash, intentNotification.Signature, intentNotification.Timestamp, intentNotification.Intent)
	g.params.connector.Broadcast(intentMsg, nil, utils.RelayProxy)

	// send intent notification into feedManager for propagation to subscribers if any
	g.sendIntentNotification(intentNotification)

	g.params.intentsManager.IncIntentSubmissions()

	return &pb.SubmitIntentReply{IntentId: intentNotification.ID}, nil
}

// SubmitIntentSolution submit intent solution
func (g *server) SubmitIntentSolution(ctx context.Context, req *pb.SubmitIntentSolutionRequest) (*pb.SubmitIntentSolutionReply, error) {
	authHeader := retrieveAuthHeader(ctx, "")
	_, err := g.validateIntentAuthHeader(authHeader, true, true, getPeerAddr(ctx))
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	g.log.Infof("received SubmitIntentSolution request, solverAddress: %s", req.SolverAddress)

	if len(req.IntentSolution) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "IntentSolution is required")
	}

	if len(req.IntentId) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "IntentId is required")
	}

	err = intent.ValidateHashAndSignature(req.SolverAddress, req.Hash, req.Signature, req.IntentSolution)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	id, err := intent.GenerateSolutionID(req.IntentId, req.IntentSolution)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	firstSeen := time.Now()

	// send solution to BDN
	intentMsg := bxmessage.NewIntentSolution(id, req.SolverAddress, req.IntentId, req.Hash, req.Signature, firstSeen, req.IntentSolution)
	g.params.connector.Broadcast(intentMsg, nil, utils.RelayProxy)

	g.params.intentsManager.IncSolutionSubmissions()

	return &pb.SubmitIntentSolutionReply{SolutionId: id, FirstSeen: timestamppb.New(firstSeen)}, nil
}

func (g *server) Quotes(req *pb.QuotesRequest, stream pb.Gateway_QuotesServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), "")
	accountModel, err := g.validateIntentAuthHeader(authHeader, true, true, getPeerAddr(stream.Context()))
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	if !common.IsHexAddress(req.DappAddress) {
		return intent.ErrInvalidAddress
	}

	subscriptions := g.params.intentsManager.AddQuotesSubscription(req.DappAddress)
	// send quoteSubscription to Relay for the first subscription
	if subscriptions == 1 {
		sub := bxmessage.NewQuotesSubscription(req.DappAddress)
		g.params.connector.Broadcast(sub, nil, utils.RelayProxy)
	}

	defer func() {
		subscriptions, err = g.params.intentsManager.RmQuotesSubscription(req.DappAddress)
		// send quotesUnsubscription to Relay if there are no more subscriptions to a specific address
		if err == nil && subscriptions == 0 {
			unsub := bxmessage.NewQuotesUnsubscription(req.DappAddress)
			g.params.connector.Broadcast(unsub, nil, utils.RelayProxy)
			g.log.Debugf("unsubscribed from quote feed for dappAddress: %s, sent IntentsUnsubscribe msg", req.DappAddress)
		}
	}()
	return g.handleQuotes(req, stream, *accountModel)
}

// Intents intents
func (g *server) Intents(req *pb.IntentsRequest, stream pb.Gateway_IntentsServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), "")
	accountModel, err := g.validateIntentAuthHeader(authHeader, true, true, getPeerAddr(stream.Context()))
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	err = intent.ValidateHashAndSignature(req.SolverAddress, req.Hash, req.Signature, []byte(req.SolverAddress))
	if err != nil {
		return status.Errorf(codes.InvalidArgument, err.Error())
	}

	if g.params.intentsManager.IntentsSubscriptionExists(req.SolverAddress) {
		return status.Errorf(codes.AlreadyExists, "intents subscription for solver address %s already exists", req.SolverAddress)
	}

	var expr conditions.Expr
	if req.GetFilters() != "" {
		expr, err = filter.ValidateIntentsFilters(req.GetFilters())
		if err != nil {
			return status.Error(codes.InvalidArgument, err.Error())
		}
	}

	g.params.intentsManager.AddIntentsSubscription(req.SolverAddress, req.Hash, req.Signature)
	// send intentsSubscription to Relay
	sub := bxmessage.NewIntentsSubscription(req.SolverAddress, req.Hash, req.Signature)
	g.params.connector.Broadcast(sub, nil, utils.RelayProxy)

	defer func() {
		g.params.intentsManager.RmIntentsSubscription(req.SolverAddress)
		// send intentsUnsubscription to Relay
		unsub := bxmessage.NewIntentsUnsubscription(req.SolverAddress)
		g.params.connector.Broadcast(unsub, nil, utils.RelayProxy)
		g.log.Debugf("unsubscribed from intents feed for solverAddress: %s, sent IntentsUnsubscribe msg", req.SolverAddress)
	}()

	return g.handleIntents(expr, stream, types.UserIntentsFeed, *accountModel)
}

// IntentSolutions intent solutions
func (g *server) IntentSolutions(req *pb.IntentSolutionsRequest, stream pb.Gateway_IntentSolutionsServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), "")
	accountModel, err := g.validateIntentAuthHeader(authHeader, true, true, getPeerAddr(stream.Context()))
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	err = intent.ValidateHashAndSignature(req.DappAddress, req.Hash, req.Signature, []byte(req.DappAddress))
	if err != nil {
		return status.Errorf(codes.InvalidArgument, err.Error())
	}

	if g.params.intentsManager.SolutionsSubscriptionExists(req.DappAddress) {
		return status.Errorf(codes.AlreadyExists, "solutions subscription for dApp address %s already exists", req.DappAddress)
	}

	g.params.intentsManager.AddSolutionsSubscription(req.DappAddress, req.Hash, req.Signature)
	// send solutionsSubscription to Relay
	sub := bxmessage.NewSolutionsSubscription(req.DappAddress, req.Hash, req.Signature)
	g.params.connector.Broadcast(sub, nil, utils.RelayProxy)

	defer func() {
		g.params.intentsManager.RmSolutionsSubscription(req.DappAddress)
		// send solutionsUnsubscription to Relay
		unsub := bxmessage.NewSolutionsUnsubscription(req.DappAddress)
		g.params.connector.Broadcast(unsub, nil, utils.RelayProxy)
	}()

	return g.handleSolutions(req, stream, types.UserIntentSolutionsFeed, *accountModel)
}

func (g *server) handleSolutions(req *pb.IntentSolutionsRequest, stream pb.Gateway_IntentSolutionsServer, feedType types.FeedType, account sdnmessage.Account) error {
	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: getPeerAddr(stream.Context()),
	}

	sub, err := g.params.feedManager.Subscribe(feedType, types.GRPCFeed, nil, ci, types.ReqOptions{}, false)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "failed to subscribe to gRPC %v Feed", feedType)
	}

	defer func() {
		err = g.params.feedManager.Unsubscribe(sub.SubscriptionID, false, "")
		if err != nil {
			log.Errorf("failed to unsubscribe from gRPC %v Feed", feedType)
		}
	}()

	for {
		select {
		case notification := <-sub.FeedChan:
			intentSolution := notification.(*types.UserIntentSolutionNotification)

			if intentSolution.DappAddress != req.DappAddress && intentSolution.SenderAddress != req.DappAddress {
				continue
			}

			err = stream.Send(&pb.IntentSolutionsReply{
				IntentId:       intentSolution.IntentID,
				IntentSolution: intentSolution.Solution,
				SolutionId:     intentSolution.ID,
			})
			if err != nil {
				return status.Error(codes.Internal, err.Error())
			}

			log.Tracef("pushed IntentSolution to subscriber: remoteAddress: %s, intentID: %s, solutionID: %s", ci.RemoteAddress, intentSolution.IntentID, intentSolution.ID)
		case <-stream.Context().Done():
			log.Debugf("stream cancelled: remoteAddress: %s", ci.RemoteAddress)
			return nil
		}
	}
}

func (g *server) handleQuotes(req *pb.QuotesRequest, stream pb.Gateway_QuotesServer, account sdnmessage.Account) error {
	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: getPeerAddr(stream.Context()),
	}
	sub, err := g.params.feedManager.Subscribe(types.QuotesFeed, types.GRPCFeed, nil, ci, types.ReqOptions{}, false)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "failed to subscribe to gRPC Quote Feed")
	}

	defer func() {
		err = g.params.feedManager.Unsubscribe(sub.SubscriptionID, false, "")
		if err != nil {
			log.Errorf("failed to unsubscribe from gRPC Quote Feed")
		}
	}()

	for {
		select {
		case notification := <-sub.FeedChan:
			quote := (notification).(*types.QuoteNotification)
			if quote.DappAddress != req.DappAddress {
				continue
			}
			err = stream.Send(&pb.QuotesReply{
				DappAddress:   quote.DappAddress,
				SolverAddress: quote.SolverAddress,
				QuoteId:       quote.ID,
				Quote:         quote.Quote,
				Timestamp:     timestamppb.New(quote.Timestamp),
			})
			if err != nil {
				return status.Error(codes.Internal, err.Error())
			}

			log.Tracef("pushed quote to subscriber: remoteAddress: %s, quoteID: %s", ci.RemoteAddress, quote.ID)
		case <-stream.Context().Done():
			log.Debugf("stream cancelled: remoteAddress: %s", ci.RemoteAddress)
			return nil
		}
	}
}

func (g *server) handleIntents(expr conditions.Expr, stream pb.Gateway_IntentsServer, feedType types.FeedType, account sdnmessage.Account) error {
	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: getPeerAddr(stream.Context()),
	}

	sub, err := g.params.feedManager.Subscribe(feedType, types.GRPCFeed, nil, ci, types.ReqOptions{}, false)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "failed to subscribe to gRPC %v Feed", feedType)
	}

	defer func() {
		err = g.params.feedManager.Unsubscribe(sub.SubscriptionID, false, "")
		if err != nil {
			log.Errorf("failed to unsubscribe from gRPC %v Feed", feedType)
		}
	}()

	for {
		select {
		case notification := <-sub.FeedChan:
			intent := (notification).(*types.UserIntentNotification).UserIntent

			if expr != nil {
				var shouldSend bool
				shouldSend, err = conditions.Evaluate(expr, map[string]interface{}{"dapp_address": intent.DappAddress})
				if err != nil {
					return status.Error(codes.Internal, err.Error())
				}
				if !shouldSend {
					continue
				}
			}

			err = stream.Send(&pb.IntentsReply{
				DappAddress:   intent.DappAddress,
				SenderAddress: intent.SenderAddress,
				IntentId:      intent.ID,
				Intent:        intent.Intent,
				Timestamp:     timestamppb.New(intent.Timestamp),
			})
			if err != nil {
				return status.Error(codes.Internal, err.Error())
			}

			log.Tracef("pushed Intent to subscriber: remoteAddress: %s, intentID: %s", ci.RemoteAddress, intent.ID)
		case <-stream.Context().Done():
			log.Debugf("stream cancelled: remoteAddress: %s", ci.RemoteAddress)
			return nil
		}
	}
}

func (g *server) sendIntentNotification(intent *types.UserIntent) {
	intentNotification := types.NewUserIntentNotification(intent)
	g.notify(intentNotification)
}

func (g *server) validateIntentAuthHeader(authHeader string, required, allowAccessToInternalGateway bool, ip string) (*sdnmessage.Account, error) {
	accountID, secretHash, err := g.accountIDAndHashFromAuthHeader(authHeader, required)
	if err != nil {
		return nil, err
	}

	accountModel, err := g.params.accService.Authorize(accountID, secretHash, allowAccessToInternalGateway, g.params.allowIntroductoryTierAccess, ip)
	if err != nil {
		return nil, err
	}

	return &accountModel, nil
}

// convertIntentRequest creates intent from request
func (g *server) convertIntentRequest(req *pb.SubmitIntentRequest) (*types.UserIntent, error) {
	intentID, err := intent.GenerateIntentID(req.DappAddress, req.Intent)
	if err != nil {
		return nil, err
	}
	return &types.UserIntent{
		ID:            intentID,
		DappAddress:   req.DappAddress,
		SenderAddress: req.SenderAddress,
		Intent:        req.Intent,
		Hash:          req.Hash,
		Signature:     req.Signature,
		Timestamp:     time.Now(),
	}, nil
}

func (g *server) convertQuoteRequest(req *pb.SubmitQuoteRequest) (*types.QuoteNotification, error) {
	quoteID, err := intent.GenerateQuoteID(req.DappAddress, req.Quote)
	if err != nil {
		return nil, err
	}
	return &types.QuoteNotification{
		ID:            quoteID,
		DappAddress:   req.DappAddress,
		SolverAddress: req.SolverAddress,
		Quote:         req.Quote,
		Hash:          req.Hash,
		Signature:     req.Signature,
		Timestamp:     time.Now(),
	}, nil
}
