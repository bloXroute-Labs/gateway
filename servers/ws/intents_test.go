package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/sync/errgroup"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth/test"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/feed"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/test/fixtures"
	"github.com/bloXroute-Labs/gateway/v2/test/mock"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

func (s *wsSuite) TestSubmitIntent() {
	s.T().Run("equal dapp and sender", func(_ *testing.T) {
		intent, err := json.Marshal(s.createIntentRequest())
		s.Require().NoError(err)
		reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_submit_intent", "params": %s}`, string(intent))
		msg := s.writeMsgToWsAndReadResponse([]byte(reqPayload), nil)
		clientRes := s.getClientResponse(msg)
		s.Require().Nil(clientRes.Error)

		b, err := json.Marshal(clientRes.Result)
		s.Require().NoError(err)
		var res rpcIntentResponse
		err = json.Unmarshal(b, &res)
		s.Require().NoError(err)
		crypto.Keccak256Hash()
		s.Assert().NotEmpty(res.IntentID)
	})
	s.T().Run("different dapp and sender", func(_ *testing.T) {
		ir := s.createIntentRequest()
		ir.DappAddress = "0x097399a35cfC20efE5FcD2e9b1d892884DAAd642"
		intent, err := json.Marshal(ir)
		s.Require().NoError(err)
		reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_submit_intent", "params": %s}`, string(intent))
		msg := s.writeMsgToWsAndReadResponse([]byte(reqPayload), nil)
		clientRes := s.getClientResponse(msg)
		s.Require().Nil(clientRes.Error)

		b, err := json.Marshal(clientRes.Result)
		s.Require().NoError(err)
		var res rpcIntentResponse
		err = json.Unmarshal(b, &res)
		s.Require().NoError(err)
		s.Assert().NotEmpty(res.IntentID)
	})
	s.T().Run("wrong hash", func(_ *testing.T) {
		req := s.createIntentRequest()
		req.Intent = []byte("wrong intent")
		intent, err := json.Marshal(req)
		s.Require().NoError(err)
		reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_submit_intent", "params": %s}`, string(intent))
		msg := s.writeMsgToWsAndReadResponse([]byte(reqPayload), nil)
		clientRes := s.getClientResponse(msg)
		s.Require().NotNil(clientRes.Error)
		errResp, ok := clientRes.Error.(map[string]interface{})
		s.Require().True(ok)
		data, ok := errResp["data"].(string)
		s.Require().True(ok)
		s.Assert().Contains(data, "hash mismatch")
	})
}

func (s *wsSuite) TestIntentsSubscribe() {
	unsubscribeMessage, subscriptionID := s.assertSubscribe(`{ "id": "2", "method": "subscribe", "params": [ "userIntentSolutionsFeed", { "dapp_address": "0x097399a35cfC20efE5FcD2e9b1d892884DAAd642", "hash": [183,2,62,89,100,47,176,195,131,57,253,105,100,82,169,64,29,188,156,99,54,36,235,232,173,157,89,149,61,184,76,66], "signature": [219,205,179,107,60,59,38,125,46,45,156,10,134,243,162,72,64,73,94,242,204,254,129,50,70,146,3,160,193,121,120,157,15,160,110,100,156,30,241,77,145,65,209,160,191,14,68,103,79,163,72,155,116,2,11,23,11,167,240,4,119,184,40,176,0]}]}`)

	solution := &types.UserIntentSolution{
		ID:            uuid.New().String(),
		SolverAddress: "0x097399a35cfC20efE5FcD2e9b1d892884DAAd642",
		DappAddress:   "0x097399a35cfC20efE5FcD2e9b1d892884DAAd642",
		IntentID:      uuid.New().String(),
		Solution:      []byte{71, 108, 111, 114, 121, 32, 116, 111, 32, 85, 107, 114, 97, 105, 110, 101, 33},
		Hash:          []byte{183, 2, 62, 89, 100, 47, 176, 195, 131, 57, 253, 105, 100, 82, 169, 64, 29, 188, 156, 99, 54, 36, 235, 232, 173, 157, 89, 149, 61, 184, 76, 66},
		Signature:     []byte{219, 205, 179, 107, 60, 59, 38, 125, 46, 45, 156, 10, 134, 243, 162, 72, 64, 73, 94, 242, 204, 254, 129, 50, 70, 146, 3, 160, 193, 121, 120, 157, 15, 160, 110, 100, 156, 30, 241, 77, 145, 65, 209, 160, 191, 14, 68, 103, 79, 163, 72, 155, 116, 2, 11, 23, 11, 167, 240, 4, 119, 184, 40, 176, 0},
		Timestamp:     time.Now(),
	}

	s.feedManager.Notify(types.NewUserIntentSolutionNotification(solution))

	time.Sleep(time.Millisecond)

	params := s.getClientSubscribeResponseParams()

	var m userIntentSolutionResponse
	err := json.Unmarshal(params, &m)
	s.Require().NoError(err)

	s.Assert().Equal(m.Subscription, subscriptionID)
	s.Assert().Equal(m.Result.IntentID, solution.IntentID)
	s.Assert().Equal(m.Result.SolutionID, solution.ID)
	s.Assert().Equal(m.Result.IntentSolution, solution.Solution)

	s.writeMsgToWsAndReadResponse([]byte(unsubscribeMessage), nil)
	time.Sleep(time.Millisecond)

	s.Assert().False(s.feedManager.SubscriptionExists(subscriptionID))
	s.handlePingRequest()
}

