package eth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

// WSManager implements the blockchain.WSManager interface for Ethereum
type WSManager struct {
	wsProviders  map[string]blockchain.WSProvider
	lock         sync.Mutex
	syncStatus   blockchain.NodeSyncStatus
	syncStatusCh chan blockchain.NodeSyncStatus
	log          *log.Entry
}

// NewEthWSManager - returns a new instance of WSManager
func NewEthWSManager(blockchainPeersInfo []network.PeerInfo, newWS func(string, types.NodeEndpoint, time.Duration) blockchain.WSProvider, timeout time.Duration, enableBlockchainRPC bool) blockchain.WSManager {
	var wsManager WSManager
	wsManager.wsProviders = make(map[string]blockchain.WSProvider)
	for _, peerInfo := range blockchainPeersInfo {
		if peerInfo.EthWSURI == "" {
			continue
		} else if peerInfo.Enode == nil {
			// if ws uri provided but no enode, we connect to the ws only if web3 bridge enabled
			if enableBlockchainRPC {
				wsProvider := newWS(peerInfo.EthWSURI, types.NodeEndpoint{IP: "", Port: 0}, timeout)
				wsProvider.UpdateSyncStatus(blockchain.Synced)
				wsManager.wsProviders[wsProvider.BlockchainPeerEndpoint().IPPort()] = wsProvider
				go wsProvider.Dial()
			}
			continue
		}

		peerEndpoint := types.NodeEndpoint{IP: peerInfo.Enode.IP().String(), Port: peerInfo.Enode.TCP()}
		wsProvider := newWS(peerInfo.EthWSURI, peerEndpoint, timeout)
		wsManager.wsProviders[wsProvider.BlockchainPeerEndpoint().IPPort()] = wsProvider
	}
	wsManager.syncStatus = blockchain.Unsynced
	wsManager.syncStatusCh = make(chan blockchain.NodeSyncStatus, 1)
	wsManager.log = log.WithFields(log.Fields{
		"component": "wsmanager",
		"gid":       utils.GetGID(),
	})

	return &wsManager
}

// Synced indicates if any ws provider nodes are synced
func (m *WSManager) Synced() bool {
	return m.syncStatus == blockchain.Synced
}

// Provider returns the WSProvider corresponding to the NodeEndpoint
func (m *WSManager) Provider(peerEndpoint *types.NodeEndpoint) (blockchain.WSProvider, bool) {
	if peerEndpoint == nil {
		return nil, false
	}
	wsProvider, ok := m.wsProviders[peerEndpoint.IPPort()]
	if !ok {
		return nil, false
	}
	return wsProvider, true
}

// SyncedProvider returns a synced WSProvider
func (m *WSManager) SyncedProvider() (blockchain.WSProvider, bool) {
	for _, wsProvider := range m.wsProviders {
		if wsProvider.SyncStatus() == blockchain.Synced {
			return wsProvider, true
		}
	}
	return nil, false
}

// ProviderWithBlock returns a WSProvider that has the blockNumber
// If preferredEndpoint is not nil, it will try to return a WSProvider that matches the endpoint
// If preferredEndpoint is nil, it will return the first WSProvider that has the blockNumber
func (m *WSManager) ProviderWithBlock(preferredEndpoint *types.NodeEndpoint, blockNumber uint64) (blockchain.WSProvider, bool) {
	if !m.Synced() {
		return nil, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), bxgateway.EthFetchBlockDeadlineInterval)
	defer cancel()

	if preferredEndpoint != nil {
		provider, ok := m.syncedPreferredProvider(preferredEndpoint)
		if ok {
			if err := m.waitProviderBlock(ctx, provider, blockNumber); err != nil {
				log.Warningf("failed to wait for block %v from %v: %v", blockNumber, provider.BlockchainPeerEndpoint(), err)
				return nil, false
			}

			log.Debugf("found blockchain provider %v", provider.BlockchainPeerEndpoint().IPPort())

			return provider, true
		}
	}

	provider, err := m.firstSyncedProviderWithBlock(ctx, blockNumber)
	if err != nil {
		log.Warningf("failed to find synced provider for block %v: %v", blockNumber, err)
		return nil, false
	}

	return provider, true
}

func (m *WSManager) syncedPreferredProvider(preferredEndpoint *types.NodeEndpoint) (blockchain.WSProvider, bool) {
	nodeWS, ok := m.Provider(preferredEndpoint)
	if !ok || nodeWS.SyncStatus() != blockchain.Synced {
		return nil, false
	}
	return nodeWS, ok
}

func (m *WSManager) waitProviderBlock(ctx context.Context, provider blockchain.WSProvider, blockNumber uint64) error {
	for {
		select {
		case <-ctx.Done():
			return errors.New("deadline exceeded")
		default:
			response, err := provider.FetchBlock([]interface{}{fmt.Sprintf("0x%x", blockNumber), false}, blockchain.RPCOptions{RetryAttempts: bxgateway.MaxEthOnBlockCallRetries, RetryInterval: bxgateway.EthOnBlockCallRetrySleepInterval})
			if err != nil {
				return err
			}

			if response != nil {
				return nil
			}

			time.Sleep(bxgateway.EthFetchBlockCallRetrySleepInterval)
		}
	}
}

