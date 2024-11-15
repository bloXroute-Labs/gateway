package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sourcegraph/jsonrpc2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"golang.org/x/sync/errgroup"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth/test"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/feed"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/services/validator"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/test/mock"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils/orderedmap"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
)

var (
	errAuth = fmt.Errorf("some error")
	wsPort  = 28332
	wsURL   = fmt.Sprintf("ws://localhost:%v/ws", wsPort)

	accountIDToAccountModel = map[types.AccountID]sdnmessage.Account{
		"a": {AccountInfo: sdnmessage.AccountInfo{AccountID: "a", TierName: sdnmessage.ATierElite}, SecretHash: "123456"},
		"b": {AccountInfo: sdnmessage.AccountInfo{AccountID: "b", TierName: sdnmessage.ATierDeveloper}, SecretHash: "7891011"},
		"c": {AccountInfo: sdnmessage.AccountInfo{AccountID: "c", TierName: sdnmessage.ATierElite}},
		"i": {AccountInfo: sdnmessage.AccountInfo{AccountID: "i", TierName: sdnmessage.ATierIntroductory}, SecretHash: "654321"},
		"gw": {
			AccountInfo: sdnmessage.AccountInfo{
				AccountID:  "gw",
				ExpireDate: "2999-12-31",
				TierName:   sdnmessage.ATierEnterprise,
			},
			SecretHash: "secret",
			NewTransactionStreaming: sdnmessage.BDNFeedService{
				ExpireDate: "2999-12-31",
				Feed: sdnmessage.FeedProperties{
					AllowFiltering:  true,
					AvailableFields: []string{"all"},
				},
			},
			TransactionReceiptFeed: sdnmessage.BDNFeedService{
				ExpireDate: "2999-12-31",
				Feed: sdnmessage.FeedProperties{
					AllowFiltering:  true,
					AvailableFields: []string{"all"},
				},
			},
			OnBlockFeed: sdnmessage.BDNFeedService{
				ExpireDate: "2999-12-31",
				Feed: sdnmessage.FeedProperties{
					AllowFiltering:  true,
					AvailableFields: []string{"all"},
				},
			},
		},
	}
)

func TestWS(t *testing.T) {
	suite.Run(t, &wsSuite{})
}

type wsSuite struct {
	suite.Suite
	ctx              context.Context
	cancel           context.CancelFunc
	wsURL            string
	feedManager      *feed.Manager
	validatorManager *validator.Manager
	nodeWSManager    blockchain.WSManager
	sdn              connections.SDNHTTP
	blockchainPeers  []types.NodeEndpoint
	server           *Server

	conn *websocket.Conn

	eg *errgroup.Group
}

func (s *wsSuite) SetupSuite() {
	s.setupSuit(bxgateway.MainnetNum)
}

func (s *wsSuite) setupSuit(networkNum types.NetworkNum) {
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx, s.cancel = ctx, cancel

	s.eg, ctx = errgroup.WithContext(ctx)

	ctl := gomock.NewController(s.T())
	mockedSdn := mock.NewMockSDNHTTP(ctl)
	mockedSdn.EXPECT().FetchCustomerAccountModel(types.AccountID("gw")).Return(accountIDToAccountModel["gw"], nil).AnyTimes()
	mockedSdn.EXPECT().NodeID().Return(types.NodeID("nodeID")).AnyTimes()
	mockedSdn.EXPECT().NetworkNum().Return(networkNum).AnyTimes()
	mockedSdn.EXPECT().AccountModel().Return(accountIDToAccountModel["gw"]).AnyTimes()
	mockedSdn.EXPECT().GetQuotaUsage(gomock.AnyOf("a", "b", "c", "i", "gw")).DoAndReturn(func(accountID string) (*connections.QuotaResponseBody, error) {
		res := connections.QuotaResponseBody{
			AccountID:   accountID,
			QuotaFilled: 1,
			QuotaLimit:  2,
		}

		return &res, nil
	}).AnyTimes()

	s.sdn = mockedSdn

	stats := statistics.NoStats{}

	s.feedManager = feed.NewManager(s.sdn, services.NewNoOpSubscriptionServices(),
		accountIDToAccountModel["gw"], stats, networkNum, true)

	im := services.NewIntentsManager()
	as := &mockAccountService{}

	g := bxmock.MockBxListener{}

	cfg := &config.Bx{
		EnableBlockchainRPC: true,
		WebsocketHost:       localhost,
		WebsocketPort:       wsPort,
		ManageWSServer:      true,
		WebsocketTLSEnabled: false,
	}
	s.wsURL = wsURL

	blockchainPeers, blockchainPeersInfo := test.GenerateBlockchainPeersInfo(3)
	s.blockchainPeers = blockchainPeers

	s.nodeWSManager = eth.NewEthWSManager(blockchainPeersInfo, eth.NewMockWSProvider, bxgateway.WSProviderTimeout, false)

	providers := s.nodeWSManager.Providers()
	p1 := providers[blockchainPeers[0].IPPort()]
	s.Assert().NotNil(p1)
	p2 := providers[blockchainPeers[1].IPPort()]
	s.Assert().NotNil(p2)
	p3 := providers[blockchainPeers[2].IPPort()]
	s.Assert().NotNil(p3)

	var validatorManager *validator.Manager
	if networkNum == bxgateway.BSCMainnetNum || networkNum == bxgateway.PolygonMainnetNum {
		nextValidatorMap := orderedmap.New[uint64, string]()
		validatorStatusMap := syncmap.NewStringMapOf[bool]()
		validatorListMap := syncmap.NewIntegerMapOf[uint64, validator.List]()

		nextValidatorMap.Set(uint64(100), "1234")

		validatorManager = validator.NewManager(nextValidatorMap, validatorStatusMap, validatorListMap)
		s.validatorManager = validatorManager
	}

	s.server = NewWSServer(cfg, "", "", s.sdn, g, as, s.feedManager, s.nodeWSManager,
		validatorManager, im, stats, true)
	// set a shorted delay for tests
	s.server.wsConnDelayOnErr = 10 * time.Millisecond

	s.eg.Go(func() error {
		s.server.intentsManager.CleanupExpiredSolutions(ctx)
		return nil
	})
	s.eg.Go(func() error {
		return s.feedManager.Start(ctx)
	})
	s.eg.Go(s.server.Run)

	dialer := websocket.DefaultDialer
	headers := make(http.Header)

	// pass - same account for server and client
	dummyAuthHeader := "Z3c6c2VjcmV0" //gw:secret
	headers.Set("Authorization", dummyAuthHeader)
	ws, _, err := dialer.Dial(s.wsURL, headers)
	s.Require().NoError(err)

	// reusing the same ws connection for all tests
	s.conn = ws
}

