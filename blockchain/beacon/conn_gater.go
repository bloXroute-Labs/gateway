package beacon

import (
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/libp2p/go-libp2p/core/control"
	"github.com/libp2p/go-libp2p/core/network"

	libp2pPeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// InterceptPeerDial tests whether we're permitted to Dial the specified peer.
func (n *Node) InterceptPeerDial(_ libp2pPeer.ID) bool {
	return true
}

// InterceptAddrDial tests whether we're permitted to dial the specified
// multiaddr for the given peer.
func (n *Node) InterceptAddrDial(_ libp2pPeer.ID, _ ma.Multiaddr) bool {
	return true
}

// InterceptAccept checks whether the incidental inbound connection is allowed.
func (n *Node) InterceptAccept(network.ConnMultiaddrs) bool {
	return true
}

// InterceptSecured tests whether a given connection, now authenticated,
// is allowed.
func (n *Node) InterceptSecured(_ network.Direction, _ libp2pPeer.ID, _ network.ConnMultiaddrs) bool {
	return true
}

// InterceptUpgraded tests whether a fully capable connection is allowed.
func (n *Node) InterceptUpgraded(conn network.Conn) (bool, control.DisconnectReason) {
	if conn.Stat().Direction == network.DirOutbound {
		return true, 0
	}

	inboundConns := utils.Filter(n.host.Network().Conns(), func(c network.Conn) bool {
		return c.Stat().Direction == network.DirInbound
	})

	if len(inboundConns) >= n.inboundLimit {
		n.log.WithFields(log.Fields{
			"inboundConnections": len(inboundConns),
			"inboundLimit":       n.inboundLimit,
			"peerID":             conn.RemotePeer(),
			"peerAddr":           conn.RemoteMultiaddr(),
		}).Infof("connection rejected: inbound connection limit reached")
		return false, control.DisconnectReason(1)
	}

	if n.enableAnyIncomingConnection {
		return true, 0
	}

	if n.trustedPeers.contains(conn.RemotePeer()) {
		return true, 0
	}

	n.log.WithFields(log.Fields{
		"peerID":   conn.RemotePeer(),
		"peerAddr": conn.RemoteMultiaddr(),
	}).Infof("connection rejected: peer is not trusted")
	return false, control.DisconnectReason(1)
}
