package servers

import (
	"context"
	"testing"
	"time"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/sync/errgroup"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/feed"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/mock"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

func TestManageServers(t *testing.T) {
	networkNum := bxtypes.NetworkNum(1)
	nm := sdnmessage.NodeModel{}
	bxConfig := &config.Bx{
		NoStats:          true,
		Config:           &log.Config{},
		NodeType:         bxtypes.Gateway,
		GRPC:             &config.GRPC{Enabled: true, Port: 9100},
		WebsocketEnabled: true,
		WebsocketPort:    28333,
		HTTPPort:         9090,
	}

	ctrl := gomock.NewController(t)
	sdn := mock.NewMockSDNHTTP(ctrl)
	sdn.EXPECT().NetworkNum().Return(networkNum).AnyTimes()
	sdn.EXPECT().NodeModel().Return(&nm).AnyTimes()
	sdn.EXPECT().NodeID().Return(bxtypes.NodeID("node_id")).AnyTimes()

	wsManager := &mockNodeWSManager{
		syncChan: make(chan blockchain.NodeSyncStatus, 1),
	}
	fm := feed.NewManager(sdn, nil, sdnmessage.Account{}, nil, 1, false)

	clientHandler := NewClientHandler(nil, bxConfig, nil, sdn, nil, nil,
		nil, services.NewNoOpSubscriptionServices(), wsManager, nil,
		time.Now(), "", fm,
		statistics.NoStats{}, nil,
		false, "", "",
	)

	ctx, cancel := context.WithCancel(context.Background())

	eg, gCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return clientHandler.ManageServers(gCtx, true)
	})

	test.WaitServerStopped(t, "localhost:28333")
	test.WaitServerStopped(t, "localhost:9100")
	test.WaitServerStopped(t, "localhost:9090")

	// test if the first sync status is 'unsynced'
	wsManager.syncChan <- blockchain.Unsynced

	wsManager.syncChan <- blockchain.Synced

	test.WaitServerStarted(t, "localhost:28333")
	test.WaitServerStarted(t, "localhost:9100")
	test.WaitServerStarted(t, "localhost:9090")

	wsManager.syncChan <- blockchain.Unsynced

	test.WaitServerStopped(t, "localhost:28333")
	test.WaitServerStopped(t, "localhost:9100")
	test.WaitServerStopped(t, "localhost:9090")

	wsManager.syncChan <- blockchain.Synced

	test.WaitServerStarted(t, "localhost:28333")
	test.WaitServerStarted(t, "localhost:9100")
	test.WaitServerStarted(t, "localhost:9090")

	clientHandler.shutdownServers()

	cancel()
	require.NoError(t, eg.Wait())
}

type mockNodeWSManager struct {
	syncChan chan blockchain.NodeSyncStatus
}

func (m *mockNodeWSManager) ReceiveNodeSyncStatusUpdate() chan blockchain.NodeSyncStatus {
	return m.syncChan
}

func (m *mockNodeWSManager) Synced() bool { return false }

func (m *mockNodeWSManager) GetSyncedWSProvider(*types.NodeEndpoint) (blockchain.WSProvider, bool) {
	return nil, false
}

func (m *mockNodeWSManager) UpdateNodeSyncStatus(types.NodeEndpoint, blockchain.NodeSyncStatus) {}

func (m *mockNodeWSManager) SyncedProvider() (blockchain.WSProvider, bool) { return nil, false }

func (m *mockNodeWSManager) Provider(*types.NodeEndpoint) (blockchain.WSProvider, bool) {
	return nil, false
}
func (m *mockNodeWSManager) Providers() map[string]blockchain.WSProvider { return nil }

func (m *mockNodeWSManager) ProviderWithBlock(*types.NodeEndpoint, uint64) (blockchain.WSProvider, error) {
	return nil, nil
}

func (m *mockNodeWSManager) SetBlockchainPeer(interface{}) bool { return false }

func (m *mockNodeWSManager) UnsetBlockchainPeer(types.NodeEndpoint) bool { return false }

func (m *mockNodeWSManager) ValidRPCCallMethods() []string { return nil }

func (m *mockNodeWSManager) ValidRPCCallPayloadFields() []string { return nil }

func (m *mockNodeWSManager) RequiredPayloadFieldsForRPCMethod(string) ([]string, bool) {
	return nil, false
}

func (m *mockNodeWSManager) ConstructRPCCallPayload(string, map[string]string, string) ([]interface{}, error) {
	return nil, nil
}
