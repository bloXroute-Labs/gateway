package sdnmessage

import (
	"github.com/bloXroute-Labs/gateway/bxgateway/types"
)

// NodeEventType represents a type of node event being reported to the SDN
type NodeEventType string

// NodeEventType enumerations
const (
	NeOnline              NodeEventType = "ONLINE"
	NeOffline             NodeEventType = "OFFLINE"
	NePeerConnEstablished NodeEventType = "PEER_CONN_ESTABLISHED"
	NePeerConnClosed      NodeEventType = "PEER_CONN_CLOSED"
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
func NewNodeConnectionEvent(peerID types.NodeID) NodeEvent {
	return NodeEvent{
		NodeID:    peerID,
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
