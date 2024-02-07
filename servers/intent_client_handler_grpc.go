package servers

import (
	"context"
	"time"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Intents handler for stream of pending user intents
func (g *GrpcHandler) Intents(req *pb.IntentsRequest, stream pb.Gateway_IntentsServer, account sdnmessage.Account) error {
	return g.handleIntents(req, stream, types.UserIntentsFeed, account)
}

// IntentSolutions handler for stream of pending intent solutions
func (g *GrpcHandler) IntentSolutions(req *pb.IntentSolutionsRequest, stream pb.Gateway_IntentSolutionsServer, account sdnmessage.Account) error {
	return g.handleSolutions(req, stream, types.UserIntentSolutionsFeed, account)
}

// SubmitIntent stores the intent in the cache
func (g *GrpcHandler) SubmitIntent(req *pb.SubmitIntentRequest) (*types.UserIntent, error) {
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

// SubmitIntentSolution gets the intent from the cache by req.IntentId
func (g *GrpcHandler) SubmitIntentSolution(_ context.Context, req *pb.SubmitIntentSolutionRequest) (*types.UserIntentSolution, error) {
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

func (g *GrpcHandler) handleIntents(_ *pb.IntentsRequest, stream pb.Gateway_IntentsServer, feedType types.FeedType, account sdnmessage.Account) error {
	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: GetPeerAddr(stream.Context()),
	}

	sub, err := g.feedManager.Subscribe(feedType, types.GRPCFeed, nil, ci, types.ReqOptions{}, false)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "failed to subscribe to gRPC %v feed", feedType)
	}

	defer func() {
		err = g.feedManager.Unsubscribe(sub.SubscriptionID, false, "")
		if err != nil {
			log.Errorf("failed to unsubscribe from gRPC %v feed", feedType)
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

func (g *GrpcHandler) handleSolutions(_ *pb.IntentSolutionsRequest, stream pb.Gateway_IntentSolutionsServer, feedType types.FeedType, account sdnmessage.Account) error {
	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: GetPeerAddr(stream.Context()),
	}

	sub, err := g.feedManager.Subscribe(feedType, types.GRPCFeed, nil, ci, types.ReqOptions{}, false)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "failed to subscribe to gRPC %v feed", feedType)
	}

	defer func() {
		err = g.feedManager.Unsubscribe(sub.SubscriptionID, false, "")
		if err != nil {
			log.Errorf("failed to unsubscribe from gRPC %v feed", feedType)
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
