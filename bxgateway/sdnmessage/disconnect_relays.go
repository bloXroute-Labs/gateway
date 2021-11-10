package sdnmessage

import (
	"github.com/bloXroute-Labs/gateway/bxgateway/types"
)

// DisconnectRelays represents the set of connections for the relay to disconnect, as sent down
// from bxapi
type DisconnectRelays = []types.NodeID
