package handler

import (
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"sync/atomic"
)

// Relay represents a connection to a relay Node
type Relay struct {
	*BxConn
	networks      *sdnmessage.BlockchainNetworks
	sendSyncReq   bool
	syncDoneCount uint32
	endpoint      types.NodeEndpoint
}

// NewOutboundRelay builds a new connection to a relay Node
func NewOutboundRelay(node connections.BxListener,
	sslCerts *utils.SSLCerts, relayIP string, relayPort int64, nodeID types.NodeID, relayType utils.NodeType,
	usePQ bool, networks *sdnmessage.BlockchainNetworks, localGEO bool, privateNetwork bool, clock utils.Clock,
	sameRegion bool, sendSyncReq bool) *Relay {
	return NewRelay(node,
		func() (connections.Socket, error) {
			return connections.NewTLS(relayIP, int(relayPort), sslCerts)
		},
		sslCerts, relayIP, relayPort, nodeID, relayType, usePQ, networks, localGEO, privateNetwork, connections.LocalInitiatedPort, clock,
		sameRegion, sendSyncReq, false)
}

// NewInboundRelay builds a relay connection from a socket event initiated by a remote relay node
func NewInboundRelay(node connections.BxListener,
	socket connections.Socket, sslCerts *utils.SSLCerts, relayIP string, nodeID types.NodeID,
	relayType utils.NodeType, usePQ bool, networks *sdnmessage.BlockchainNetworks,
	localGEO bool, privateNetwork bool, localPort int64, clock utils.Clock,
	sameRegion bool, sendSyncReq bool) *Relay {
	return NewRelay(node,
		func() (connections.Socket, error) {
			return socket, nil
		},
		sslCerts, relayIP, connections.RemoteInitiatedPort, nodeID, relayType, usePQ, networks, localGEO, privateNetwork, localPort, clock,
		sameRegion, sendSyncReq, true)
}

// NewRelay should only be called from test cases or NewOutboundRelay. It allows specifying a particular connect function for the SSL socket. However, in essentially all usages this should not be necessary as any node will initiate a connection to the relay, and as such should just use the default connect function to open a new socket.
func NewRelay(node connections.BxListener,
	connect func() (connections.Socket, error), sslCerts *utils.SSLCerts, relayIP string, relayPort int64,
	nodeID types.NodeID, relayType utils.NodeType, usePQ bool, networks *sdnmessage.BlockchainNetworks,
	localGEO bool, privateNetwork bool, localPort int64, clock utils.Clock,
	sameRegion bool, sendSyncReq bool, inbound bool) *Relay {
	if networks == nil {
		log.Panicf("TxStore sync: networks not provided. Please provide empty list of networks")
	}
	r := &Relay{
		networks:    networks,
		sendSyncReq: sendSyncReq,
		endpoint: types.NodeEndpoint{
			IP:      relayIP,
			Port:    int(relayPort),
			Inbound: inbound,
		},
	}
	r.BxConn = NewBxConn(node, connect, r, sslCerts, relayIP, relayPort, nodeID, relayType,
		usePQ, true, localGEO, privateNetwork, localPort, clock, sameRegion)
	return r
}

// NodeEndpoint return the blockchain connection endpoint
func (r *Relay) NodeEndpoint() types.NodeEndpoint {
	return r.endpoint
}

// ProcessMessage handles messages received on the relay connection, delegating to the BxListener when appropriate
func (r *Relay) ProcessMessage(msg bxmessage.MessageBytes) {
	var err error

	msgType := msg.BxType()
	if msgType != bxmessage.TxType {
		r.Log().Tracef("processing message %v", msgType)
	}

	switch msgType {

	case bxmessage.TxType:
		tx := &bxmessage.Tx{}
		_ = tx.Unpack(msg, r.Protocol())
		_ = r.Node.HandleMsg(tx, r, connections.RunForeground)
	case bxmessage.HelloType:
		r.BxConn.ProcessMessage(msg)
		r.syncDoneCount = 0

		if !r.sendSyncReq {
			break
		}

		for _, network := range *r.networks {
			r.Log().Debugf("TxStore sync: requesting network %v", network.NetworkNum)
			syncReq := bxmessage.SyncReq{}
			syncReq.SetNetworkNum(network.NetworkNum)
			_ = r.Send(&syncReq)
		}

	case bxmessage.SyncTxsType:
		txs := &bxmessage.SyncTxsMessage{}
		err := txs.Unpack(msg, r.Protocol())
		if err != nil {
			r.Log().Errorf("unable to unpack SyncTxsMessage: %v. Closing connetion", err)
			r.Close(err.Error())
		}
		_ = r.Node.HandleMsg(txs, r, connections.RunBackground)
		// TODO: add txs to txservice
	case bxmessage.SyncDoneType:
		syncDone := &bxmessage.SyncDone{}
		_ = syncDone.Unpack(msg, r.Protocol())
		r.Log().Debugf("TxStore sync: done for network %v", syncDone.GetNetworkNum())
		atomic.AddUint32(&r.syncDoneCount, 1)
		if atomic.CompareAndSwapUint32(&r.syncDoneCount, uint32(len(*r.networks)), 0) {
			r.Log().Debugf("TxStore sync: done for %v networks", len(*r.networks))
			_ = r.Node.HandleMsg(syncDone, r, connections.RunBackground)
		}
	case bxmessage.RefreshBlockchainNetworkType:
		refresh := &bxmessage.RefreshBlockchainNetwork{}
		_ = refresh.Unpack(msg, r.Protocol())
		_ = r.Node.HandleMsg(refresh, r, connections.RunForeground)
	case bxmessage.TransactionsType:
		txs := &bxmessage.Txs{}
		err = txs.Unpack(msg, r.Protocol())
		if err != nil {
			break
		}

		err = r.Node.HandleMsg(txs, r, connections.RunForeground)
	case bxmessage.BlockTxsType:
	default:
		r.BxConn.ProcessMessage(msg)
	}

	if err != nil {
		r.Log().Errorf("encountered error processing message %v: %v", msgType, err)
	}
}