func (s *wsSuite) TestIntentsSubscribeWithFilter() {
	s.T().Run("happy path", func(_ *testing.T) {
		unsubscribeMessage, subscriptionID := s.assertSubscribe(`{ "id": "2", "method": "subscribe", "params": [ "userIntentFeed", { "filters":"dapp_address=0x097399a35cfC20efE5FcD2e9b1d892884DAAd642", "solver_address": "0xB0c6E56246F863E4d8708B0f7B928FD6c90CE935", "hash": [104,224,229,103,124,84,82,19,198,59,72,51,180,175,56,179,174,29,110,175,234,66,83,231,106,237,238,130,196,37,186,234], "signature": [175,215,153,143,90,43,18,79,37,7,117,34,76,133,228,194,23,45,41,250,218,204,151,163,142,220,189,139,87,104,56,81,92,130,120,198,254,208,161,5,183,224,250,172,80,3,239,40,41,172,166,80,187,253,185,45,87,140,49,117,213,142,155,229,1]}]}`)

		intent := &types.UserIntent{
			ID:            uuid.New().String(),
			DappAddress:   "0x097399a35cfC20efE5FcD2e9b1d892884DAAd642",
			SenderAddress: "0x097399a35cfC20efE5FcD2e9b1d892884DAAd642",
		}

		s.feedManager.Notify(types.NewUserIntentNotification(intent))

		time.Sleep(time.Millisecond)

		params := s.getClientSubscribeResponseParams()

		var m userIntentResponse
		err := json.Unmarshal(params, &m)
		s.Require().NoError(err)

		s.Assert().Equal(m.Subscription, subscriptionID)
		s.Assert().Equal(m.Result.IntentID, intent.ID)
		s.Assert().Equal(m.Result.DappAddress, intent.DappAddress)

		s.writeMsgToWsAndReadResponse([]byte(unsubscribeMessage), nil)
		time.Sleep(time.Millisecond)

		s.Assert().False(s.feedManager.SubscriptionExists(subscriptionID))
	})
	s.T().Run("invalid filter", func(_ *testing.T) {
		subscribeMsg := s.writeMsgToWsAndReadResponse([]byte(`{ "id": "2", "method": "subscribe", "params": [ "userIntentFeed", { "filters":"something=!0x097399a35cfC20efE5FcD2e9b1d892884DAAd642", "solver_address": "0xB0c6E56246F863E4d8708B0f7B928FD6c90CE935", "hash": [104,224,229,103,124,84,82,19,198,59,72,51,180,175,56,179,174,29,110,175,234,66,83,231,106,237,238,130,196,37,186,234], "signature": [175,215,153,143,90,43,18,79,37,7,117,34,76,133,228,194,23,45,41,250,218,204,151,163,142,220,189,139,87,104,56,81,92,130,120,198,254,208,161,5,183,224,250,172,80,3,239,40,41,172,166,80,187,253,185,45,87,140,49,117,213,142,155,229,1]}]}`), nil)
		clientRes := s.getClientResponse(subscribeMsg)
		s.Require().NotNil(clientRes.Error)
		errResp, ok := clientRes.Error.(map[string]interface{})
		s.Require().True(ok)
		data, ok := errResp["data"].(string)
		s.Require().True(ok)
		s.Assert().Contains(data, "error parsing Filters")
	})
}

