package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/sdnsdk"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	"github.com/bloXroute-Labs/gateway/v2/metrics"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/sync/errgroup"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth/test"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/feed"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/test/fixtures"
	"github.com/bloXroute-Labs/gateway/v2/test/mock"
)

func TestAuthorization(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	eg, gCtx := errgroup.WithContext(ctx)

	ctl := gomock.NewController(t)
	sdn := mock.NewMockSDNHTTP(ctl)
	sdn.EXPECT().NodeID().Return(bxtypes.NodeID("nodeID")).AnyTimes()
	sdn.EXPECT().NetworkNum().Return(bxtypes.NetworkNum(5)).AnyTimes()
	sdn.EXPECT().AccountModel().Return(accountIDToAccountModel["gw"]).AnyTimes()
	sdn.EXPECT().GetQuotaUsage(gomock.AnyOf("a", "b", "c", "i", "gw")).DoAndReturn(func(accountID string) (*sdnsdk.QuotaResponseBody, error) {
		sdn.EXPECT().NodeID().Return(bxtypes.NodeID("nodeID")).AnyTimes()
		res := sdnsdk.QuotaResponseBody{
			AccountID:   accountID,
			QuotaFilled: 1,
			QuotaLimit:  2,
		}

		return &res, nil
	}).AnyTimes()

	stats := statistics.NoStats{}

	feedManager := feed.NewManager(sdn, services.NewNoOpSubscriptionServices(),
		accountIDToAccountModel["gw"], stats, bxtypes.NetworkNum(5), true, &metrics.NoOpExporter{})

	accService := &mockAccountService{}

	g := bxmock.MockBxListener{}

	cfg := &config.Bx{
		EnableBlockchainRPC: false,
		WebsocketHost:       localhost,
		WebsocketPort:       wsPort,
		ManageWSServer:      true,
		WebsocketTLSEnabled: false,
	}

	_, blockchainPeersInfo := test.GenerateBlockchainPeersInfo(3)

	nodeWSManager := eth.NewEthWSManager(blockchainPeersInfo, eth.NewMockWSProvider, bxgateway.WSProviderTimeout, false)

	server := NewWSServer(cfg, "", "", sdn, g, accService, feedManager, nodeWSManager, stats, true, nil, nil)
	server.wsConnDelayOnErr = 10 * time.Millisecond // set a shorted delay for tests

	eg.Go(func() error {
		feedManager.Start(gCtx)
		return nil
	})
	eg.Go(server.Run)

	dialer := websocket.DefaultDialer
	headers := make(http.Header)

	tests := []struct {
		name            string
		header          string
		wantErr         bool
		errMsg          string
		shouldCloseConn bool
	}{
		{
			name:            "different account for server and client",
			header:          "YToxMjM0NTY=", // a:123456
			wantErr:         true,
			errMsg:          "blxr_tx is not allowed when account authentication is different from the node account",
			shouldCloseConn: false,
		},
		{
			name:            "invalid header",
			header:          "invalid",
			wantErr:         true,
			shouldCloseConn: true,
		},
		{
			name:            "no header",
			header:          "",
			wantErr:         true,
			shouldCloseConn: true,
		},
		{
			name:            "sdn error",
			header:          "ZDo3ODkxMDEx", // d:7891011
			errMsg:          "some error",
			wantErr:         true,
			shouldCloseConn: true,
		},
		{
			name:            "same account for server and client",
			header:          "Z3c6c2VjcmV0", //gw:secret
			wantErr:         false,
			shouldCloseConn: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers.Set("Authorization", tt.header)
			ws, _, err := dialer.Dial(wsURL, headers)
			if err != nil {
				t.Errorf("dialer.Dial() error = %v", err)
			}

			reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s"}}`, fixtures.LegacyTransaction)
			err = ws.WriteMessage(websocket.TextMessage, []byte(reqPayload))
			require.NoError(t, err)
			_, msg, err := ws.ReadMessage()
			if tt.shouldCloseConn {
				require.Error(t, err)
				return
			}
			clientRes := getClientResponse(t, msg)
			if tt.wantErr {
				require.NotNil(t, clientRes.Error)
				cErrm, ok := clientRes.Error.(map[string]interface{})
				require.True(t, ok)

				if tt.errMsg != "" {
					require.Containsf(t, cErrm["data"].(string), tt.errMsg, "expected error message %s, got %s", tt.errMsg, cErrm["data"].(string))
				}
			}

			err = ws.WriteMessage(websocket.TextMessage, []byte(`{"id": "1", "method": "ping"}`))
			require.NoError(t, err)
			_, msg, err = ws.ReadMessage()
			require.NoError(t, err)
			clientRes = getClientResponse(t, msg)
			var b []byte
			b, err = json.Marshal(clientRes.Result)
			require.NoError(t, err)
			var res rpcPingResponse
			err = json.Unmarshal(b, &res)
			require.NoError(t, err)
			assert.NotEmpty(t, res.Pong)
		})
	}

	cancel()
	server.Shutdown()
	require.NoError(t, eg.Wait())
}
