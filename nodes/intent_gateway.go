package nodes

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/servers"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/sourcegraph/jsonrpc2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

//go:generate mockgen -destination ../test/mock/gw_intents_manager_mock.go -package mock . IntentsManager

// ErrInvalidSignature reports about invalid signature in request
var ErrInvalidSignature = errors.New("invalid signature")

//go:generate mockgen -destination ../test/mock/mock_grpc_feed_manager.go -package mock . GRPCFeedManager

// GRPCFeedManager declares the interface of the feed manager for grpc handler
type GRPCFeedManager interface {
	Subscribe(feedName types.FeedType, feedConnectionType types.FeedConnectionType, conn *jsonrpc2.Conn, ci types.ClientInfo, ro types.ReqOptions, ethSubscribe bool) (*servers.ClientSubscriptionHandlingInfo, error)
	Unsubscribe(subscriptionID string, closeClientConnection bool, errMsg string) error
	GetSyncedWSProvider(preferredProviderEndpoint *types.NodeEndpoint) (blockchain.WSProvider, bool)
}

// CreateIntent stores the intent in the cache
func (g *GatewayGrpc) CreateIntent(req *pb.SubmitIntentRequest) (*types.UserIntent, error) {
	intentID := utils.GenerateUUID()
	intent := types.UserIntent{
		ID:            intentID,
		DappAddress:   req.DappAddress,
		SenderAddress: req.SenderAddress,
		Intent:        req.Intent,
		Hash:          req.Hash,
		Signature:     req.Signature,
		Timestamp:     time.Now(),
	}

	// todo: need check if the intent is already in the cache to avoid spamming
	// todo: do not store intents in cache for now
	// g.IntentsStore.Cache.Store(intentID, intent)
	return &intent, nil
}

// CreateIntentSolution gets the intent from the cache by req.IntentId
func (g *GatewayGrpc) CreateIntentSolution(_ context.Context, req *pb.SubmitIntentSolutionRequest) (*types.UserIntentSolution, error) {
	intentSolutionID := utils.GenerateUUID()
	intentSolution := &types.UserIntentSolution{
		ID:            intentSolutionID,
		SolverAddress: req.SolverAddress,
		IntentID:      req.IntentId,
		Solution:      req.IntentSolution,
		Hash:          req.Hash,
		Signature:     req.Signature,
		Timestamp:     time.Now(),
	}

	return intentSolution, nil
}

// SubmitIntent submit intent
func (g *GatewayGrpc) SubmitIntent(ctx context.Context, req *pb.SubmitIntentRequest) (*pb.SubmitIntentReply, error) {
	authHeader := retrieveAuthHeader(ctx, "")
	_, err := g.validateIntentAuthHeader(authHeader, true, true, servers.GetPeerAddr(ctx))
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	g.log.Infof("received SubmitIntent request, dAppAddress: %s, senderAddress: %s", req.DappAddress, req.SenderAddress)

	if !common.IsHexAddress(req.DappAddress) {
		return nil, status.Errorf(codes.InvalidArgument, "DappAddress is invalid")
	}

	if (req.DappAddress != req.SenderAddress) && !common.IsHexAddress(req.SenderAddress) {
		return nil, status.Errorf(codes.InvalidArgument, "SenderAddress is invalid")
	}

	if len(req.Intent) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Intent is required")
	}

	if len(req.Hash) != bxmessage.Keccak256HashLen {
		return nil, status.Errorf(codes.InvalidArgument, "Hash is invalid")
	}

	if len(req.Signature) != bxmessage.ECDSASignatureLen {
		return nil, status.Errorf(codes.InvalidArgument, "Signature is invalid")
	}

	ok, err := ValidateSignature(req.SenderAddress, req.Hash, req.Signature)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, ErrInvalidSignature
	}

	intent, err := g.CreateIntent(req)
	if err != nil {
		return nil, err
	}

	// send intent to connected Relays
	intentMsg := bxmessage.NewIntent(intent.ID, intent.DappAddress, intent.SenderAddress, intent.Hash, intent.Signature, intent.Timestamp, intent.Intent)
	g.broadcast(intentMsg, nil, utils.Relay)

	// send intent notification into FeedManager for propagation to subscribers if any
	g.sendIntentNotification(intent)

	return &pb.SubmitIntentReply{IntentId: intent.ID}, nil
}

