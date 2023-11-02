package types

// ConnectionType type of connection between cloudServices and gateway
type ConnectionType string

// ConnectionType enumeration
const (
	WebSocketConnType ConnectionType = "ws"
	GRPCConnType      ConnectionType = "grpc"
)
