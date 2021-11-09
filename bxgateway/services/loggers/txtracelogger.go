package loggers

import (
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/connections"
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/types"
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/utils"
	log "github.com/sirupsen/logrus"
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
	if source.Info().ConnectionType == utils.Blockchain {
		sourceName = "Blockchain"
	} else {
		sourceName = "BDN"
	}
	tt.logger.Tracef("%v - %v %v", hash, sourceName, source.Info().PeerIP)
}
