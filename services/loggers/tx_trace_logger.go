package loggers

import (
	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// TxTrace is used to generate log records for TxTrace
type TxTrace interface {
	Log(hash types.SHA256Hash, source connections.Conn)
}

type noStats struct {
}

func (noStats) Log(hash types.SHA256Hash, source connections.Conn) {
}

type txTrace struct {
	logger *log.Logger
}

// NewTxTrace is used to create TxTrace logger
func NewTxTrace(txTraceLogger *log.Logger) TxTrace {
	if txTraceLogger == nil {
		return noStats{}
	}
	return txTrace{logger: txTraceLogger}
}

func (tt txTrace) Log(hash types.SHA256Hash, source connections.Conn) {
	var sourceName string
	if source.GetConnectionType() == bxtypes.Blockchain {
		sourceName = "Blockchain"
	} else {
		sourceName = "BDN"
	}
	tt.logger.Trace().Msgf("%v - %v %v", hash, sourceName, source.GetPeerIP())
}
