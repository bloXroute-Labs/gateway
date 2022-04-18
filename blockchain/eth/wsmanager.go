package eth

import (
	"fmt"
	"github.com/bloXroute-Labs/gateway/blockchain"
	"github.com/bloXroute-Labs/gateway/blockchain/network"
	log "github.com/bloXroute-Labs/gateway/logger"
	"github.com/bloXroute-Labs/gateway/types"
	"sync"
	"time"
)

// WSManager implements the blockchain.WSManager interface for Ethereum
type WSManager struct {
	wsProviders  map[string]blockchain.WSProvider
	lock         sync.Mutex
	syncStatus   blockchain.NodeSyncStatus
	syncStatusCh chan blockchain.NodeSyncStatus
}

// NewEthWSManager - returns a new instance of WSManager
func NewEthWSManager(blockchainPeersInfo []network.PeerInfo, newWS func(string, types.NodeEndpoint, time.Duration) blockchain.WSProvider, timeout time.Duration) blockchain.WSManager {
	var wsManager WSManager
	wsManager.wsProviders = make(map[string]blockchain.WSProvider)
	for _, peerInfo := range blockchainPeersInfo {
		if peerInfo.EthWSURI == "" {
			continue
		}
		peerEndpoint := types.NodeEndpoint{IP: peerInfo.Enode.IP().String(), Port: peerInfo.Enode.TCP()}
		wsProvider := newWS(peerInfo.EthWSURI, peerEndpoint, timeout)
		wsManager.wsProviders[wsProvider.BlockchainPeerEndpoint().IPPort()] = wsProvider
	}
	wsManager.syncStatus = blockchain.Unsynced
	wsManager.syncStatusCh = make(chan blockchain.NodeSyncStatus, 1)
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

// Providers returns map of NodeEndpoint to WSProvider
func (m *WSManager) Providers() map[string]blockchain.WSProvider {
	return m.wsProviders
}

// SetBlockchainPeer sets the blockchain peer for corresponding ws provider
func (m *WSManager) SetBlockchainPeer(peer interface{}) bool {
	peerEndpoint := peer.(*Peer).endpoint.IPPort()
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
		log.Errorf("received request to update node sync status for unknown websockets provider %v", nodeEndpoint)
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
			log.Error("unable to update node sync status to synced: channel full")
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
			log.Error("unable to update node sync status to unsynced: channel full")
		}
	}
}

// ReceiveNodeSyncStatusUpdate provides a channel to receive NodeSyncStatus updates
func (m *WSManager) ReceiveNodeSyncStatusUpdate() chan blockchain.NodeSyncStatus {
	return m.syncStatusCh
}
