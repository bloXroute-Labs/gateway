package servers

// RPCErrorCode represents an error condition while processing an RPC request
type RPCErrorCode int64

// RPCErrorCode types
const (
	// ParseError - json parse error
	ParseError RPCErrorCode = -32700

	// InvalidRequest - invalid request
	InvalidRequest RPCErrorCode = -32600

	// MethodNotFound - method not found
	MethodNotFound RPCErrorCode = -32601

	// InvalidParams - invalid params
	InvalidParams RPCErrorCode = -32602

	// InternalError - internal error
	InternalError RPCErrorCode = -32603

	// AccountIDError - invalid account ID
	AccountIDError RPCErrorCode = -32004
)

var errorMsg = map[RPCErrorCode]string{
	MethodNotFound: "Invalid method",
	InvalidParams:  "Invalid params",
	AccountIDError: "Invalid account ID",
	InternalError:  "Internal error",
}

// RPCRequestType represents the JSON-RPC methods that are callable
type RPCRequestType string

// RPCRequestType enumeration
const (
	RPCSubscribe   RPCRequestType = "subscribe"
	RPCUnsubscribe RPCRequestType = "unsubscribe"
	RPCTx          RPCRequestType = "blxr_tx"
	RPCPing        RPCRequestType = "ping"
	RPCMevSearcher RPCRequestType = "blxr_mev_searcher"
	RPCMevBuilder  RPCRequestType = "blxr_mev_builder"
)