func (s *wsSuite) TestGetSolutionsForIntent() {
	s.T().Run("happy path", func(_ *testing.T) {
		intentID := uuid.New().String()
		getIntent := s.createGetSolutionsRequest(intentID)
		intent, err := json.Marshal(getIntent)
		s.Require().NoError(err)
		reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_get_intent_solutions", "params": %s}`, string(intent))

		// add intent solutions to the cache
		s.server.intentsManager.AddIntentOfInterest(intentID)
		dAppAddress := s.generateRandomAddress()
		solutions := []bxmessage.IntentSolution{
			{
				ID:            uuid.New().String(),
				SolverAddress: s.generateRandomAddress(),
				IntentID:      intentID,
				Solution:      []byte("test intent solution 1"),
				Timestamp:     time.Now(),
				DappAddress:   dAppAddress,
			},
			{
				ID:            uuid.New().String(),
				SolverAddress: s.generateRandomAddress(),
				IntentID:      intentID,
				Solution:      []byte("test intent solution 2"),
				Timestamp:     time.Now(),
				DappAddress:   dAppAddress,
			},
		}
		s.server.intentsManager.AppendSolutionsForIntent(bxmessage.NewIntentSolutions(solutions))

		msg := s.writeMsgToWsAndReadResponse([]byte(reqPayload), nil)
		clientRes := s.getClientResponse(msg)

		b, err := json.Marshal(clientRes.Result)
		s.Require().NoError(err)

		var res rpcGetIntentSolutionsResponse
		err = json.Unmarshal(b, &res)
		s.Require().NoError(err)

		s.Require().Len(res, 2)
		s.Require().Equal(solutions[0].DappAddress, res[0].DappAddress)
		s.Require().Equal(solutions[1].DappAddress, res[1].DappAddress)
	})
	s.T().Run("no solutions", func(_ *testing.T) {
		intentID := uuid.New().String()
		getIntent := s.createGetSolutionsRequest(intentID)
		intent, err := json.Marshal(getIntent)
		s.Require().NoError(err)
		reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_get_intent_solutions", "params": %s}`, string(intent))

		msg := s.writeMsgToWsAndReadResponse([]byte(reqPayload), nil)
		clientRes := s.getClientResponse(msg)

		b, err := json.Marshal(clientRes.Result)
		s.Require().NoError(err)

		var res rpcGetIntentSolutionsResponse
		err = json.Unmarshal(b, &res)
		s.Require().NoError(err)

		s.Require().Len(res, 0)
	})
}

func (s *wsSuite) createIntentRequest() *jsonrpc.RPCSubmitIntentPayload {
	privKey, err := crypto.GenerateKey()
	s.Require().NoError(err)
	senderAddress := crypto.PubkeyToAddress(privKey.PublicKey).String()
	intent := []byte("test intent")
	intentHash := crypto.Keccak256Hash(intent).Bytes()
	intentSignature, err := crypto.Sign(intentHash, privKey)
	s.Require().NoError(err)

	return &jsonrpc.RPCSubmitIntentPayload{
		DappAddress:   senderAddress,
		SenderAddress: senderAddress,
		Intent:        intent,
		Hash:          intentHash,
		Signature:     intentSignature,
	}
}

func (s *wsSuite) createGetSolutionsRequest(intentID string) *jsonrpc.RPCGetIntentSolutionsPayload {
	privKey, err := crypto.GenerateKey()
	s.Require().NoError(err)
	dappAddress := crypto.PubkeyToAddress(privKey.PublicKey).String()
	data := []byte(dappAddress + intentID)
	intentHash := crypto.Keccak256Hash(data).Bytes()
	intentSignature, err := crypto.Sign(intentHash, privKey)
	s.Require().NoError(err)

	return &jsonrpc.RPCGetIntentSolutionsPayload{
		IntentID:            intentID,
		DappOrSenderAddress: dappAddress,
		Hash:                intentHash,
		Signature:           intentSignature,
	}
}

func (s *wsSuite) generateRandomAddress() string {
	privKey, err := crypto.GenerateKey()
	s.Require().NoError(err)
	return crypto.PubkeyToAddress(privKey.PublicKey).String()
}

