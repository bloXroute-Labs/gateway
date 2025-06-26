package grpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/sourcegraph/jsonrpc2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/sdnsdk"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/rpc"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/account"
	"github.com/bloXroute-Labs/gateway/v2/services/feed"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

//go:generate mockgen -destination ../../test/mock/connector_mock.go -package mock . Connector

const (
	windowSize = 128 * 1024
	// bufferSize determines how much data can be batched before doing a writing
	// on the wire. Zero or negative values will disable the write buffer such that each
	// writing will be on underlying connection.
	bufferSize = 0

	connectionStatusConnected    = "connected"
	connectionStatusNotConnected = "not_connected"
)

// Connector is responsible for broadcasting messages to all connected clients
// and getting info about connected clients
type Connector interface {
	Broadcast(msg bxmessage.Message, source connections.Conn, to bxtypes.NodeType) types.BroadcastResults
	Peers(peerType string) []bxmessage.PeerInfo
	Relays() map[string]bxmessage.RelayConnectionInfo
}

// feedManager declares the interface of the feed manager
type feedManager interface {
	Notify(notification types.Notification)
	GetGrpcSubscriptionReply() []feed.ClientSubscriptionFullInfo
	Subscribe(feedName types.FeedType, feedConnectionType types.FeedConnectionType, conn *jsonrpc2.Conn, ci types.ClientInfo, ro types.ReqOptions, ethSubscribe bool) (*feed.ClientSubscriptionHandlingInfo, error)
	Unsubscribe(subscriptionID string, closeClientConnection bool, errMsg string) error
}

// Server implementation for grpc server
type Server struct {
	listenAddr       string
	encodedAuth      string
	stats            statistics.Stats
	gatewayAccountID bxtypes.AccountID
	server           *grpc.Server
	gatewayServer    pb.GatewayServer
	mu               sync.RWMutex
}

// NewGRPCServer is the constructor of the Server object
func NewGRPCServer(
	config *config.Bx,
	stats statistics.Stats,
	node connections.BxListener,
	sdn sdnsdk.SDNHTTP,
	accService account.Accounter,
	bridge blockchain.Bridge,
	blockchainPeers []types.NodeEndpoint,
	wsManager blockchain.WSManager,
	bdnStats *bxmessage.BdnPerformanceStats,
	timeStarted time.Time,
	gatewayPublicKey string,
	connector Connector,
	feedManager feedManager,
	txStore services.TxStore,
	txFromFieldIncludable bool,
	oFACList *types.OFACMap,
) *Server {
	params := grpcParams{
		node:                           node,
		sdn:                            sdn,
		accService:                     accService,
		bridge:                         bridge,
		blockchainPeers:                blockchainPeers,
		wsManager:                      wsManager,
		bdnStats:                       bdnStats,
		timeStarted:                    timeStarted,
		gatewayPublicKey:               gatewayPublicKey,
		connector:                      connector,
		txFromFieldIncludable:          txFromFieldIncludable,
		feedManager:                    feedManager,
		txStore:                        txStore,
		chainID:                        bxtypes.NetworkNumToChainID[sdn.NetworkNum()],
		oFACList:                       oFACList,
	}

	grpcHostPort := fmt.Sprintf("%v:%v", config.Host, config.Port)

	var encodedAuth string
	if config.User != "" && config.Password != "" {
		encodedAuth = rpc.EncodeUserSecret(config.User, config.Password)
	} else {
		encodedAuth = ""
	}

	gRPCServer := &Server{
		listenAddr:       grpcHostPort,
		encodedAuth:      encodedAuth,
		stats:            stats,
		gatewayAccountID: sdn.NodeModel().AccountID,
		gatewayServer:    newServer(params),
	}

	return gRPCServer
}

