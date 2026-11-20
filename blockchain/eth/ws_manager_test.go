package eth

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

type nilPeerWSProvider struct {
	MockWSProvider
}

func (n *nilPeerWSProvider) BlockchainPeer() interface{} {
	return nil
}

func TestWSManager_UpdateNodeSyncStatus_NilPeer(t *testing.T) {
	peerEndpoint := types.NodeEndpoint{IP: "127.0.0.1", Port: 8546}
	peersInfo := []network.PeerInfo{}

	wsManager := NewEthWSManager(peersInfo, func(s string, endpoint types.NodeEndpoint, d time.Duration) blockchain.WSProvider {
		return &nilPeerWSProvider{
			MockWSProvider: MockWSProvider{
				endpoint:   peerEndpoint,
				syncStatus: blockchain.Unsynced,
			},
		}
	}, time.Second, false)

	ethWSManager, ok := wsManager.(*WSManager)
	assert.True(t, ok)

	provider := &nilPeerWSProvider{
		MockWSProvider: MockWSProvider{
			endpoint:   peerEndpoint,
			syncStatus: blockchain.Unsynced,
		},
	}
	ethWSManager.wsProviders[peerEndpoint.IPPort()] = provider

	assert.NotPanics(t, func() {
		ethWSManager.UpdateNodeSyncStatus(peerEndpoint, blockchain.Synced)
	})
	assert.True(t, ethWSManager.Synced())
}

func TestWSManager_SetBlockchainPeer_NilSafe(t *testing.T) {
	peerEndpoint := types.NodeEndpoint{IP: "127.0.0.1", Port: 8546}
	peersInfo := []network.PeerInfo{}

	wsManager := NewEthWSManager(peersInfo, NewMockWSProvider, time.Second, false)
	ethWSManager, ok := wsManager.(*WSManager)
	assert.True(t, ok)

	assert.NotPanics(t, func() {
		assert.False(t, ethWSManager.SetBlockchainPeer(nil))
		assert.False(t, ethWSManager.SetBlockchainPeer("not-a-peer"))
	})
	assert.False(t, ethWSManager.UnsetBlockchainPeer(peerEndpoint))
}

func TestWSManager_Synced_ConcurrentSafe(t *testing.T) {
	peerEndpoint := types.NodeEndpoint{IP: "127.0.0.1", Port: 8546}
	peersInfo := []network.PeerInfo{}

	wsManager := NewEthWSManager(peersInfo, func(s string, endpoint types.NodeEndpoint, d time.Duration) blockchain.WSProvider {
		return NewMockWSProvider(s, endpoint, d)
	}, time.Second, false)

	ethWSManager, ok := wsManager.(*WSManager)
	assert.True(t, ok)
	ethWSManager.wsProviders[peerEndpoint.IPPort()] = NewMockWSProvider("ws://127.0.0.1:8546", peerEndpoint, time.Second)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Concurrently read Synced()
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = ethWSManager.Synced()
				}
			}
		}()
	}

	// Concurrently write status via UpdateNodeSyncStatus
	for i := 0; i < 50; i++ {
		ethWSManager.UpdateNodeSyncStatus(peerEndpoint, blockchain.Synced)
		ethWSManager.UpdateNodeSyncStatus(peerEndpoint, blockchain.Unsynced)
	}

	close(stop)
	wg.Wait()
}
