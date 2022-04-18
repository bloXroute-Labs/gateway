package connections

import (
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/bloXroute-Labs/gateway/utils"
	"time"
)

// Info represents various information fields about the connection.
type Info struct {
	NodeID          types.NodeID
	AccountID       types.AccountID
	PeerIP          string
	PeerPort        int64
	PeerEnode       string
	LocalPort       int64 // either the local listening server port, or 0 for outbound connections
	ConnectionType  utils.NodeType
	ConnectionState string // TODO: flag?
	NetworkNum      types.NetworkNum
	FromMe          bool
	LocalGEO        bool
	PrivateNetwork  bool
	Capabilities    types.CapabilityFlags
	Version         string
	SameRegion      bool
	ConnectedAt     time.Time
}

// IsCustomerGateway indicates whether the connected gateway belongs to a customer
func (ci Info) IsCustomerGateway() bool {
	return ci.ConnectionType&(utils.ExternalGateway|utils.GatewayGo) != 0 && ci.AccountID != types.BloxrouteAccountID
}

// IsBloxrouteGateway indicates if the connected gateway belongs to bloxroute
func (ci Info) IsBloxrouteGateway() bool {
	return ci.ConnectionType&utils.Gateway != 0 && ci.AccountID == types.BloxrouteAccountID
}

// IsGateway indicates if the connection is a gateway
func (ci Info) IsGateway() bool {
	return ci.ConnectionType&utils.Gateway != 0
}

// IsMevMinerGateway indicates if the connection is a mev-miner gateway
func (ci Info) IsMevMinerGateway() bool {
	return ci.Capabilities&types.CapabilityMEVMiner != 0
}

// IsMevBuilderGateway indicates if the connection is a mev-builder gateway
func (ci Info) IsMevBuilderGateway() bool {
	return ci.Capabilities&types.CapabilityMEVBuilder != 0
}

// IsBDN indicates if the connection is a BDN gateway
func (ci Info) IsBDN() bool {
	return ci.Capabilities&types.CapabilityBDN != 0
}

// IsCloudAPI indicates if the connection is a cloud-api
func (ci Info) IsCloudAPI() bool {
	return ci.ConnectionType&utils.CloudAPI != 0
}

// IsLocalRegion indicates if the connection is a GW or a cloud-api
func (ci Info) IsLocalRegion() bool {
	return ci.IsCloudAPI() || ci.IsGateway()
}

// IsRelayTransaction indicates if the connection is a transaction relay
func (ci Info) IsRelayTransaction() bool {
	return ci.ConnectionType&utils.RelayTransaction != 0
}

// IsRelayProxy indicates if the connection is a relay proxy
func (ci Info) IsRelayProxy() bool {
	return ci.ConnectionType&utils.RelayProxy != 0
}

// IsRelay indicates if the connection is a relay type
func (ci Info) IsRelay() bool {
	return ci.ConnectionType&utils.RelayProxy != 0 || ci.ConnectionType&utils.RelayTransaction != 0 || ci.ConnectionType&utils.RelayBlock != 0
}

// IsPrivateNetwork indicates of the peer connection is over a private network (CEN)
func (ci Info) IsPrivateNetwork() bool {
	return ci.PrivateNetwork
}

// IsLocalGEO indicates if the peer is form the same GEO as we (China vs non-China)
func (ci Info) IsLocalGEO() bool {
	return ci.LocalGEO
}

// IsSameRegion indicates if the peer is from the same region as we (us-east1, eu-west1, ...)
func (ci Info) IsSameRegion() bool {
	return ci.SameRegion
}