func TestClientHandlerAuth(t *testing.T) {
	for _, allowIntroductoryTierAccess := range []bool{true, false} {
		t.Run(fmt.Sprintf("allowIntroductoryTierAccess-%t", allowIntroductoryTierAccess), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			eg, gCtx := errgroup.WithContext(ctx)

			ctl := gomock.NewController(t)
			sdn := mock.NewMockSDNHTTP(ctl)
			sdn.EXPECT().FetchCustomerAccountModel(types.AccountID("gw")).Return(accountIDToAccountModel["gw"], nil).AnyTimes()
			sdn.EXPECT().NodeID().Return(types.NodeID("nodeID")).AnyTimes()
			sdn.EXPECT().NetworkNum().Return(types.NetworkNum(5)).AnyTimes()
			sdn.EXPECT().AccountModel().Return(accountIDToAccountModel["gw"]).AnyTimes()
			sdn.EXPECT().GetQuotaUsage(gomock.AnyOf("a", "b", "c", "i", "gw")).DoAndReturn(func(accountID string) (*connections.QuotaResponseBody, error) {
				res := connections.QuotaResponseBody{
					AccountID:   accountID,
					QuotaFilled: 1,
					QuotaLimit:  2,
				}

				return &res, nil
			}).AnyTimes()

			stats := statistics.NoStats{}

			feedManager := feed.NewManager(sdn, services.NewNoOpSubscriptionServices(),
				accountIDToAccountModel["gw"], stats, types.NetworkNum(5), true)

			im := services.NewIntentsManager()
			as := &mockAccountService{}

			g := bxmock.MockBxListener{}

			cfg := &config.Bx{
				EnableBlockchainRPC:         false,
				WebsocketHost:               localhost,
				WebsocketPort:               wsPort,
				ManageWSServer:              true,
				WebsocketTLSEnabled:         false,
				AllowIntroductoryTierAccess: allowIntroductoryTierAccess,
			}

			_, blockchainPeersInfo := test.GenerateBlockchainPeersInfo(3)

			nodeWSManager := eth.NewEthWSManager(blockchainPeersInfo, eth.NewMockWSProvider, bxgateway.WSProviderTimeout, false)

			server := NewWSServer(cfg, "", "", sdn, g, as, feedManager, nodeWSManager,
				nil, im, stats, true)
			server.wsConnDelayOnErr = 10 * time.Millisecond

			eg.Go(func() error {
				return feedManager.Start(gCtx)
			})
			eg.Go(server.Run)

			dialer := websocket.DefaultDialer
			headers := make(http.Header)

			dummyAuthHeader := "aTo2NTQzMjE=" // i:654321
			headers.Set("Authorization", dummyAuthHeader)
			ws, _, err := dialer.Dial(wsURL, headers)
			require.NoError(t, err)

			// ping
			_ = writeMsgToWsAndReadResponse(t, ws, []byte(`{"id": "1", "method": "ping"}`), nil)

			reqPayload := fmt.Sprintf(`{"jsonrpc": "2.0", "id": "1", "method": "eth_sendRawTransaction", "params": ["0x%v"]}`, fixtures.DynamicFeeTransaction)

			var msg []byte
			if allowIntroductoryTierAccess {
				msg = writeMsgToWsAndReadResponse(t, ws, []byte(reqPayload), nil)
				assert.Equal(t, `{"id":"1","error":{"code":-32001,"message":"Insufficient quota","data":"account must be enterprise / enterprise elite / ultra"},"jsonrpc":"2.0"}
`, string(msg))
				err = ws.WriteMessage(websocket.TextMessage, msg)
				assert.NoError(t, err)
				_, _, err = ws.ReadMessage()
				assert.Error(t, err, "connection should be closed by the server")
			} else {
				msg = writeMsgToWsAndReadResponse(t, ws, []byte(reqPayload), nil)
				assert.Equal(t, `{"id":"1","error":{"code":-32601,"message":"Invalid method","data":"got unsupported method name: eth_sendRawTransaction"},"jsonrpc":"2.0"}
`, string(msg))
				// client should be still connected
				msg = writeMsgToWsAndReadResponse(t, ws, []byte(reqPayload), nil)
				assert.Equal(t, `{"id":"1","error":{"code":-32601,"message":"Invalid method","data":"got unsupported method name: eth_sendRawTransaction"},"jsonrpc":"2.0"}
`, string(msg))
			}

			cancel()
			server.Shutdown()
			require.NoError(t, eg.Wait())
		})
	}
}
