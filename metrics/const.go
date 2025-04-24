package metrics

// These tags are used to tag very basic spans
const (
	AccountID                   = "account.id"
	AccountTier                 = "account.tier"
	UUID                        = "uuid"
	RPCMethod                   = "cloudapi.rpc.method"
	Protocol                    = "cloudapi.rpc.protocol"
	RPCForwarded                = "cloudapi.rpc.forwarded"
	RPCError                    = "cloudapi.rpc.error"
	RPCStatusCode               = "cloudapi.rpc.status_code"
	HTTPStatusCode              = "cloudapi.http.status_code"
	RequestID                   = "cloudapi.api.request_id"
	ForwardingRequestFromMaster = "cloudapi.forwarding.request_from_master_node"

	Status = "status"
	Ok     = "ok"
	Error  = "error"

	// RPCError is very specific error details.
	//
	// DataDog supports two status for traces: Ok and Error. Error means something unexpected happened:
	// panic, internal server error so on.
	// Other status codes are OK for DD:
	//
	// for example, our endpoints may return non-5xx status code, which means the application works as we expected.
	// But in fact there is something related to our rpc api: authorization failure, calling unknown rpc method, etc.
	// We should use this tag to enrich traces with some descriptive errors.
	//
	// Another case is when we call RPC on remote services: it might return 400 what is taken as Error by the http.Client wrapper.
	// But in fact we are fine with this status code: the service works as we expect, and it shouldn't be taken as an Error in datadog terms.
	// We can use RPCError to enrich traces with rpc response.
)

const (
	// Service is the current service name
	Service = "cloudapi"
)

// Basic operations related to handling incoming API requests
const (
	OperationWSReadClientRequest = "cloudapi.api.read_ws_request"
	OperationProcessRPC          = "cloudapi.api.handle_rpc"
)

// Other type of operations
const (
	OperationForwardBundleToBuilders        = "cloudapi.forward_to_builders"
	OperationForwardRPCRequestToMasterNodes = "cloudapi.forward_to_master_nodes"
)

// Tags related to MevBlockEvent parameters
const (
	MevBlockEventRelayIPTag = "mev_block_event.relay_ip"
)

// Tags related to builder parameters
const (
	BuilderName     = "builder.name"
	BuilderEndpoint = "builder.endpoint"
	BlockNumber     = "block.number"
)

// Tags related to blockchain network
const (
	NetworkNum               = "blockchain.network_num"
	BlockChainNetwork        = "blockchain.network"
	BlockchainNetworkChainID = "blockchain.network_chain_id"
)

// Tags related to Bundle parameters
const (
	BundleHash = "bundle.hash"
)

// Tags related to feed parameters
const (
	FeedName = "cloudapi.feed.name"
)

const (
	// BLXRForwardedHeader is a hjeader used to identify if rpc request is original or forwarded
	BLXRForwardedHeader = "X-BLXR-Forwarded"
)
