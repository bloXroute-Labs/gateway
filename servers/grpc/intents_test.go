package grpc

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/bloXroute-Labs/gateway/v2/config"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/rpc"
	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/mock"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

func TestGateway_Intents(t *testing.T) {
	setupIntentManager := func(s *server, rq *pb.IntentsRequest) {
		ctrl := gomock.NewController(t)
		intentsManagerMock := mock.NewMockIntentsManager(ctrl)
		intentsManagerMock.EXPECT().IntentsSubscriptionExists(rq.SolverAddress).Return(false)
		intentsManagerMock.EXPECT().AddIntentsSubscription(rq.SolverAddress, rq.Hash, rq.Signature)
		intentsManagerMock.EXPECT().RmIntentsSubscription(rq.SolverAddress)
		s.params.intentsManager = intentsManagerMock
	}

	testCases := []struct {
		description           string
		generateIntentRequest func(t *testing.T) *pb.IntentsRequest
		setupIntentManager    func(*server, *pb.IntentsRequest)
		expectedErrSubStr     string
	}{
		{
			description:           "successful intents subscription request",
			generateIntentRequest: genIntentsRequest,
			setupIntentManager:    setupIntentManager,
		},
		{
			description: "invalid signature",
			generateIntentRequest: func(t *testing.T) *pb.IntentsRequest {
				rq := genIntentsRequest(t)
				rq.Signature = []byte("invalid")
				return rq
			},
			expectedErrSubStr: "invalid signature",
		},
		{
			description:           "subscription already exists",
			generateIntentRequest: genIntentsRequest,
			setupIntentManager: func(s *server, rq *pb.IntentsRequest) {
				ctrl := gomock.NewController(t)
				intentsManagerMock := mock.NewMockIntentsManager(ctrl)
				intentsManagerMock.EXPECT().IntentsSubscriptionExists(rq.SolverAddress).Return(true)
				s.params.intentsManager = intentsManagerMock
			},
			expectedErrSubStr: "already exists",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			port := test.NextTestPort()
			ctx, cancel := context.WithCancel(context.Background())
			testServer, feedMngr := testGRPCServer(t, port, "", "")

			rq := tc.generateIntentRequest(t)
			if tc.setupIntentManager != nil {
				tc.setupIntentManager(testServer.gatewayServer.(*server), rq)
			}

			ctl := gomock.NewController(t)
			bx := mock.NewMockConnector(ctl)
			bx.EXPECT().Broadcast(gomock.Any(), nil, utils.RelayProxy).Return(types.BroadcastResults{}).AnyTimes()
			testServer.gatewayServer.(*server).params.connector = bx

			wait := start(ctx, t, fmt.Sprintf("0.0.0.0:%v", port), testServer, feedMngr)
			defer func() {
				testServer.Shutdown()
				cancel()
				require.NoError(t, wait())
			}()

			clientConfig := &config.GRPC{
				Enabled:        true,
				Host:           "127.0.0.1",
				Port:           port,
				AuthEnabled:    true,
				EncodedAuthSet: true,
				EncodedAuth:    testGatewayUserAuthHeader,
				Timeout:        1 * time.Second,
			}

			client, err := rpc.GatewayClient(clientConfig)
			require.NoError(t, err)

			stream, err := client.Intents(ctx, rq)
			require.Nil(t, err)
			if tc.expectedErrSubStr != "" {
				_, err = stream.Recv()
				require.NotNil(t, err)
				require.True(t, strings.Contains(err.Error(), tc.expectedErrSubStr))
			} else {
				// wait for the subscription to be created
				for {
					var subs *pb.SubscriptionsReply
					subs, err = client.Subscriptions(ctx, &pb.SubscriptionsRequest{})
					require.NoError(t, err)
					if len(subs.Subscriptions) > 0 {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}

				intent := &types.UserIntent{ID: "123"}
				intentNotification := types.NewUserIntentNotification(intent)
				feedMngr.Notify(intentNotification)

				res, err := stream.Recv()
				require.Nil(t, err)

				require.Nil(t, err)
				require.NotNil(t, res)

				err = stream.CloseSend()
				require.NoError(t, err)
			}
		})
	}
}

func genIntentsRequest(t *testing.T) *pb.IntentsRequest {
	// Generate an ECDSA key pair using secp256k1 curve
	privKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	// Extract the public key
	pubKey := privKey.PublicKey
	signerAddress := crypto.PubkeyToAddress(pubKey)
	hash := crypto.Keccak256Hash([]byte(signerAddress.String())).Bytes()
	sig, err := crypto.Sign(hash, privKey)
	require.NoError(t, err)

	return &pb.IntentsRequest{
		SolverAddress: signerAddress.String(),
		Hash:          hash,
		Signature:     sig,
	}
}