// SubmitIntentSolution submit intent solution
func (g *GatewayGrpc) SubmitIntentSolution(ctx context.Context, req *pb.SubmitIntentSolutionRequest) (*pb.SubmitIntentSolutionReply, error) {
	authHeader := retrieveAuthHeader(ctx, "")
	_, err := g.validateIntentAuthHeader(authHeader, true, true, servers.GetPeerAddr(ctx))
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	g.log.Infof("received SubmitIntentSolution request, solverAddress: %s", req.SolverAddress)

	if !common.IsHexAddress(req.SolverAddress) {
		return nil, status.Errorf(codes.InvalidArgument, "SolverAddress is invalid")
	}

	if len(req.IntentSolution) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "IntentSolution is required")
	}

	if len(req.Hash) != bxmessage.Keccak256HashLen {
		return nil, status.Errorf(codes.InvalidArgument, "Hash is invalid")
	}

	if len(req.Signature) != bxmessage.ECDSASignatureLen {
		return nil, status.Errorf(codes.InvalidArgument, "Signature is invalid")
	}

	ok, err := ValidateSignature(req.SolverAddress, req.Hash, req.Signature)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, ErrInvalidSignature
	}

	intentSolution, err := g.CreateIntentSolution(ctx, req)
	if err != nil {
		return nil, err
	}

	// send solution to BDN
	intentMsg := bxmessage.NewIntentSolution(intentSolution.ID,
		intentSolution.SolverAddress,
		intentSolution.IntentID,
		intentSolution.Hash,
		intentSolution.Signature,
		intentSolution.Timestamp,
		intentSolution.Solution)
	g.broadcast(intentMsg, nil, utils.Relay)

	// send solution notification into FeedManager for propagation to subscribers
	g.sendSolutionNotification(intentSolution)

	return &pb.SubmitIntentSolutionReply{SolutionId: intentSolution.ID, FirstSeen: timestamppb.New(intentSolution.Timestamp)}, nil
}

// Intents intents
func (g *GatewayGrpc) Intents(req *pb.IntentsRequest, stream pb.Gateway_IntentsServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), "")
	accountModel, err := g.validateIntentAuthHeader(authHeader, true, true, servers.GetPeerAddr(stream.Context()))
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	ok, err := ValidateSignature(req.SolverAddress, req.Hash, req.Signature)
	if err != nil {
		return err
	}

	if !ok {
		return status.Errorf(codes.InvalidArgument, ErrInvalidSignature.Error())
	}

	if g.params.intentsManager.IntentsSubscriptionExists(req.SolverAddress) {
		return status.Errorf(codes.AlreadyExists, "intents subscription for solver address %s already exists", req.SolverAddress)
	}

	g.params.intentsManager.AddIntentsSubscription(req)
	// send intentsSubscription to Relay
	sub := bxmessage.NewIntentsSubscription(req.SolverAddress, req.Hash, req.Signature)
	g.broadcast(sub, nil, utils.Relay)

	defer func() {
		g.params.intentsManager.RmIntentsSubscription(req.SolverAddress)
		// send intentsUnsubscription to Relay
		unsub := bxmessage.NewIntentsUnsubscription(req.SolverAddress)
		g.broadcast(unsub, nil, utils.Relay)
		g.log.Debugf("unsubscribed from intents feed for solverAddress: %s, sent IntentsUnsubscribe msg", req.SolverAddress)
	}()

	return g.handleIntents(req, stream, types.UserIntentsFeed, *accountModel)
}

