package nodes

import (
	"bytes"
	"context"
	"errors"
	"sync"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/servers"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

//go:generate mockgen -destination ../test/mock/gw_intents_manager_mock.go -package mock . IntentsManager

// ErrInvalidSignature reports about invalid signature in request
var ErrInvalidSignature = errors.New("invalid signature")

func (g *gateway) SubmitIntent(ctx context.Context, req *pb.SubmitIntentRequest) (*pb.SubmitIntentReply, error) {
	authHeader := retrieveAuthHeader(ctx, "")
	_, err := g.validateAuthHeader(authHeader, true, true, servers.GetPeerAddr(ctx))
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

	intent, err := g.grpcHandler.SubmitIntent(req)
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

func (g *gateway) SubmitIntentSolution(ctx context.Context, req *pb.SubmitIntentSolutionRequest) (*pb.SubmitIntentSolutionReply, error) {
	authHeader := retrieveAuthHeader(ctx, "")
	_, err := g.validateAuthHeader(authHeader, true, true, servers.GetPeerAddr(ctx))
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

	intentSolution, err := g.grpcHandler.SubmitIntentSolution(ctx, req)
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

func (g *gateway) Intents(req *pb.IntentsRequest, stream pb.Gateway_IntentsServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), "")
	accountModel, err := g.validateAuthHeader(authHeader, true, true, servers.GetPeerAddr(stream.Context()))
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

	if g.intentsManager.IntentsSubscriptionExists(req.SolverAddress) {
		return status.Errorf(codes.AlreadyExists, "intents subscription for solver address %s already exists", req.SolverAddress)
	}

	g.intentsManager.AddIntentsSubscription(req)
	// send intentsSubscription to Relay
	sub := bxmessage.NewIntentsSubscription(req.SolverAddress, req.Hash, req.Signature)
	g.broadcast(sub, nil, utils.Relay)

	defer func() {
		g.intentsManager.RmIntentsSubscription(req.SolverAddress)
		// send intentsUnsubscription to Relay
		unsub := bxmessage.NewIntentsUnsubscription(req.SolverAddress)
		g.broadcast(unsub, nil, utils.Relay)
		g.log.Debugf("unsubscribed from intents feed for solverAddress: %s, sent IntentsUnsubscribe msg", req.SolverAddress)
	}()

	return g.grpcHandler.Intents(req, stream, *accountModel)
}

func (g *gateway) IntentSolutions(req *pb.IntentSolutionsRequest, stream pb.Gateway_IntentSolutionsServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), "")
	accountModel, err := g.validateAuthHeader(authHeader, true, true, servers.GetPeerAddr(stream.Context()))
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

	if g.intentsManager.SolutionsSubscriptionExists(req.DappAddress) {
		return status.Errorf(codes.AlreadyExists, "solutions subscription for dApp address %s already exists", req.DappAddress)
	}

	g.intentsManager.AddSolutionsSubscription(req)
	// send solutionsSubscription to Relay
	sub := bxmessage.NewSolutionsSubscription(req.DappAddress, req.Hash, req.Signature)
	g.broadcast(sub, nil, utils.Relay)

	defer func() {
		g.intentsManager.RmSolutionsSubscription(req.DappAddress)
		// send solutionsUnsubscription to Relay
		unsub := bxmessage.NewSolutionsUnsubscription(req.DappAddress)
		g.broadcast(unsub, nil, utils.Relay)
	}()

	return g.grpcHandler.IntentSolutions(req, stream, *accountModel)
}

func (g *gateway) sendIntentNotification(intent *types.UserIntent) {
	intentNotification := types.NewUserIntentNotification(intent)
	g.notify(intentNotification)
}

func (g *gateway) sendSolutionNotification(solution *types.UserIntentSolution) {
	solutionNotification := types.NewUserIntentSolutionNotification(solution)
	g.notify(solutionNotification)
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
