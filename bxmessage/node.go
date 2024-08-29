package bxmessage

// PeerInfo represents information about connected peer
type PeerInfo struct {
	IP               string
	NodeID           string
	Type             string
	State            string
	Network          uint32
	Initiator        bool
	Protocol         uint32
	Capability       uint32
	Trusted          string
	AccountID        string
	Port             int64
	Disabled         bool
	MinUsRoundTrip   int64
	SlowTrafficCount int64
	MinUsFromPeer    int64
	MinUsToPeer      int64
}

// RelayConnectionInfo represents information about connected relay
type RelayConnectionInfo struct {
	Status      string
	ConnectedAt string
	Latency     *ConnectionLatency
}

// ConnectionLatency represents connection latencies form and to peer
type ConnectionLatency struct {
	MinMsFromPeer    int64
	MinMsToPeer      int64
	SlowTrafficCount int64
	MinMsRoundTrip   int64
}
