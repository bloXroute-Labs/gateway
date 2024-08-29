package grpc

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/config"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/rpc"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/account"
	"github.com/bloXroute-Labs/gateway/v2/services/feed"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/services/validator"
	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/test/mock"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils/orderedmap"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
)

func TestNewTxs(t *testing.T) {
	port := test.NextTestPort()
	ctx, cancel := context.WithCancel(context.Background())
	testServer, feedMngr := testGRPCServer(t, port, "", "")
	wait := start(ctx, t, fmt.Sprintf("0.0.0.0:%v", port), testServer, feedMngr)
	defer func() {
		testServer.Shutdown()
		cancel()
		require.NoError(t, wait())
	}()

	clientConfig := config.NewGRPC("127.0.0.1", port, testGatewayAccountID, testGatewaySecretHash)

	// create a new transaction
	var hash types.SHA256Hash
	hashRes, _ := hex.DecodeString("ed2b4580a766bc9d81c73c35a8496f0461e9c261621cb9f4565ae52ade56056d")
	copy(hash[:], hashRes)
	content, _ := hex.DecodeString("f8708301b7f8851bf08eb0008301388094b877c7e556d50b0027053336b90f36becf67b3dd88050b32f902486000801ca0aa803263146bda76a58ebf9f54be589280e920616bc57e7bd68248821f46fd0ca040266f84a2ecd4719057b0633cc80e3e0b3666f6f6ec1890a920239634ec6531")
	tx := types.NewBxTransaction(hash, networkNum, types.TFPaidTx, time.Now())
	tx.SetContent(content)

	client, err := rpc.GatewayClient(clientConfig)
	require.NoError(t, err)

	newTxsStream, err := client.NewTxs(ctx, &pb.TxsRequest{})
	require.NoError(t, err)

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

	// notify the feed manager about the new transaction
	testServer.gatewayServer.(*server).params.feedManager.Notify(types.CreateNewTransactionNotification(tx))

	var txNotification *pb.TxsReply
	txNotification, err = newTxsStream.Recv()
	require.NoError(t, err)
	require.NotNil(t, txNotification.Tx)
	require.Equal(t, 1, len(txNotification.Tx))

	err = newTxsStream.CloseSend()
	require.NoError(t, err)
}

func TestPendingTxs(t *testing.T) {
	port := test.NextTestPort()
	ctx, cancel := context.WithCancel(context.Background())
	testServer, feedMngr := testGRPCServer(t, port, "", "")
	wait := start(ctx, t, fmt.Sprintf("0.0.0.0:%v", port), testServer, feedMngr)
	defer func() {
		testServer.Shutdown()
		cancel()
		require.NoError(t, wait())
	}()

	clientConfig := config.NewGRPC("127.0.0.1", port, testGatewayAccountID, testGatewaySecretHash)

	// create a new transaction
	var hash types.SHA256Hash
	hashRes, _ := hex.DecodeString("ed2b4580a766bc9d81c73c35a8496f0461e9c261621cb9f4565ae52ade56056d")
	copy(hash[:], hashRes)
	content, _ := hex.DecodeString("f8708301b7f8851bf08eb0008301388094b877c7e556d50b0027053336b90f36becf67b3dd88050b32f902486000801ca0aa803263146bda76a58ebf9f54be589280e920616bc57e7bd68248821f46fd0ca040266f84a2ecd4719057b0633cc80e3e0b3666f6f6ec1890a920239634ec6531")
	tx := types.NewBxTransaction(hash, networkNum, types.TFPaidTx, time.Now())
	tx.SetContent(content)

	client, err := rpc.GatewayClient(clientConfig)
	require.NoError(t, err)

	pendingTxsStream, err := client.PendingTxs(ctx, &pb.TxsRequest{})
	require.NoError(t, err)

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

	// notify the feed manager about the pending transaction
	testServer.gatewayServer.(*server).params.feedManager.Notify(types.CreatePendingTransactionNotification(tx))

	var txNotification *pb.TxsReply
	txNotification, err = pendingTxsStream.Recv()
	require.NoError(t, err)
	require.NotNil(t, txNotification.Tx)
	require.Equal(t, 1, len(txNotification.Tx))

	err = pendingTxsStream.CloseSend()
	require.NoError(t, err)
}

