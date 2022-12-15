package sdnmessage

import (
	"fmt"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// NodeEventType represents a type of node event being reported to the SDN
type NodeEventType string

// NodeEventType enumerations
const (
	NeOnline                        NodeEventType = "ONLINE"
	NeOffline                       NodeEventType = "OFFLINE"
	NePeerConnEstablished           NodeEventType = "PEER_CONN_ESTABLISHED"
	NePeerConnClosed                NodeEventType = "PEER_CONN_CLOSED"
	NePeerConnDisabled              NodeEventType = "PEER_CONN_DISABLED"
	NeBlockchainNodeConnEstablished NodeEventType = "BLOCKCHAIN_NODE_CONN_ESTABLISHED"
	NeBlockchainNodeConnError       NodeEventType = "BLOCKCHAIN_NODE_CONN_ERR"
)

// NodeEvent represents a node event and its context being reported to the SDN
// In most cases, NodeID refers to the peer
type NodeEvent struct {
	NodeID    types.NodeID  `json:"node_id"`
	EventType NodeEventType `json:"event_type"`
	PeerIP    string        `json:"peer_ip"`
	PeerPort  int           `json:"peer_port"`
	Timestamp string        `json:"timestamp"`
	EventID   string        `json:"event_id"`
	Payload   string        `json:"payload"`
}

// NewNodeConnectionEvent returns an online NodeEvent for a peer.
func NewNodeConnectionEvent(peerID types.NodeID, networkNum types.NetworkNum) NodeEvent {
	return NodeEvent{
		NodeID:    peerID,
		Payload:   fmt.Sprint(networkNum),
		EventType: NePeerConnEstablished,
	}
}

// NewNodeDisconnectionEvent returns an offline NodeEvent for a peer.
func NewNodeDisconnectionEvent(peerID types.NodeID) NodeEvent {
	return NodeEvent{
		NodeID:    peerID,
		EventType: NePeerConnClosed,
	}
}

// NewNodeDisabledEvent returns a disabled NodeEvent for a peer.
func NewNodeDisabledEvent(peerID types.NodeID, reason string) NodeEvent {
	return NodeEvent{
		NodeID:    peerID,
		EventType: NePeerConnDisabled,
		Payload:   reason,
	}
}

// NewBlockchainNodeConnEstablishedEvent returns an established NodeEvent for a peer.
func NewBlockchainNodeConnEstablishedEvent(NodeID types.NodeID, peerIP string, peerPort int, timestamp string) NodeEvent {
	return NodeEvent{
		Timestamp: timestamp,
		NodeID:    NodeID,
		EventType: NeBlockchainNodeConnEstablished,
		PeerIP:    peerIP,
		PeerPort:  peerPort,
	}
}

// NewBlockchainNodeConnError returns an error NodeEvent for a peer.
func NewBlockchainNodeConnError(NodeID types.NodeID, peerIP string, peerPort int, timestamp string) NodeEvent {
	return NodeEvent{
		Timestamp: timestamp,
		NodeID:    NodeID,
		EventType: NeBlockchainNodeConnError,
		PeerIP:    peerIP,
		PeerPort:  peerPort,
	}
}
