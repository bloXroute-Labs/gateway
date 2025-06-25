package grpc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/sync/errgroup"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	ethtest "github.com/bloXroute-Labs/gateway/v2/blockchain/eth/test"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/metrics"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/rpc"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/feed"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/test/mock"
	"github.com/bloXroute-Labs/gateway/v2/version"
)

const (
	networkNum                bxtypes.NetworkNum = 5
	testGatewayAccountID                         = "user"
	testGatewaySecretHash                        = "password"
	testGatewayUserAuthHeader                    = "dXNlcjpwYXNzd29yZA==" // encoded testGatewayAccountID and testGatewaySecretHash
	testGatewayAccountID2                        = "user2"
	testGatewaySecretHash2                       = "password2"
	testWalletID                                 = "0x00112233445566778899AABBCCDDEEFFGGHHIIJJ"
	testWalletID2                                = "0xAABBCCDDEEFFGGHHIIJJ00112233445566778899"
)

var (
	testAccountModel = sdnmessage.Account{
		AccountInfo: sdnmessage.AccountInfo{
			AccountID: bxtypes.AccountID(testGatewayAccountID),
			TierName:  testGatewaySecretHash,
		},
		SecretHash: testGatewaySecretHash,
	}
	blockchainNetworks = sdnmessage.BlockchainNetworks{5: bxmock.MockNetwork(networkNum, "Ethereum", "Mainnet", 0)}
	errTestAuth        = fmt.Errorf("some error")

	accountIDToAccountModel = map[bxtypes.AccountID]sdnmessage.Account{
		"user":  {AccountInfo: sdnmessage.AccountInfo{AccountID: testGatewayAccountID, TierName: sdnmessage.ATierUltra}, SecretHash: testGatewaySecretHash},
		"user2": {AccountInfo: sdnmessage.AccountInfo{AccountID: testGatewayAccountID2, TierName: sdnmessage.ATierDeveloper}, SecretHash: testGatewaySecretHash2},
	}
)

type mockAccountService struct{}

func (s *mockAccountService) Authorize(accountID bxtypes.AccountID, hash string, _ bool, _ string) (sdnmessage.Account, error) {
	acc, ok := accountIDToAccountModel[accountID]
	if !ok {
		return sdnmessage.Account{}, errTestAuth
	}
	if acc.SecretHash != hash {
		return sdnmessage.Account{}, errTestAuth
	}

	return acc, nil
}

func testGRPCServer(t *testing.T, port int, user string, password string) (*Server, *feed.Manager) {
	nm := sdnmessage.NodeModel{
		NodeType:             "EXTERNAL_GATEWAY",
		BlockchainNetworkNum: networkNum,
		ExternalIP:           "172.0.0.1",
	}
	logConfig := log.Config{
		AppName:      "gateway-test",
		FileName:     "test-logfile",
		FileLevel:    log.TraceLevel,
		ConsoleLevel: log.TraceLevel,
		MaxSize:      100,
		MaxBackups:   2,
		MaxAge:       1,
	}
	txTraceLog := config.TxTraceLog{
		Enabled:        true,
		MaxFileSize:    100,
		MaxBackupFiles: 3,
	}
	serverConfig := config.NewGRPC("0.0.0.0", port, user, password)
	cfg := &config.Bx{
		Host:       "0.0.0.0",
		NoStats:    true,
		Config:     &logConfig,
		NodeType:   bxtypes.Gateway,
		TxTraceLog: &txTraceLog,
		GRPC:       serverConfig,
	}

	stats := statistics.NoStats{}

	ctl := gomock.NewController(t)
	sdn := mock.NewMockSDNHTTP(ctl)
	sdn.EXPECT().AccountModel().Return(testAccountModel).AnyTimes()
	sdn.EXPECT().NodeID().Return(bxtypes.NodeID("node_id")).AnyTimes()
	sdn.EXPECT().NodeModel().Return(&nm).AnyTimes()
	sdn.EXPECT().Networks().Return(&blockchainNetworks).AnyTimes()
	sdn.EXPECT().NetworkNum().Return(networkNum).AnyTimes()

	bridge := blockchain.NewBxBridge(eth.Converter{}, true)
	blockchainPeers, blockchainPeersInfo := ethtest.GenerateBlockchainPeersInfo(1)

	wsManager := eth.NewEthWSManager(blockchainPeersInfo, eth.NewMockWSProvider, bxgateway.WSProviderTimeout, false)
	wsManager.UpdateNodeSyncStatus(blockchainPeers[0], blockchain.Synced) // required for TxReceipts feed

	bx := mock.NewMockConnector(ctl)

	feedMngr := feed.NewManager(sdn, services.NewNoOpSubscriptionServices(),
		accountIDToAccountModel["gw"], stats, bxtypes.NetworkNum(5), true, &metrics.NoOpExporter{})

	grpcServer := NewGRPCServer(
		cfg,
		stats,
		bxmock.MockBxListener{},
		sdn,
		&mockAccountService{},
		bridge,
		blockchainPeers,
		wsManager,
		bxmessage.NewBDNStats(blockchainPeers, make(map[string]struct{})),
		time.Now(),
		"",
		bx,
		feedMngr,
		nil,
		false,
		nil,
	)

	return grpcServer, feedMngr
}