func TestBlxrTx(t *testing.T) {
	privKey, _ := crypto.GenerateKey()

	resetNetworks := func() {
		bxgateway.NetworkNumToChainID = map[types.NetworkNum]types.NetworkID{
			bxgateway.MainnetNum:        bxgateway.EthChainID,
			bxgateway.BSCMainnetNum:     bxgateway.BSCChainID,
			bxgateway.PolygonMainnetNum: bxgateway.PolygonChainID,
			bxgateway.PolygonMumbaiNum:  bxgateway.PolygonChainID,
			bxgateway.HoleskyNum:        bxgateway.HoleskyChainID,
		}
	}
	// swapChainID swaps the chainID for the mainnet to 10
	swapChainID := func() {
		bxgateway.NetworkNumToChainID = map[types.NetworkNum]types.NetworkID{
			bxgateway.MainnetNum: 10,
		}
	}
	generateLegacyTxAndHash := func() (string, string) {
		tx, ethTxBytes := bxmock.NewSignedEthTxBytes(ethtypes.LegacyTxType, 1, privKey, nil)
		return hex.EncodeToString(ethTxBytes), tx.Hash().String()
	}
	generateDynamicFeeTxAndHash := func() (string, string) {
		tx, ethTxBytes := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, privKey, nil)
		return hex.EncodeToString(ethTxBytes), tx.Hash().String()
	}
	setupAccountService := func(g *server) {
		g.params.accService = account.NewService(g.params.sdn, g.log)
	}

	testCases := []struct {
		description          string
		changeNetwork        func()
		setupSdnFunc         func(*server)
		setupFeedManagerFunc func(*server)
		setupAccServiceFunc  func(*server)
		setupValidatorFunc   func(*server)
		request              *pb.BlxrTxRequest
		expectedErrSubStr    string
		generateTxAndHash    func() (string, string)
	}{
		{
			description: "Wrong chainID",
			setupFeedManagerFunc: func(g *server) {
				g.params.feedManager = feed.NewManager(g.params.sdn, services.NewNoOpSubscriptionServices(),
					g.params.sdn.AccountModel(), statistics.NoStats{}, networkNum, true)
			},
			request:           &pb.BlxrTxRequest{},
			generateTxAndHash: generateLegacyTxAndHash,
			expectedErrSubStr: "chainID mismatch",
		},
		{
			description: "Empty account",
			setupSdnFunc: func(s *server) {
				ctl := gomock.NewController(t)
				sdn := mock.NewMockSDNHTTP(ctl)

				sdn.EXPECT().AccountModel().Return(sdnmessage.Account{}).AnyTimes()
				sdn.EXPECT().NetworkNum().Return(networkNum).AnyTimes()
				s.params.sdn = sdn
			},
			setupAccServiceFunc: setupAccountService,
			request:             &pb.BlxrTxRequest{},
			generateTxAndHash:   generateLegacyTxAndHash,
			expectedErrSubStr:   "not authorized to call this method",
		},
		{
			description: "Wrong transaction format",
			request:     &pb.BlxrTxRequest{},
			generateTxAndHash: func() (string, string) {
				return "f800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000", "0x00"
			},

			expectedErrSubStr: "failed to unmarshal tx",
		},
		{
			description:       "Send legacy type transaction",
			changeNetwork:     swapChainID,
			request:           &pb.BlxrTxRequest{},
			generateTxAndHash: generateLegacyTxAndHash,
		},
		{
			description:       "Send dynamic fee type transaction",
			changeNetwork:     swapChainID,
			request:           &pb.BlxrTxRequest{},
			generateTxAndHash: generateDynamicFeeTxAndHash,
		},
		{
			description:   "Send transaction with node validation",
			changeNetwork: swapChainID,
			request: &pb.BlxrTxRequest{
				NodeValidation: true,
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
		},
		{
			description:   "Send transaction with validators_only",
			changeNetwork: swapChainID,
			request: &pb.BlxrTxRequest{
				ValidatorsOnly: true,
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
		},
		{
			description:   "Send transaction with next validator",
			changeNetwork: swapChainID,
			setupSdnFunc: func(s *server) {
				ctl := gomock.NewController(t)
				sdn := mock.NewMockSDNHTTP(ctl)
				sdn.EXPECT().AccountModel().Return(testAccountModel).AnyTimes()
				sdn.EXPECT().NetworkNum().Return(bxgateway.BSCMainnetNum).AnyTimes()
				s.params.sdn = sdn
			},
			setupValidatorFunc: func(g *server) {
				validatorStatusMap := syncmap.NewStringMapOf[bool]()
				validatorStatusMap.Store(testWalletID, true)
				validatorStatusMap.Store(testWalletID2, true)
				nextValidatorMap := orderedmap.New()
				nextValidatorMap.Set(1, testWalletID)
				nextValidatorMap.Set(2, testWalletID2)

				g.params.validatorsManager = validator.NewManager(nextValidatorMap, validatorStatusMap, nil)
			},
			request: &pb.BlxrTxRequest{
				NextValidator: true,
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
		},
		{
			description:   "Wrong network on next validator",
			changeNetwork: swapChainID,
			setupValidatorFunc: func(g *server) {
				g.params.validatorsManager = nil
			},
			request: &pb.BlxrTxRequest{
				NextValidator: true,
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "currently next_validator is only supported on BSC and Polygon networks",
		},
		{
			description:   "Nil validator map on next validator",
			changeNetwork: swapChainID,
			setupSdnFunc: func(s *server) {
				ctl := gomock.NewController(t)
				sdn := mock.NewMockSDNHTTP(ctl)
				sdn.EXPECT().AccountModel().Return(testAccountModel).AnyTimes()
				sdn.EXPECT().NetworkNum().Return(bxgateway.BSCMainnetNum).AnyTimes()
				s.params.sdn = sdn
			},
			setupValidatorFunc: func(g *server) {
				g.params.validatorsManager = nil
			},
			request: &pb.BlxrTxRequest{
				NextValidator: true,
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "failed to send next validator tx",
		},
		{
			description:   "Empty validator map on next validator",
			changeNetwork: swapChainID,
			setupSdnFunc: func(s *server) {
				ctl := gomock.NewController(t)
				sdn := mock.NewMockSDNHTTP(ctl)
				sdn.EXPECT().AccountModel().Return(testAccountModel).AnyTimes()
				sdn.EXPECT().NetworkNum().Return(bxgateway.BSCMainnetNum).AnyTimes()
				s.params.sdn = sdn
			},
			setupValidatorFunc: func(g *server) {
				validatorStatusMap := syncmap.NewStringMapOf[bool]()
				nextValidatorMap := orderedmap.New()

				g.params.validatorsManager = validator.NewManager(nextValidatorMap, validatorStatusMap, nil)
			},
			request: &pb.BlxrTxRequest{
				NextValidator: true,
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "can't send tx with next_validator because the gateway encountered an issue fetching the epoch block",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			if tc.changeNetwork != nil {
				tc.changeNetwork()
			}

			defer resetNetworks()

			port := test.NextTestPort()
			ctx, cancel := context.WithCancel(context.Background())
			testServer, _ := testGRPCServer(t, port, "", "")
			if tc.setupSdnFunc != nil {
				tc.setupSdnFunc(testServer.gatewayServer.(*server))
			}
			if tc.setupFeedManagerFunc != nil {
				tc.setupFeedManagerFunc(testServer.gatewayServer.(*server))
			}
			if tc.setupValidatorFunc != nil {
				tc.setupValidatorFunc(testServer.gatewayServer.(*server))
			}
			if tc.setupAccServiceFunc != nil {
				tc.setupAccServiceFunc(testServer.gatewayServer.(*server))
			}

			wait := start(ctx, t, fmt.Sprintf("0.0.0.0:%v", port), testServer, testServer.gatewayServer.(*server).params.feedManager.(*feed.Manager))
			defer func() {
				testServer.Shutdown()
				cancel()
				require.NoError(t, wait())
			}()

			tx, hash := tc.generateTxAndHash()
			tc.request.Transaction = tx

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
			res, err := client.BlxrTx(ctx, tc.request)
			if tc.expectedErrSubStr != "" {
				require.True(t, strings.Contains(err.Error(), tc.expectedErrSubStr))
			} else {
				require.Nil(t, err)
				require.Equal(t, fmt.Sprintf("0x%v", res.TxHash), hash)
			}
		})
	}
}

func TestGatewayGRPCBlxrBatchTx(t *testing.T) {
	privKey, _ := crypto.GenerateKey()

	resetNetworks := func() {
		bxgateway.NetworkNumToChainID = map[types.NetworkNum]types.NetworkID{
			bxgateway.MainnetNum:        bxgateway.EthChainID,
			bxgateway.BSCMainnetNum:     bxgateway.BSCChainID,
			bxgateway.PolygonMainnetNum: bxgateway.PolygonChainID,
			bxgateway.PolygonMumbaiNum:  bxgateway.PolygonChainID,
			bxgateway.HoleskyNum:        bxgateway.HoleskyChainID,
		}
	}
	// swapChainID swaps the chainID for the mainnet to 10
	swapChainID := func() {
		bxgateway.NetworkNumToChainID = map[types.NetworkNum]types.NetworkID{
			bxgateway.MainnetNum: 10,
		}
	}

	generateLegacyTxAndHash := func() ([]string, []string) {
		tx1, ethTxBytes1 := bxmock.NewSignedEthTxBytes(ethtypes.LegacyTxType, 1, privKey, nil)
		tx2, ethTxBytes2 := bxmock.NewSignedEthTxBytes(ethtypes.LegacyTxType, 1, privKey, nil)
		return []string{hex.EncodeToString(ethTxBytes1), hex.EncodeToString(ethTxBytes2)}, []string{tx1.Hash().String(), tx2.Hash().String()}
	}
	generateDynamicFeeTxAndHash := func() ([]string, []string) {
		tx1, ethTxBytes1 := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, privKey, nil)
		tx2, ethTxBytes2 := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, privKey, nil)
		return []string{hex.EncodeToString(ethTxBytes1), hex.EncodeToString(ethTxBytes2)}, []string{tx1.Hash().String(), tx2.Hash().String()}
	}

	testCases := []struct {
		description          string
		changeNetwork        func()
		setupSdnFunc         func(*server)
		setupFeedManagerFunc func(*server)
		setupAccServiceFunc  func(*server)
		setupValidatorFunc   func(*server)
		request              *pb.BlxrBatchTXRequest
		expectedErrSubStr    []string
		generateTxAndHash    func() ([]string, []string)
	}{
		{
			description:       "Send legacy type transaction",
			changeNetwork:     swapChainID,
			request:           &pb.BlxrBatchTXRequest{},
			generateTxAndHash: generateLegacyTxAndHash,
		},
		{
			description:       "Send dynamic fee type transaction",
			changeNetwork:     swapChainID,
			request:           &pb.BlxrBatchTXRequest{},
			generateTxAndHash: generateDynamicFeeTxAndHash,
		},
		{
			description: "Batch limit exceeded",
			request:     &pb.BlxrBatchTXRequest{},
			generateTxAndHash: func() ([]string, []string) {
				batchLimit := 10
				var txs []string
				var hashes []string

				for i := 0; i <= batchLimit; i++ {
					tx, ethTxBytes := bxmock.NewSignedEthTxBytes(ethtypes.LegacyTxType, 1, privKey, nil)
					txs = append(txs, hex.EncodeToString(ethTxBytes))
					hashes = append(hashes, tx.Hash().String())
				}
				return txs, hashes
			},
			expectedErrSubStr: []string{"blxr-batch-tx currently supports a maximum of"},
		},
		{
			description:   "First successful, second failed",
			changeNetwork: swapChainID,
			request:       &pb.BlxrBatchTXRequest{},
			generateTxAndHash: func() ([]string, []string) {
				tx1, ethTxBytes1 := bxmock.NewSignedEthTxBytes(ethtypes.LegacyTxType, 1, privKey, nil)
				return []string{hex.EncodeToString(ethTxBytes1), "00"}, []string{tx1.Hash().String(), "0x00"}
			},
			expectedErrSubStr: []string{"", "typed transaction too short"},
		},
		{
			description:   "First failed, second successful",
			changeNetwork: swapChainID,
			request:       &pb.BlxrBatchTXRequest{},
			generateTxAndHash: func() ([]string, []string) {
				tx1, ethTxBytes1 := bxmock.NewSignedEthTxBytes(ethtypes.LegacyTxType, 1, privKey, nil)
				return []string{"00", hex.EncodeToString(ethTxBytes1)}, []string{"0x00", tx1.Hash().String()}
			},
			expectedErrSubStr: []string{"typed transaction too short", ""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			if tc.changeNetwork != nil {
				tc.changeNetwork()
			}

			defer resetNetworks()

			port := test.NextTestPort()
			ctx, cancel := context.WithCancel(context.Background())
			testServer, _ := testGRPCServer(t, port, "", "")
			if tc.setupSdnFunc != nil {
				tc.setupSdnFunc(testServer.gatewayServer.(*server))
			}
			if tc.setupFeedManagerFunc != nil {
				tc.setupFeedManagerFunc(testServer.gatewayServer.(*server))
			}
			if tc.setupValidatorFunc != nil {
				tc.setupValidatorFunc(testServer.gatewayServer.(*server))
			}
			if tc.setupAccServiceFunc != nil {
				tc.setupAccServiceFunc(testServer.gatewayServer.(*server))
			}

			wait := start(ctx, t, fmt.Sprintf("0.0.0.0:%v", port), testServer, testServer.gatewayServer.(*server).params.feedManager.(*feed.Manager))
			defer func() {
				testServer.Shutdown()
				cancel()
				require.NoError(t, wait())
			}()

			txs, hashes := tc.generateTxAndHash()
			var txsAndSenders []*pb.TxAndSender
			var expectedHashes []string

			for idx, tx := range txs {
				txsAndSenders = append(txsAndSenders, &pb.TxAndSender{Transaction: tx})
				expectedHashes = append(expectedHashes, hashes[idx])
			}
			tc.request.TransactionsAndSenders = txsAndSenders

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
			res, err := client.BlxrBatchTX(ctx, tc.request)
			require.Nil(t, err)
			for _, respErrAndIdx := range res.TxErrors {
				require.NotEqual(t, "", tc.expectedErrSubStr[respErrAndIdx.Idx])
				require.True(t, strings.Contains(respErrAndIdx.Error, tc.expectedErrSubStr[respErrAndIdx.Idx]))
			}
			for _, respHashAndIdx := range res.TxHashes {
				require.Equal(t, fmt.Sprintf("0x%v", respHashAndIdx.TxHash), expectedHashes[respHashAndIdx.Idx])
			}
		})
	}
}

func TestGatewaySubmitBundle(t *testing.T) {
	generateLegacyTxAndHash := func(privKey *ecdsa.PrivateKey, chain *big.Int) ([]string, []string) {
		tx1, ethTxBytes1 := bxmock.NewSignedEthTxBytes(ethtypes.LegacyTxType, 1, privKey, chain)
		tx2, ethTxBytes2 := bxmock.NewSignedEthTxBytes(ethtypes.LegacyTxType, 1, privKey, chain)
		return []string{hex.EncodeToString(ethTxBytes1), hex.EncodeToString(ethTxBytes2)}, []string{tx1.Hash().String(), tx2.Hash().String()}
	}
	generateDynamicFeeTxAndHash := func(privKey *ecdsa.PrivateKey, chain *big.Int) ([]string, []string) {
		tx1, ethTxBytes1 := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, privKey, chain)
		tx2, ethTxBytes2 := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, privKey, chain)
		return []string{hex.EncodeToString(ethTxBytes1), hex.EncodeToString(ethTxBytes2)}, []string{tx1.Hash().String(), tx2.Hash().String()}
	}

	testCases := []struct {
		description         string
		setupSdnFunc        func(*server)
		setupAccServiceFunc func(*server)
		request             *pb.BlxrSubmitBundleRequest
		expectedErrSubStr   string
		privKey             *ecdsa.PrivateKey
		chainID             *big.Int
		generateTxAndHash   func(*ecdsa.PrivateKey, *big.Int) ([]string, []string)
	}{
		{
			description: "Send legacy type transactions",
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "0x1f71710",
			},
			generateTxAndHash: generateLegacyTxAndHash,
		},
		{
			description: "Send dynamic fee type transactions",
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "0x1f71710",
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
		},
		{
			description: "Txs limit exceeded",
			setupSdnFunc: func(s *server) {
				ctl := gomock.NewController(t)
				sdn := mock.NewMockSDNHTTP(ctl)
				sdn.EXPECT().AccountModel().Return(sdnmessage.Account{
					AccountInfo: sdnmessage.AccountInfo{
						AccountID: testGatewayAccountID,
						TierName:  testTierName,
					},
					SecretHash: testGatewaySecretHash,
					Bundles: sdnmessage.BDNBundlesService{
						Networks: map[string]sdnmessage.BundleProperties{
							"Mainnet": {
								TxsLenLimit: 1,
							},
						},
					},
				}).AnyTimes()
				sdn.EXPECT().NetworkNum().Return(networkNum).AnyTimes()
				s.params.sdn = sdn
			},
			setupAccServiceFunc: func(s *server) {
				s.params.accService = account.NewService(s.params.sdn, s.log)
			},
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "0x1f71710",
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "txs limit exceeded",
		},
		{
			description: "Account wrong tier",
			setupSdnFunc: func(s *server) {
				ctl := gomock.NewController(t)
				sdn := mock.NewMockSDNHTTP(ctl)
				sdn.EXPECT().AccountModel().Return(sdnmessage.Account{
					AccountInfo: sdnmessage.AccountInfo{
						AccountID: testGatewayAccountID,
						TierName:  sdnmessage.ATierIntroductory,
					},
					SecretHash: testGatewaySecretHash,
				}).AnyTimes()
				sdn.EXPECT().NetworkNum().Return(networkNum).AnyTimes()
				s.params.sdn = sdn
			},
			setupAccServiceFunc: func(s *server) {
				s.params.accService = account.NewService(s.params.sdn, s.log)
			},
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "0x1f71710",
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "enterprise elite account is required in order to send bundle",
		},
		{
			description: "Wrong chainID",
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "0x1f71710",
			},
			chainID:           big.NewInt(10),
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "invalid chain id for signer",
		},
		{
			description:       "Bundle with missing block number",
			request:           &pb.BlxrSubmitBundleRequest{},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "bundle missing blockNumber",
		},
		{
			description: "Bundle with block number wrong format",
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "1",
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "blockNumber must be hex",
		},
		{
			description: "Bundle with no txs",
			request:     &pb.BlxrSubmitBundleRequest{},
			generateTxAndHash: func(*ecdsa.PrivateKey, *big.Int) ([]string, []string) {
				return []string{}, []string{}
			},
			expectedErrSubStr: "bundle missing txs",
		},
		{
			description: "Bundle with negative min timestamp",
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber:  "0x1f71710",
				MinTimestamp: -1,
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "min timestamp must be greater than or equal to 0",
		},
		{
			description: "Bundle with negative max timestamp",
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber:  "0x1f71710",
				MaxTimestamp: -1,
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "max timestamp must be greater than or equal to 0",
		},
		{
			description: "Bundle with wrong UUID",
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "0x1f71710",
				Uuid:        "0",
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "invalid UUID",
		},
		{
			description: "Wrong network number",
			setupSdnFunc: func(s *server) {
				ctl := gomock.NewController(t)
				sdn := mock.NewMockSDNHTTP(ctl)
				sdn.EXPECT().AccountModel().Return(testAccountModel).AnyTimes()
				sdn.EXPECT().NodeID().Return(types.NodeID("node_id")).AnyTimes()
				sdn.EXPECT().Networks().Return(&blockchainNetworks).AnyTimes()
				sdn.EXPECT().NetworkNum().Return(types.NetworkNum(36)).AnyTimes()
				s.params.sdn = sdn
			},
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "0x1f71710",
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "invalid network",
		},
		{
			description: "Wrong tx format",
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "0x1f71710",
			},
			generateTxAndHash: func(privKey *ecdsa.PrivateKey, chain *big.Int) ([]string, []string) {
				tx1, ethTxBytes1 := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, privKey, chain)
				return []string{hex.EncodeToString(ethTxBytes1), "f800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"},
					[]string{tx1.Hash().String(), "0x00"}
			},
			expectedErrSubStr: "unable to parse bundle",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			privKey, _ := crypto.GenerateKey()
			port := test.NextTestPort()
			chain := big.NewInt(1)

			ctx, cancel := context.WithCancel(context.Background())
			testServer, _ := testGRPCServer(t, port, "", "")
			if tc.setupSdnFunc != nil {
				tc.setupSdnFunc(testServer.gatewayServer.(*server))
			}
			if tc.setupAccServiceFunc != nil {
				tc.setupAccServiceFunc(testServer.gatewayServer.(*server))
			}

			wait := start(ctx, t, fmt.Sprintf("0.0.0.0:%v", port), testServer, testServer.gatewayServer.(*server).params.feedManager.(*feed.Manager))
			defer func() {
				testServer.Shutdown()
				cancel()
				require.NoError(t, wait())
			}()

			if tc.privKey != nil {
				privKey = tc.privKey
			}
			if tc.chainID != nil {
				chain = tc.chainID
			}

			txs, _ := tc.generateTxAndHash(privKey, chain)
			tc.request.Transactions = txs

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
			_, err = client.BlxrSubmitBundle(ctx, tc.request)
			if tc.expectedErrSubStr != "" {
				require.NotNil(t, err)
				require.True(t, strings.Contains(err.Error(), tc.expectedErrSubStr))
			} else {
				require.Nil(t, err)
			}
		})
	}
}