// Run run grpc server
func (gs *Server) Run() error {
	serverOptions := []grpc.ServerOption{
		grpc.WriteBufferSize(bufferSize),
		grpc.InitialConnWindowSize(windowSize),
		grpc.UnaryInterceptor(gs.authenticate),
		grpc.ChainUnaryInterceptor(gs.authenticate, gs.reqSDKStats),
	}
	gs.mu.Lock()
	gs.server = grpc.NewServer(serverOptions...)
	gs.mu.Unlock()
	pb.RegisterGatewayServer(gs.server, gs.gatewayServer)

	listener, err := net.Listen("tcp", gs.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	log.Infof("GRPC server is starting on %v", gs.listenAddr)

	if err := gs.server.Serve(listener); err != nil {
		return fmt.Errorf("failed to serve: %v", err)
	}

	return nil
}

// Shutdown shuts down the grpc server
func (gs *Server) Shutdown() {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	if gs.server != nil {
		log.Infof("shutting down gRPC server")
		gs.server.Stop()
	}
}

func (gs *Server) authenticate(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if gs.encodedAuth != "" {
		auth, err := rpc.ReadAuthMetadata(ctx)
		if err != nil {
			return nil, err
		}

		if auth != gs.encodedAuth {
			return nil, errors.New("provided auth information was incorrect")
		}
	}
	return handler(ctx, req)
}

func (gs *Server) reqSDKStats(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	start := time.Now()

	resp, err := handler(ctx, req)

	if ok && len(md.Get(types.SDKVersionHeaderKey)) > 0 {
		method := string(jsonrpc.RPCMethodToRPCRequestType[info.FullMethod])
		if method == "" {
			// in case the method is not mapped
			method = info.FullMethod
		}
		go gs.sdkStat(md, method, start)
	}

	return resp, err
}

func (gs *Server) sdkStat(md metadata.MD, method string, start time.Time) {
	var blockchain, sourceCode, version string

	if blockchainHeader := md.Get(types.SDKBlockchainHeaderKey); len(blockchainHeader) > 0 {
		blockchain = blockchainHeader[0]
	}
	if sourceCodeHeader := md.Get(types.SDKCodeLanguageHeaderKey); len(sourceCodeHeader) > 0 {
		sourceCode = sourceCodeHeader[0]
	}
	if sdkVersionHeader := md.Get(types.SDKVersionHeaderKey); len(sdkVersionHeader) > 0 {
		version = sdkVersionHeader[0]
	}

	// blockchain, method, Feed, sourceCode string, start, end time.Time, count int
	gs.stats.LogSDKInfo(blockchain, method, sourceCode, version, gs.gatewayAccountID, types.GRPCFeed, start, time.Now())
}

// getPeerAddr returns the address of the gRPC connected client given its context
func getPeerAddr(ctx context.Context) string {
	var peerAddress string
	if p, ok := peer.FromContext(ctx); ok {
		peerAddress = p.Addr.String()
	}
	return peerAddress
}

func retrieveAuthHeader(ctx context.Context, authFromRequestBody string) string {
	authHeader, err := rpc.ReadAuthMetadata(ctx)
	if err == nil {
		return authHeader
	}

	// deprecated
	return authFromRequestBody
}

func retrieveOriginalSenderAccountID(ctx context.Context, accountModel *sdnmessage.Account) (*bxtypes.AccountID, error) {
	accountID := accountModel.AccountID
	if accountModel.AccountID == bxtypes.BloxrouteAccountID {
		md, ok := metadata.FromIncomingContext(ctx)
		if ok && len(md.Get(types.OriginalSenderAccountIDHeaderKey)) > 0 {
			accountID = bxtypes.AccountID(md.Get(types.OriginalSenderAccountIDHeaderKey)[0])
		} else {
			return nil, fmt.Errorf("request sent from cloud services and should include %v header", types.OriginalSenderAccountIDHeaderKey)
		}
	}
	return &accountID, nil
}

func ipport(ip string, port int) string { return fmt.Sprintf("%s:%d", ip, port) }
