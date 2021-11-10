package sdnmessage

import (
	"fmt"
	"github.com/bloXroute-Labs/gateway/types"
)

// Peer represents a peer returned by the SDN for a node to connect to
type Peer struct {
	AssigningShortIds bool         `json:"assigning_short_ids"`
	Attributes        Attributes   `json:"attributes"`
	Idx               int64        `json:"idx"`
	IP                string       `json:"ip"`
	IsInternalGateway bool         `json:"is_internal_gateway"`
	NodeID            types.NodeID `json:"node_id"`
	NodeType          string       `json:"node_type"`
	NonSslPort        int64        `json:"non_ssl_port"`
	Port              int64        `json:"port"`
	PrivateNetwork    bool
}

// Attributes - contains peer attributes
type Attributes struct {
	Continent        string      `json:"continent"`
	Country          string      `json:"country"`
	NodePublicKey    interface{} `json:"node_public_key"`
	Region           string      `json:"region"`
	PlatformProvider string      `json:"platform_provider"`
	PrivateNode      bool        `json:"private_node"`
}

// Matches return if a given peer is equal on ip/port/nodeID fields (generally good enough for an equals-type comparison)
func (p Peer) Matches(ip string, port int64, nodeID types.NodeID) bool {
	return p.IP == ip && p.Port == port && p.NodeID == nodeID
}

// String returns a formatted representation of the peer model
func (p Peer) String() string {
	return fmt.Sprintf("%v@%v:%v", p.NodeID, p.IP, p.Port)
}

// Peers represents a list of Peer from the SDN
type Peers []Peer

// Contains returns if the given peer is in the peer list
func (pl Peers) Contains(p Peer) bool {
	for _, peer := range pl {
		if peer.NodeID == p.NodeID && peer.IP == p.IP && peer.Port == p.Port {
			return true
		}
	}
	return false
}

// ContainsPeerID returns if the given peer ID matches one of the peers in the peer list
func (pl Peers) ContainsPeerID(peerID types.NodeID) bool {
	for _, peer := range pl {
		if peer.NodeID == peerID {
			return true
		}
	}
	return false
}
