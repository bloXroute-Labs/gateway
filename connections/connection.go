package connections

import (
	"time"

	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

// ConnectionDetails interface of base details for all connections
type ConnectionDetails interface {
	GetNodeID() types.NodeID
	GetPeerIP() string
	GetVersion() string
	GetPeerPort() int64
	GetPeerEnode() string
	GetLocalPort() int64
	GetAccountID() types.AccountID
	GetNetworkNum() types.NetworkNum
	GetConnectedAt() time.Time
	GetCapabilities() types.CapabilityFlags
	GetConnectionType() utils.NodeType
	GetConnectionState() string

	IsLocalGEO() bool
	IsInitiator() bool
	IsSameRegion() bool
	IsPrivateNetwork() bool
}

// ConnDetails is base details for all connections
type ConnDetails struct{}

// GetNodeID return node ID
func (b ConnDetails) GetNodeID() types.NodeID { return "" }

// GetPeerIP return peer IP
func (b ConnDetails) GetPeerIP() string { return "" }

// GetVersion return version
func (b ConnDetails) GetVersion() string { return "" }

// GetPeerPort return peer port
func (b ConnDetails) GetPeerPort() int64 { return 0 }

// GetPeerEnode return peer enode
func (b ConnDetails) GetPeerEnode() string { return "" }

// GetLocalPort return local port
func (b ConnDetails) GetLocalPort() int64 { return 0 }

// GetAccountID return account ID (default empty)
func (b ConnDetails) GetAccountID() types.AccountID { return "" }

// GetNetworkNum gets the message network number
func (b ConnDetails) GetNetworkNum() types.NetworkNum { return types.AllNetworkNum }

// GetConnectedAt gets ttime of connection
func (b ConnDetails) GetConnectedAt() time.Time { return time.Time{} }

// GetCapabilities return capabilities
func (b ConnDetails) GetCapabilities() types.CapabilityFlags { return 0 }

// GetConnectionType returns type of the connection
func (b ConnDetails) GetConnectionType() utils.NodeType { return utils.Blockchain }

// GetConnectionState returns state of the connection
func (b ConnDetails) GetConnectionState() string { return "" }

// IsLocalGEO indicates if the peer is form the same GEO as we (China vs non-China)
func (b ConnDetails) IsLocalGEO() bool { return false }

// IsInitiator returns whether this node initiated the connection
func (b ConnDetails) IsInitiator() bool { return false }

// IsSameRegion indicates if the peer is from the same region as we (us-east1, eu-west1, ...)
func (b ConnDetails) IsSameRegion() bool { return false }

// IsPrivateNetwork indicates of the peer connection is over a private network (CEN)
func (b ConnDetails) IsPrivateNetwork() bool { return false }

// IsCustomerGateway indicates whether the connected gateway belongs to a customer
func IsCustomerGateway(connectionType utils.NodeType, accountID types.AccountID) bool {
	return connectionType&utils.ExternalGateway != 0 && accountID != types.BloxrouteAccountID
}

// IsBloxrouteGateway indicates if the connected gateway belongs to bloxroute
func IsBloxrouteGateway(connectionType utils.NodeType, accountID types.AccountID) bool {
	return connectionType&utils.Gateway != 0 && accountID == types.BloxrouteAccountID
}

// IsGateway indicates if the connection is a gateway
func IsGateway(connectionType utils.NodeType) bool {
	return connectionType&utils.Gateway != 0
}

// IsMevBuilderGateway indicates if the connection is a mev-builder gateway
func IsMevBuilderGateway(capabilities types.CapabilityFlags) bool {
	return capabilities&types.CapabilityMEVBuilder != 0
}

// IsBlockchainRPCEnabled indicates if the connection is enabled web3 bridge
func IsBlockchainRPCEnabled(capabilities types.CapabilityFlags) bool {
	return capabilities&types.CapabilityBlockchainRPCEnabled != 0
}

// IsNoBlocks indicates if the connection has no blocks flag enabled
func IsNoBlocks(capabilities types.CapabilityFlags) bool {
	return capabilities&types.CapabilityNoBlocks != 0
}

// IsCloudAPI indicates if the connection is a cloud-api
func IsCloudAPI(connectionType utils.NodeType) bool {
	return connectionType&utils.CloudAPI != 0
}

// IsLocalRegion indicates if the connection is a GW or a cloud-api
func IsLocalRegion(connectionType utils.NodeType) bool {
	return IsCloudAPI(connectionType) || IsGateway(connectionType)
}

// IsAPISocket indicates if the connection is api-socket
func IsAPISocket(connectionType utils.NodeType) bool {
	return connectionType&utils.APISocket != 0
}

// IsRelay indicates if the connection is a relay type
func IsRelay(connectionType utils.NodeType) bool {
	return connectionType&utils.RelayProxy != 0
}

// IsGrpc indicates if the connection is a gRPC type
func IsGrpc(connectionType utils.NodeType) bool {
	return connectionType&utils.GRPC != 0
}