// IntentSolutions intent solutions
func (g *GatewayGrpc) IntentSolutions(req *pb.IntentSolutionsRequest, stream pb.Gateway_IntentSolutionsServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), "")
	accountModel, err := g.validateIntentAuthHeader(authHeader, true, true, servers.GetPeerAddr(stream.Context()))
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	ok, err := ValidateSignature(req.DappAddress, req.Hash, req.Signature)
	if err != nil {
		return err
	}

	if !ok {
		return status.Errorf(codes.InvalidArgument, ErrInvalidSignature.Error())
	}

	if g.params.intentsManager.SolutionsSubscriptionExists(req.DappAddress) {
		return status.Errorf(codes.AlreadyExists, "solutions subscription for dApp address %s already exists", req.DappAddress)
	}

	g.params.intentsManager.AddSolutionsSubscription(req)
	// send solutionsSubscription to Relay
	sub := bxmessage.NewSolutionsSubscription(req.DappAddress, req.Hash, req.Signature)
	g.broadcast(sub, nil, utils.Relay)

	defer func() {
		g.params.intentsManager.RmSolutionsSubscription(req.DappAddress)
		// send solutionsUnsubscription to Relay
		unsub := bxmessage.NewSolutionsUnsubscription(req.DappAddress)
		g.broadcast(unsub, nil, utils.Relay)
	}()

	return g.handleSolutions(req, stream, types.UserIntentSolutionsFeed, *accountModel)
}

