package nodes

import (
	"context"
	"encoding/base64"
	"sync"
	"testing"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	gateway2 "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/servers"
	"github.com/bloXroute-Labs/gateway/v2/test/mock"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/sourcegraph/jsonrpc2"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/metadata"
)

//go:generate mockgen -destination ../test/mock/gw_intents_server_mock.go -package mock github.com/bloXroute-Labs/gateway/v2/protobuf Gateway_IntentsServer

func TestGateway_Intents(t *testing.T) {
	var (
		logEntry            = log.TestEntry()
		ctrl                = gomock.NewController(t)
		relayConnMock       = mock.NewMockConn(ctrl)
		gRPCFeedManagerMock = mock.NewMockGRPCFeedManager(ctrl)
		intentsStreamMock   = mock.NewMockGateway_IntentsServer(ctrl)
		sdnHTTPMock         = mock.NewMockSDNHTTP(ctrl)
		intentsManagerMock  = mock.NewMockIntentsManager(ctrl)
		authHeader          = base64.StdEncoding.EncodeToString([]byte("x:x"))
		rq                  = genIntentsRequest(t)
		c, cancel           = context.WithCancel(context.Background())

		ctx = metadata.NewIncomingContext(c,
			metadata.New(map[string]string{"authorization": authHeader}),
		)

		target = gateway{
			Bx: Bx{
				Connections:     make(connections.ConnList, 0),
				ConnectionsLock: &sync.RWMutex{},
				clock:           utils.RealClock{},
			},
			log:            logEntry,
			sdn:            sdnHTTPMock,
			intentsManager: intentsManagerMock,
			grpcHandler:    servers.NewGrpcHandler(gRPCFeedManagerMock, nil, false),
		}
	)

	// simulate new relay connection and expect calls by bx.OnConnEstablished
	relayConnMock.EXPECT().Log().Return(logEntry)
	relayConnMock.EXPECT().GetConnectionType().Return(utils.Relay).Times(2)
	relayConnMock.EXPECT().GetCapabilities().Return(types.CapabilityFlags(0))
	relayConnMock.EXPECT().Protocol().Return(bxmessage.Protocol(bxmessage.CurrentProtocol))
	relayConnMock.EXPECT().GetNetworkNum().Return(types.AllNetworkNum)
	relayConnMock.EXPECT().GetLocalPort().Return(int64(2222))
	relayConnMock.EXPECT().GetPeerIP().Return("localhost")
	intentsManagerMock.EXPECT().SubscriptionMessages().Return([]bxmessage.Message{})
	require.NoError(t, target.OnConnEstablished(relayConnMock))

	// simulate gRPC subscription
	intentsStreamMock.EXPECT().Context().Return(ctx)

	// mock SDN request
	sdnHTTPMock.EXPECT().AccountModel().Return(sdnmessage.Account{
		SecretHash: "x",
		AccountInfo: sdnmessage.AccountInfo{
			AccountID: "x",
		},
	})

	intentsManagerMock.EXPECT().IntentsSubscriptionExists(rq.SolverAddress).Return(false)
	intentsManagerMock.EXPECT().AddIntentsSubscription(rq)

	// register calls for broadcast of initial subscription message to the connected relay
	relayConnMock.EXPECT().GetConnectionType().Return(utils.Relay)
	relayConnMock.EXPECT().IsOpen().Return(true)
	relayConnMock.EXPECT().Send(&bxmessage.IntentsSubscription{
		SolverAddress: rq.SolverAddress,
		Hash:          rq.Hash,
		Signature:     rq.Signature,
	}).Return(nil)

	// mock MetaInfo extraction in grpc handler after the broadcast
	// including the call during the feed manager subscription
	// plus GetPeerAddr call from validating
	intentsStreamMock.EXPECT().Context().Return(ctx).Times(4)

	// push a signal during the feed manager subscription
	var doneSubscribeFeedManager = make(chan struct{})
	gRPCFeedManagerMock.EXPECT().Subscribe(
		types.FeedType("userIntentFeed"),
		types.GRPCFeed,
		nil,
		gomock.Any(),
		types.ReqOptions{},
		false,
	).DoAndReturn(
		func(feedName types.FeedType,
			feedConnectionType types.FeedConnectionType,
			conn *jsonrpc2.Conn,
			ci types.ClientInfo,
			ro types.ReqOptions,
			ethSubscribe bool,
		) (*servers.ClientSubscriptionHandlingInfo, error) {
			doneSubscribeFeedManager <- struct{}{}
			return &servers.ClientSubscriptionHandlingInfo{
				SubscriptionID:     "id",
				FeedChan:           make(chan types.Notification),
				ErrMsgChan:         make(chan string),
				PermissionRespChan: make(chan *sdnmessage.SubscriptionPermissionMessage),
			}, nil
		})

	// handling unsubscription message (called in defer)
	gRPCFeedManagerMock.EXPECT().Unsubscribe(gomock.Any(), false, "").Return(nil)

	intentsManagerMock.EXPECT().RmIntentsSubscription(rq.SolverAddress)

	// register calls for broadcast of unsubscription message (called in defer)
	relayConnMock.EXPECT().GetConnectionType().Return(utils.Relay)
	relayConnMock.EXPECT().IsOpen().Return(true)
	relayConnMock.EXPECT().Send(&bxmessage.IntentsUnsubscription{
		SolverAddress: rq.SolverAddress,
	}).Return(nil)

	// subscribe to intents
	var doneGRPCSubscribe = make(chan struct{})
	go func() {
		require.NoError(t, target.Intents(rq, intentsStreamMock))
		doneGRPCSubscribe <- struct{}{}
	}()

	// subscription setup is done
	<-doneSubscribeFeedManager

	// simulate another relay (re)connection
	relayConnMock2 := mock.NewMockConn(ctrl)

	// mock calls by bx.OnConnEstablished
	relayConnMock2.EXPECT().Log().Return(logEntry)
	relayConnMock2.EXPECT().GetConnectionType().Return(utils.Relay).Times(2)
	relayConnMock2.EXPECT().GetCapabilities().Return(types.CapabilityFlags(0))
	relayConnMock2.EXPECT().Protocol().Return(bxmessage.Protocol(bxmessage.CurrentProtocol))
	relayConnMock2.EXPECT().GetNetworkNum().Return(types.AllNetworkNum)
	relayConnMock2.EXPECT().GetLocalPort().Return(int64(2222))
	relayConnMock2.EXPECT().GetPeerIP().Return("localhost")

	intentsSubscriptionMessage := &bxmessage.IntentsSubscription{
		SolverAddress: rq.SolverAddress,
		Hash:          rq.Hash,
		Signature:     rq.Signature,
	}

	intentsManagerMock.EXPECT().SubscriptionMessages().Return([]bxmessage.Message{intentsSubscriptionMessage})

	// mock Send call during the OnConnEstablished
	relayConnMock2.EXPECT().Send(intentsSubscriptionMessage).
		Do(func(_ bxmessage.Message) { cancel() }) // kill gRPC request after sending the subscription to a newly connected relay

	// expect unsubscription message to be sent to the new relay after the cancel
	relayConnMock2.EXPECT().GetConnectionType().Return(utils.Relay)
	relayConnMock2.EXPECT().IsOpen().Return(true)
	relayConnMock2.EXPECT().Send(&bxmessage.IntentsUnsubscription{
		SolverAddress: rq.SolverAddress,
	}).Return(nil)

	// connect the second relay
	require.NoError(t, target.OnConnEstablished(relayConnMock2))

	// wait for the handler to die to make sure all expected calls were executed
	<-doneGRPCSubscribe
}