func start(ctx context.Context, t *testing.T, serverAddress string, grpcServer *Server, manager *feed.Manager) func() error {
	eg, gCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return grpcServer.Run()
	})
	eg.Go(func() error {
		manager.Start(gCtx)
		return nil
	})

	test.WaitServerStarted(t, serverAddress)

	return eg.Wait
}

func TestServerAuth(t *testing.T) {
	port := test.NextTestPort()
	ctx, cancel := context.WithCancel(context.Background())
	testServer, feedMngr := testGRPCServer(t, port, "", "")
	wait := start(ctx, t, fmt.Sprintf("0.0.0.0:%v", port), testServer, feedMngr)
	defer func() {
		testServer.Shutdown()
		cancel()
		require.NoError(t, wait())
	}()

	ctl := gomock.NewController(t)
	mockedSdn := mock.NewMockSDNHTTP(ctl)
	mockedSdn.EXPECT().AccountModel().Return(testAccountModel).AnyTimes()
	testServer.gatewayServer.(*server).params.sdn = mockedSdn

	tests := []struct {
		name       string
		grpcConfig *config.GRPC
		grpcCall   func(ctx context.Context, client pb.GatewayClient) (interface{}, error)
		err        error
	}{
		{
			name:       "no header required",
			grpcConfig: config.NewGRPC("127.0.0.1", port, "", ""),
			grpcCall: func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
				return client.Version(ctx, &pb.VersionRequest{})
			},
			err: nil,
		},
		{
			name:       "user and password",
			grpcConfig: config.NewGRPC("127.0.0.1", port, testGatewayAccountID, testGatewaySecretHash),
			grpcCall: func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
				return client.Version(ctx, &pb.VersionRequest{})
			},
			err: nil,
		},
		{
			name:       "encoded auth",
			grpcConfig: &config.GRPC{Host: "127.0.0.1", Port: port, AuthEnabled: true, EncodedAuthSet: true, EncodedAuth: testGatewayUserAuthHeader, Timeout: 1 * time.Second},
			grpcCall: func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
				return client.Version(ctx, &pb.VersionRequest{})
			},
			err: nil,
		},
		{
			name:       "wrong password",
			grpcConfig: config.NewGRPC("127.0.0.1", port, "user", "wrongpassword"),
			grpcCall: func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
				return client.Version(ctx, &pb.VersionRequest{})
			},
			err: errTestAuth,
		},
		{
			name:       "wrong user",
			grpcConfig: config.NewGRPC("127.0.0.1", port, "user3", "password3"),
			grpcCall: func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
				return client.Version(ctx, &pb.VersionRequest{})
			},
			err: errTestAuth,
		},
		{
			name:       "required header",
			grpcConfig: config.NewGRPC("127.0.0.1", port, "", ""),
			grpcCall: func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
				return client.BlxrTx(ctx, &pb.BlxrTxRequest{})
			},
			err: errMissingAuthHeader,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := rpc.GatewayCall(tt.grpcConfig, tt.grpcCall)
			if tt.err != nil {
				require.NotNil(t, err)
				assert.Contains(t, err.Error(), tt.err.Error())
			} else {
				require.Nil(t, err)
				versionReply, ok := res.(*pb.VersionReply)
				require.True(t, ok)
				require.Equal(t, version.BuildVersion, versionReply.GetVersion())
			}
		})
	}

	t.Run("internal gateway unauthorized access", func(t *testing.T) {
		mockedSdn = mock.NewMockSDNHTTP(ctl)
		mockedSdn.EXPECT().AccountModel().Return(sdnmessage.Account{AccountInfo: sdnmessage.AccountInfo{AccountID: bxtypes.BloxrouteAccountID}}).AnyTimes()
		testServer.gatewayServer.(*server).params.sdn = mockedSdn
		grpcConfig := config.NewGRPC("127.0.0.1", port, "", "")
		_, err := rpc.GatewayCall(grpcConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.Version(ctx, &pb.VersionRequest{})
		})
		require.NotNil(t, err)
		assert.Contains(t, err.Error(), errInternalGwRequiredHeader.Error())
	})
}