func (m *WSManager) firstSyncedProviderWithBlock(ctx context.Context, blockNumber uint64) (blockchain.WSProvider, error) {
	providersChan := make(chan blockchain.WSProvider)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // stop all goroutines if one of them finds a provider

	providers := make([]blockchain.WSProvider, 0)
	for _, provider := range m.Providers() {
		if provider.SyncStatus() != blockchain.Synced {
			continue
		}
		providers = append(providers, provider)
	}

	if len(providers) == 0 {
		return nil, errors.New("no synced providers")
	}

	for _, provider := range providers {
		go func(provider blockchain.WSProvider) {
			if err := m.waitProviderBlock(ctx, provider, blockNumber); err != nil {
				log.Debugf("waitWSProviderBlock failed to wait block %v from %v: %v", blockNumber, provider.BlockchainPeerEndpoint(), err)
				return
			}

			select {
			case providersChan <- provider:
			default:
			}
		}(provider)
	}

	select {
	case provider := <-providersChan:
		return provider, nil
	case <-ctx.Done():
		return nil, errors.New("no provider with block found")
	}
}

// Providers returns map of NodeEndpoint to WSProvider
func (m *WSManager) Providers() map[string]blockchain.WSProvider {
	return m.wsProviders
}

// SetBlockchainPeer sets the blockchain peer for corresponding ws provider
func (m *WSManager) SetBlockchainPeer(peer interface{}) bool {
	peerEndpoint := peer.(*Peer).endpoint.IPPort()
	m.log.Debugf("WSManager: SetBlockchainPeer %v  process %v", peerEndpoint, utils.GetGID())
	for endpoint, ws := range m.wsProviders {
		if endpoint == peerEndpoint {
			ws.SetBlockchainPeer(peer)
			return true
		}
	}
	return false
}

// UnsetBlockchainPeer unsets the blockchain peer for corresponding ws provider
func (m *WSManager) UnsetBlockchainPeer(peerEndpoint types.NodeEndpoint) bool {
	m.log.Debugf("WSManager: UnsetBlockchainPeer %v process %v", peerEndpoint.String(), utils.GetGID())
	for endpoint, ws := range m.wsProviders {
		if endpoint == peerEndpoint.IPPort() {
			ws.UnsetBlockchainPeer()
			return true
		}
	}
	return false
}

// ValidRPCCallMethods returns valid Ethereum RPC command methods
func (m *WSManager) ValidRPCCallMethods() []string {
	return validRPCCallMethods
}

// ValidRPCCallPayloadFields returns valid Ethereum RPC method payload fields
func (m *WSManager) ValidRPCCallPayloadFields() []string {
	return validRPCCallPayloadFields
}

// RequiredPayloadFieldsForRPCMethod returns the valid payload fields for the provided Ethereum RPC command method
func (m *WSManager) RequiredPayloadFieldsForRPCMethod(method string) ([]string, bool) {
	requiredFields, ok := commandMethodsToRequiredPayloadFields[method]
	return requiredFields, ok
}

// ConstructRPCCallPayload returns payload used in RPC call
func (m *WSManager) ConstructRPCCallPayload(method string, callParams map[string]string, tag string) ([]interface{}, error) {
	switch method {
	case "eth_call":
		payload := []interface{}{callParams, tag}
		return payload, nil
	case "eth_blockNumber":
		return []interface{}{}, nil
	case "eth_getStorageAt":
		payload := []interface{}{callParams["address"], callParams["pos"], tag}
		return payload, nil
	case "eth_getBalance":
		fallthrough
	case "eth_getCode":
		fallthrough
	case "eth_getTransactionCount":
		payload := []interface{}{callParams["address"], tag}
		return payload, nil
	default:
		return nil, fmt.Errorf("unexpectedly failed to match method %v", method)
	}
}

// UpdateNodeSyncStatus sends update on NodeSyncStatus channel if overall sync status has changed
func (m *WSManager) UpdateNodeSyncStatus(nodeEndpoint types.NodeEndpoint, syncStatus blockchain.NodeSyncStatus) {
	m.lock.Lock()
	defer m.lock.Unlock()

	wsProvider, ok := m.wsProviders[nodeEndpoint.IPPort()]
	if !ok {
		m.log.Errorf("received request to update node sync status for unknown websockets provider %v  process %v", nodeEndpoint, utils.GetGID())
		return
	}
	if wsProvider.SyncStatus() == syncStatus {
		return
	}
	wsProvider.UpdateSyncStatus(syncStatus)

	peer := wsProvider.BlockchainPeer().(*Peer)
	if peer != nil {
		if wsProvider.SyncStatus() == blockchain.Synced {
			peer.RequestConfirmations = false
		} else {
			peer.RequestConfirmations = true
		}
	}

	// send Synced status update if overall status was Unsynced & node becomes synced
	if syncStatus == blockchain.Synced && m.syncStatus == blockchain.Unsynced {
		m.syncStatus = blockchain.Synced
		select {
		case m.syncStatusCh <- blockchain.Synced:
		default:
			m.log.Errorf("unable to update node sync status to synced: channel full  process %v", utils.GetGID())
		}
		return
	}

	// send Unsynced status update if overall status was Synced & node becomes unsynced & no other nodes are synced
	if syncStatus == blockchain.Unsynced && m.syncStatus == blockchain.Synced {
		for _, wsp := range m.wsProviders {
			if wsp.SyncStatus() == blockchain.Synced {
				return
			}
		}
		m.syncStatus = blockchain.Unsynced
		select {
		case m.syncStatusCh <- blockchain.Unsynced:
		default:
			m.log.Errorf("unable to update node sync status to unsynced: channel full  process %v", utils.GetGID())
		}
	}
}

// ReceiveNodeSyncStatusUpdate provides a channel to receive NodeSyncStatus updates
func (m *WSManager) ReceiveNodeSyncStatusUpdate() chan blockchain.NodeSyncStatus {
	return m.syncStatusCh
}
