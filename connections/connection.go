package connections

import (
	"time"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

//go:generate mockgen -destination ../../bxgateway/test/mock/mock_sdnhttp.go -package mock github.com/bloXroute-Labs/bxcommon-go/sdnsdk SDNHTTP

// ConnectionDetails interface of base details for all connections
type ConnectionDetails interface {
	GetNodeID() bxtypes.NodeID
	GetPeerIP() string
	GetVersion() string
	GetPeerPort() int64
	GetPeerEnode() string
	GetLocalPort() int64
	GetAccountID() bxtypes.AccountID
	GetNetworkNum() bxtypes.NetworkNum
	GetConnectedAt() time.Time
	GetCapabilities() types.CapabilityFlags
	GetConnectionType() bxtypes.NodeType
	GetConnectionState() string

	IsLocalGEO() bool
	IsInitiator() bool
	IsSameRegion() bool
	IsPrivateNetwork() bool
}

// ConnDetails is base details for all connections
type ConnDetails struct{}

// GetNodeID return node ID
func (b ConnDetails) GetNodeID() bxtypes.NodeID { return "" }

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
func (b ConnDetails) GetAccountID() bxtypes.AccountID { return "" }

// GetNetworkNum gets the message network number
func (b ConnDetails) GetNetworkNum() bxtypes.NetworkNum { return types.AllNetworkNum }

// GetConnectedAt gets ttime of connection
func (b ConnDetails) GetConnectedAt() time.Time { return time.Time{} }

// GetCapabilities return capabilities
func (b ConnDetails) GetCapabilities() types.CapabilityFlags { return 0 }

// GetConnectionType returns type of the connection
func (b ConnDetails) GetConnectionType() bxtypes.NodeType { return bxtypes.Blockchain }

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
func IsCustomerGateway(connectionType bxtypes.NodeType, accountID bxtypes.AccountID) bool {
	return connectionType&bxtypes.ExternalGateway != 0 && accountID != bxtypes.BloxrouteAccountID
}

// IsBloxrouteGateway indicates if the connected gateway belongs to bloxroute
func IsBloxrouteGateway(connectionType bxtypes.NodeType, accountID bxtypes.AccountID) bool {
	return connectionType&bxtypes.Gateway != 0 && accountID == bxtypes.BloxrouteAccountID
}

// IsGateway indicates if the connection is a gateway
func IsGateway(connectionType bxtypes.NodeType) bool {
	return connectionType&bxtypes.Gateway != 0
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
func IsCloudAPI(connectionType bxtypes.NodeType) bool {
	return connectionType&bxtypes.CloudAPI != 0
}

// IsLocalRegion indicates if the connection is a GW or a cloud-api
func IsLocalRegion(connectionType bxtypes.NodeType) bool {
	return IsCloudAPI(connectionType) || IsGateway(connectionType)
}

// IsAPISocket indicates if the connection is api-socket
func IsAPISocket(connectionType bxtypes.NodeType) bool {
	return connectionType&bxtypes.APISocket != 0
}

// IsRelay indicates if the connection is a relay type
func IsRelay(connectionType bxtypes.NodeType) bool {
	return connectionType&bxtypes.RelayProxy != 0
}

// IsGrpc indicates if the connection is a gRPC type
func IsGrpc(connectionType bxtypes.NodeType) bool {
	return connectionType&bxtypes.GRPC != 0
}