func (g *GatewayGrpc) handleSolutions(_ *pb.IntentSolutionsRequest, stream pb.Gateway_IntentSolutionsServer, feedType types.FeedType, account sdnmessage.Account) error {
	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: servers.GetPeerAddr(stream.Context()),
	}

	sub, err := g.params.grpcFeedManager.Subscribe(feedType, types.GRPCFeed, nil, ci, types.ReqOptions{}, false)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "failed to subscribe to gRPC %v Feed", feedType)
	}

	defer func() {
		err = g.params.grpcFeedManager.Unsubscribe(sub.SubscriptionID, false, "")
		if err != nil {
			log.Errorf("failed to unsubscribe from gRPC %v Feed", feedType)
		}
	}()

	for {
		select {
		case notification := <-sub.FeedChan:
			solutionNotification := (notification).(*types.UserIntentSolutionNotification)
			intentSolution := solutionNotification.UserIntentSolution

			err = stream.Send(&pb.IntentSolutionsReply{
				IntentId:       intentSolution.IntentID,
				IntentSolution: intentSolution.Solution,
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

func (g *GatewayGrpc) handleIntents(_ *pb.IntentsRequest, stream pb.Gateway_IntentsServer, feedType types.FeedType, account sdnmessage.Account) error {
	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: servers.GetPeerAddr(stream.Context()),
	}

	sub, err := g.params.grpcFeedManager.Subscribe(feedType, types.GRPCFeed, nil, ci, types.ReqOptions{}, false)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "failed to subscribe to gRPC %v Feed", feedType)
	}

	defer func() {
		err = g.params.grpcFeedManager.Unsubscribe(sub.SubscriptionID, false, "")
		if err != nil {
			log.Errorf("failed to unsubscribe from gRPC %v Feed", feedType)
		}
	}()

	for {
		select {
		case notification := <-sub.FeedChan:
			intentNotification := (notification).(*types.UserIntentNotification)
			intent := intentNotification.UserIntent
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

func (g *GatewayGrpc) sendIntentNotification(intent *types.UserIntent) {
	intentNotification := types.NewUserIntentNotification(intent)
	g.notify(intentNotification)
}

func (g *GatewayGrpc) sendSolutionNotification(solution *types.UserIntentSolution) {
	solutionNotification := types.NewUserIntentSolutionNotification(solution)
	g.notify(solutionNotification)
}

func (g *GatewayGrpc) validateIntentAuthHeader(authHeader string, required, allowAccessToInternalGateway bool, ip string) (*sdnmessage.Account, error) {
	accountID, secretHash, err := g.accountIDAndHashFromAuthHeader(authHeader, required)
	if err != nil {
		return nil, err
	}

	accountModel, err := g.params.authorize(accountID, secretHash, allowAccessToInternalGateway, g.params.allowIntroductoryTierAccess, ip)
	if err != nil {
		return nil, err
	}

	return &accountModel, nil
}

// ValidateSignature validates the signature
func ValidateSignature(signingAddress string, hash []byte, signature []byte) (bool, error) {
	addressBytes := common.HexToAddress(signingAddress)

	pubKey, err := crypto.SigToPub(hash, signature)
	if err != nil {
		return false, err
	}

	recoveredAddress := crypto.PubkeyToAddress(*pubKey)

	return bytes.Equal(recoveredAddress.Bytes(), addressBytes.Bytes()), nil
}

type subscription struct {
	addr      string
	hash      []byte
	signature []byte
}

// IntentsManager interface for mocking
type IntentsManager interface {
	SubscriptionMessages() []bxmessage.Message
	AddIntentsSubscription(req *pb.IntentsRequest)
	RmIntentsSubscription(solverAddr string)
	IntentsSubscriptionExists(solverAddr string) bool
	AddSolutionsSubscription(req *pb.IntentSolutionsRequest)
	RmSolutionsSubscription(dAppAddr string)
	SolutionsSubscriptionExists(dAppAddr string) bool
}

func newIntentsManager() *intentsManager {
	return &intentsManager{
		intentsSubscriptions:   make(map[string]*subscription),
		isMx:                   new(sync.RWMutex),
		solutionsSubscriptions: make(map[string]*subscription),
		ssMx:                   new(sync.RWMutex),
	}
}

type intentsManager struct {
	intentsSubscriptions   map[string]*subscription
	isMx                   *sync.RWMutex
	solutionsSubscriptions map[string]*subscription
	ssMx                   *sync.RWMutex
}

func (i *intentsManager) SubscriptionMessages() []bxmessage.Message {
	var m = make([]bxmessage.Message, 0)

	i.isMx.RLock()
	for _, v := range i.intentsSubscriptions {
		m = append(m, bxmessage.NewIntentsSubscription(v.addr, v.hash, v.signature))
	}
	i.isMx.RUnlock()

	i.ssMx.RLock()
	for _, v := range i.solutionsSubscriptions {
		m = append(m, bxmessage.NewSolutionsSubscription(v.addr, v.hash, v.signature))
	}
	i.ssMx.RUnlock()

	return m
}

func (i *intentsManager) AddIntentsSubscription(req *pb.IntentsRequest) {
	i.isMx.Lock()
	defer i.isMx.Unlock()
	i.intentsSubscriptions[req.SolverAddress] = &subscription{
		addr:      req.SolverAddress,
		hash:      req.Hash,
		signature: req.Signature,
	}
}

func (i *intentsManager) RmIntentsSubscription(solverAddr string) {
	i.isMx.Lock()
	defer i.isMx.Unlock()
	delete(i.intentsSubscriptions, solverAddr)
}

func (i *intentsManager) IntentsSubscriptionExists(solverAddr string) bool {
	i.isMx.RLock()
	defer i.isMx.RUnlock()
	_, ok := i.intentsSubscriptions[solverAddr]
	return ok
}

func (i *intentsManager) AddSolutionsSubscription(req *pb.IntentSolutionsRequest) {
	i.ssMx.Lock()
	defer i.ssMx.Unlock()
	i.solutionsSubscriptions[req.DappAddress] = &subscription{
		addr:      req.DappAddress,
		hash:      req.Hash,
		signature: req.Signature,
	}
}

func (i *intentsManager) RmSolutionsSubscription(dAppAddr string) {
	i.ssMx.Lock()
	defer i.ssMx.Unlock()
	delete(i.solutionsSubscriptions, dAppAddr)
}

func (i *intentsManager) SolutionsSubscriptionExists(dAppAddr string) bool {
	i.ssMx.Lock()
	defer i.ssMx.Unlock()
	_, ok := i.solutionsSubscriptions[dAppAddr]
	return ok
}