func genIntentsRequest(t *testing.T) *gateway2.IntentsRequest {
	// Generate an ECDSA key pair using secp256k1 curve
	privKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	// Extract the public key
	pubKey := privKey.PublicKey
	signerAddress := crypto.PubkeyToAddress(pubKey)
	hash := crypto.Keccak256Hash([]byte(signerAddress.String())).Bytes()
	sig, err := crypto.Sign(hash, privKey)
	require.NoError(t, err)

	return &gateway2.IntentsRequest{
		SolverAddress: signerAddress.String(),
		Hash:          hash,
		Signature:     sig,
	}
}

func TestIntentsManager(t *testing.T) {
	t.Run("SubscriptionMessages", func(t *testing.T) {
		var target = newIntentsManager()
		target.AddIntentsSubscription(&gateway2.IntentsRequest{SolverAddress: "1"})
		target.AddSolutionsSubscription(&gateway2.IntentSolutionsRequest{DappAddress: "1"})
		require.Len(t, target.SubscriptionMessages(), 2)
	})

	t.Run("AddIntentsSubscription", func(t *testing.T) {
		var target = newIntentsManager()
		target.AddIntentsSubscription(&gateway2.IntentsRequest{SolverAddress: "1"})
		_, ok := target.intentsSubscriptions["1"]
		require.True(t, ok)
	})

	t.Run("RmIntentsSubscription", func(t *testing.T) {
		var target = newIntentsManager()
		target.AddIntentsSubscription(&gateway2.IntentsRequest{SolverAddress: "1"})
		_, ok := target.intentsSubscriptions["1"]
		require.True(t, ok)

		target.RmIntentsSubscription("1")
		_, ok = target.intentsSubscriptions["1"]
		require.False(t, ok)
	})

	t.Run("IntentsSubscriptionExists", func(t *testing.T) {
		var target = newIntentsManager()
		target.AddIntentsSubscription(&gateway2.IntentsRequest{SolverAddress: "1"})
		require.True(t, target.IntentsSubscriptionExists("1"))

		target.RmIntentsSubscription("1")
		require.False(t, target.IntentsSubscriptionExists("1"))
	})

	t.Run("AddSolutionsSubscription", func(t *testing.T) {
		var target = newIntentsManager()
		target.AddSolutionsSubscription(&gateway2.IntentSolutionsRequest{DappAddress: "1"})
		_, ok := target.solutionsSubscriptions["1"]
		require.True(t, ok)
	})

	t.Run("RmSolutionsSubscription", func(t *testing.T) {
		var target = newIntentsManager()
		target.AddSolutionsSubscription(&gateway2.IntentSolutionsRequest{DappAddress: "1"})
		_, ok := target.solutionsSubscriptions["1"]
		require.True(t, ok)

		target.RmSolutionsSubscription("1")
		_, ok = target.solutionsSubscriptions["1"]
		require.False(t, ok)
	})

	t.Run("SolutionsSubscriptionExists", func(t *testing.T) {
		var target = newIntentsManager()
		target.AddSolutionsSubscription(&gateway2.IntentSolutionsRequest{DappAddress: "1"})
		require.True(t, target.SolutionsSubscriptionExists("1"))

		target.RmSolutionsSubscription("1")
		require.False(t, target.SolutionsSubscriptionExists("1"))
	})
}