func (s *wsSuite) SetupTest() {
	s.clearWSProviderStats()
	s.markAllPeersWithSyncStatus(blockchain.Synced)
}

func (s *wsSuite) TearDownSuite() {
	s.cancel()
	s.server.Shutdown()
	s.NoError(s.eg.Wait())
}

func (s *wsSuite) assertSubscribe(filter string) (string, string) {
	subscribeMsg := s.writeMsgToWsAndReadResponse([]byte(filter), nil)
	clientRes := s.getClientResponse(subscribeMsg)
	subscriptionID := fmt.Sprintf("%v", clientRes.Result)

	s.Require().True(s.feedManager.SubscriptionExists(subscriptionID))

	return fmt.Sprintf(
		`{"id": 1, "method": "unsubscribe", "params": ["%v"]}`,
		subscriptionID,
	), subscriptionID
}

func (s *wsSuite) writeMsgToWsAndReadResponse(msg []byte, expectedErr *websocket.CloseError) (response []byte) {
	return writeMsgToWsAndReadResponse(s.T(), s.conn, msg, expectedErr)
}

func writeMsgToWsAndReadResponse(t *testing.T, conn *websocket.Conn, msg []byte, expectedErr *websocket.CloseError) (response []byte) {
	err := conn.WriteMessage(websocket.TextMessage, msg)
	require.NoError(t, err)
	_, response, err = conn.ReadMessage()
	assert.True(t, (expectedErr == nil && err == nil) || (expectedErr != nil && err != nil))
	return response
}

type clientResponse struct {
	Jsonrpc string      `json:"JSONRPC"`
	ID      string      `json:"id"`
	Result  interface{} `json:"result"`
	Error   interface{} `json:"error"`
}

func (s *wsSuite) getClientResponse(msg []byte) (cr clientResponse) {
	return getClientResponse(s.T(), msg)
}

func getClientResponse(t *testing.T, msg []byte) (cr clientResponse) {
	res := clientResponse{}
	err := json.Unmarshal(msg, &res)
	require.NoError(t, err)
	return res
}

func (s *wsSuite) getClientSubscribeResponseParams() json.RawMessage {
	_, message, err := s.conn.ReadMessage()
	s.Require().NoError(err)
	var req jsonrpc2.Request
	err = json.Unmarshal(message, &req)
	s.Require().NoError(err)
	s.Require().NotNil(req.Params)

	return *req.Params
}

func (s *wsSuite) clearWSProviderStats() {
	for _, wsProvider := range s.nodeWSManager.Providers() {
		wsProvider.(*eth.MockWSProvider).ResetCounters()
	}
}

func (s *wsSuite) markAllPeersWithSyncStatus(status blockchain.NodeSyncStatus) {
	for _, peer := range s.blockchainPeers {
		s.nodeWSManager.UpdateNodeSyncStatus(peer, status)
	}
}

type mockAccountService struct{}

func (s *mockAccountService) Authorize(accountID types.AccountID, _ string, _ bool, _ bool, _ string) (sdnmessage.Account, error) {
	var err error
	if accountID == "d" {
		err = errAuth
	}
	return accountIDToAccountModel[accountID], err
}
