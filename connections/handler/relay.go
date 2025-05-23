package handler

import (
	"sync/atomic"

	"github.com/bloXroute-Labs/bxcommon-go/cert"
	"github.com/bloXroute-Labs/bxcommon-go/clock"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// Relay represents a connection to a relay Node
type Relay struct {
	*BxConn
	networks      *sdnmessage.BlockchainNetworks
	syncDoneCount uint32
	endpoint      types.NodeEndpoint
}

// NewOutboundRelay builds a new connection to a relay Node
func NewOutboundRelay(node connections.BxListener,
	sslCerts *cert.SSLCerts, relayIP string, relayPort int64, nodeID bxtypes.NodeID, relayType bxtypes.NodeType,
	networks *sdnmessage.BlockchainNetworks, localGEO bool, privateNetwork bool, clock clock.Clock,
	sameRegion bool) *Relay {
	return NewRelay(node,
		func() (connections.Socket, error) {
			return connections.NewTLS(relayIP, int(relayPort), sslCerts)
		},
		sslCerts, relayIP, relayPort, nodeID, relayType, networks, localGEO, privateNetwork, connections.LocalInitiatedPort, clock,
		sameRegion)
}

// NewInboundRelay builds a relay connection from a socket event initiated by a remote relay node
func NewInboundRelay(node connections.BxListener,
	socket connections.Socket, sslCerts *cert.SSLCerts, relayIP string, nodeID bxtypes.NodeID,
	relayType bxtypes.NodeType, networks *sdnmessage.BlockchainNetworks,
	localGEO bool, privateNetwork bool, localPort int64, clock clock.Clock,
	sameRegion bool) *Relay {
	return NewRelay(node,
		func() (connections.Socket, error) {
			return socket, nil
		},
		sslCerts, relayIP, connections.RemoteInitiatedPort, nodeID, relayType, networks, localGEO, privateNetwork, localPort, clock,
		sameRegion)
}

// NewRelay should only be called from test cases or NewOutboundRelay. It allows specifying a particular connect function for the SSL socket. However, in essentially all usages this should not be necessary as any node will initiate a connection to the relay, and as such should just use the default connect function to open a new socket.
func NewRelay(node connections.BxListener,
	connect func() (connections.Socket, error), sslCerts *cert.SSLCerts, relayIP string, relayPort int64,
	nodeID bxtypes.NodeID, relayType bxtypes.NodeType, networks *sdnmessage.BlockchainNetworks,
	localGEO bool, privateNetwork bool, localPort int64, clock clock.Clock,
	sameRegion bool) *Relay {
	if networks == nil {
		log.Panicf("TxStore sync: networks not provided. Please provide empty list of networks")
	}
	r := &Relay{
		networks: networks,
		endpoint: types.NodeEndpoint{
			IP:   relayIP,
			Port: int(relayPort),
		},
	}
	r.BxConn = NewBxConn(node, connect, r, sslCerts, relayIP, relayPort, nodeID, relayType,
		true, localGEO, privateNetwork, localPort, clock, sameRegion)
	return r
}

// NodeEndpoint return the blockchain connection endpoint
func (r *Relay) NodeEndpoint() types.NodeEndpoint {
	return r.endpoint
}

// ProcessMessage handles messages received on the relay connection, delegating to the BxListener when appropriate
func (r *Relay) ProcessMessage(msgBytes bxmessage.MessageBytes) {
	var err error

	msgType := msgBytes.BxType()
	msg := msgBytes.Raw()
	if msgType != bxmessage.TxType {
		r.Log().Tracef("processing message %v, msg len %v", msgType, len(msg))
	}
	switch msgType {

	case bxmessage.TxType:
		tx := &bxmessage.Tx{}
		_ = tx.Unpack(msg, r.Protocol())
		tx.SetReceiveTime(msgBytes.ReceiveTime())
		tx.SetReceiveStats(msgBytes.WaitingDuration(), msgBytes.ChannelPosition())
		_ = r.Node.HandleMsg(tx, r, connections.RunForeground)
	case bxmessage.HelloType:
		r.BxConn.ProcessMessage(msgBytes)
		r.syncDoneCount = 0

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
		r.BxConn.ProcessMessage(msgBytes)
	}

	if err != nil {
		r.Log().Errorf("encountered error processing message %v: %v", msgType, err)
	}
}
