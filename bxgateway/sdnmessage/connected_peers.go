package sdnmessage

import (
	"github.com/bloXroute-Labs/gateway/bxgateway/types"
)

// ConnectedPeer represents info about a connected peer to a relay
type ConnectedPeer struct {
	NodeID          types.NodeID     `json:"node_id"`
	ExternalIP      string           `json:"external_ip"`
	ConnectionType  string           `json:"connection_type"`
	ConnectionState string           `json:"connection_state"`
	NetworkNum      types.NetworkNum `json:"network_num"`
	FromMe          bool             `json:"from_me"`
}

// ConnectedPeers represents both the request from bxapi to fetch the relay's connected peers,
// as well as the response back containing said peers
type ConnectedPeers struct {
	RequestID      string          `json:"request_id"`
	ConnectedPeers []ConnectedPeer `json:"connected_peers"`
}
