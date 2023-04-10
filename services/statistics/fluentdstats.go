package statistics

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"time"
	"unsafe"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/fluent/fluent-logger-golang/fluent"
	uuid "github.com/satori/go.uuid"
)

const (
	// MaxTailByteValue is the highest number of bytes based on TailByByteCount
	MaxTailByteValue = uint16(math.MaxUint16)

	// TailByteCount is the number of bytes to consider in tx hash for writing events
	TailByteCount = unsafe.Sizeof(MaxTailByteValue)

	// DateFormat is an example to date time string format
	DateFormat = "2006-01-02T15:04:05.000000"
)

// Stats is used to generate STATS record for transactions
type Stats interface {
	AddBlockEvent(name string, source connections.Conn, blockHash, beaconBlockHash types.SHA256Hash, networkNum types.NetworkNum,
		sentPeers int, startTime time.Time, sentGatewayPeers int)
	AddTxsByShortIDsEvent(name string, source connections.Conn, txInfo *types.BxTransaction,
		shortID types.ShortID, sourceID types.NodeID, sentPeers int, sentGatewayPeers int,
		startTime time.Time, priority bxmessage.SendPriority, debugData interface{})
	AddGatewayBlockEvent(name string, source connections.Conn, blockHash, beaconBlockHash types.SHA256Hash, networkNum types.NetworkNum,
		sentPeers int, startTime time.Time, sentGatewayPeers int, originalSize int, compressSize int, shortIDsCount int, txsCount int, recoveredTxsCount int, block *types.BxBlock)
	LogSubscribeStats(subscriptionID *uuid.UUID, accountID types.AccountID, feedName types.FeedType, tierName sdnmessage.AccountTier,
		ip string, networkNum types.NetworkNum, feedInclude []string, feedFilter string, feedProject string)
	LogUnsubscribeStats(subscriptionID *uuid.UUID, feedName types.FeedType, networkNum types.NetworkNum, accountID types.AccountID, tierName sdnmessage.AccountTier)
}

// NoStats is used to generate empty stats
type NoStats struct {
}

// AddBlockEvent does nothing
func (NoStats) AddBlockEvent(name string, source connections.Conn, blockHash, beaconBlockHash types.SHA256Hash, networkNum types.NetworkNum,
	sentPeers int, startTime time.Time, sentGatewayPeers int) {
}

// AddGatewayBlockEvent does nothing
func (NoStats) AddGatewayBlockEvent(name string, source connections.Conn, blockHash, beaconBlockHash types.SHA256Hash, networkNum types.NetworkNum,
	sentPeers int, startTime time.Time, sentGatewayPeers int, originalSize int, compressSize int, shortIDsCount int, txsCount int, recoveredTxsCount int, block *types.BxBlock) {
}

// AddTxsByShortIDsEvent does nothing
func (NoStats) AddTxsByShortIDsEvent(name string, source connections.Conn, txInfo *types.BxTransaction,
	shortID types.ShortID, sourceID types.NodeID, sentPeers int, sentGatewayPeers int,
	startTime time.Time, priority bxmessage.SendPriority, debugData interface{}) {
}

// LogSubscribeStats does nothing
func (NoStats) LogSubscribeStats(subscriptionID *uuid.UUID, accountID types.AccountID, feedName types.FeedType, tierName sdnmessage.AccountTier,
	ip string, networkNum types.NetworkNum, feedInclude []string, feedFilter string, feedProject string) {
}

// LogUnsubscribeStats does nothing
func (NoStats) LogUnsubscribeStats(subscriptionID *uuid.UUID, feedName types.FeedType, networkNum types.NetworkNum, accountID types.AccountID, tierName sdnmessage.AccountTier) {
}

// FluentdStats struct that represents fluentd stats info
type FluentdStats struct {
	NodeID            types.NodeID
	FluentD           *fluent.Fluent
	Networks          map[types.NetworkNum]sdnmessage.BlockchainNetwork
	Lock              *sync.RWMutex
	logNetworkContent bool
}

// AddTxsByShortIDsEvent generates a fluentd STATS event
func (s FluentdStats) AddTxsByShortIDsEvent(name string, source connections.Conn, txInfo *types.BxTransaction,
	shortID types.ShortID, sourceID types.NodeID, sentPeers int, sentGatewayPeers int,
	startTime time.Time, priority bxmessage.SendPriority, debugData interface{}) {
	if txInfo == nil || !s.shouldLogEventForTx(txInfo) {
		return
	}

	now := time.Now()

	logic := EventLogicNone
	connectionType := source.GetConnectionType()
	notFromBDN := connections.IsGateway(connectionType) || connections.IsCloudAPI(connectionType)
	switch {
	// if from external source
	case name == "TxProcessedByRelayProxyFromPeer" && notFromBDN:
		logic = EventLogicPropagationStart | EventLogicSummary
	// if from bdn
	case name == "TxProcessedByRelayProxyFromPeer":
		logic = EventLogicPropagationEnd | EventLogicSummary
	}

	record := Record{
		Type: "BxTransaction",
		Data: txRecord{
			EventSubjectID:   txInfo.Hash().Format(false),
			EventLogic:       logic,
			NodeID:           s.NodeID,
			EventName:        name,
			NetworkNum:       txInfo.NetworkNum(),
			StartDateTime:    startTime.Format(DateFormat),
			SentGatewayPeers: sentGatewayPeers,
			ExtraData: txExtraData{
				MoreInfo:             fmt.Sprintf("source: %v - %v, priority %v, sent: %v (%v), duration: %v, debug data: %v", source, connectionType.FormatShortNodeType(), priority, sentPeers, sentGatewayPeers, now.Sub(startTime), debugData),
				ShortID:              shortID,
				NetworkNum:           txInfo.NetworkNum(),
				SourceID:             sourceID,
				IsCompactTransaction: len(txInfo.Content()) == 0,
				AlreadySeenContent:   false,
				ExistingShortIds:     txInfo.ShortIDs(),
			},
			EndDateTime: now.Format(DateFormat),
		},
	}
	s.LogToFluentD(record, now, "stats.transactions.events.p")
}

// AddBlockEvent generates a fluentd STATS event
func (s FluentdStats) AddBlockEvent(name string, source connections.Conn, blockHash, beaconBlockHash types.SHA256Hash, networkNum types.NetworkNum,
	sentPeers int, startTime time.Time, sentGatewayPeers int) {
	now := time.Now()

	record := Record{
		Type: "BlockInfo",
		Data: blockRecord{
			EventSubjectID:   blockHash.String(),
			EventLogic:       EventLogicNone,
			NodeID:           s.NodeID,
			EventName:        name,
			NetworkNum:       networkNum,
			SourceID:         source.GetNodeID(),
			StartDateTime:    startTime.Format(DateFormat),
			EndDateTime:      now.Format(DateFormat),
			SentGatewayPeers: sentGatewayPeers,
			ExtraData: blockExtraData{
				MoreInfo: fmt.Sprintf("source: %v - %v, sent: %v", source, source.GetConnectionType().FormatShortNodeType(), sentPeers),
			},
			BeaconBlockHash: beaconBlockHash.String(),
		},
	}
	s.LogToFluentD(record, now, "stats.blocks.events.p")
}

// AddGatewayBlockEvent add block event for the gateway
func (s FluentdStats) AddGatewayBlockEvent(name string, source connections.Conn, blockHash, beaconBlockHash types.SHA256Hash, networkNum types.NetworkNum,
	sentPeers int, startTime time.Time, sentGatewayPeers int, originalSize int, compressSize int, shortIDsCount int, txsCount int, recoveredTxsCount int, block *types.BxBlock) {
	now := time.Now()

	record := Record{
		Type: "GatewayBlockInfo",
		Data: gatewayBlockRecord{
			EventSubjectID:   blockHash.String(),
			EventLogic:       EventLogicNone,
			NodeID:           s.NodeID,
			EventName:        name,
			NetworkNum:       networkNum,
			SourceID:         source.GetNodeID(),
			StartDateTime:    startTime.Format(DateFormat),
			EndDateTime:      now.Format(DateFormat),
			SentGatewayPeers: sentGatewayPeers,
			ExtraData: blockExtraData{
				MoreInfo: fmt.Sprintf("source: %v - %v, sent: %v", source, source.GetConnectionType().FormatShortNodeType(), sentPeers),
			},
			OriginalSize:      originalSize,
			CompressSize:      compressSize,
			ShortIDsCount:     shortIDsCount,
			TxsCount:          txsCount,
			RecoveredTxsCount: recoveredTxsCount,
			BeaconBlockHash:   beaconBlockHash.String(),
		},
	}
	s.LogToFluentD(record, now, "stats.gateway.blocks.events.p")

	s.addBlockContent(name, networkNum, blockHash, block)
}

func (s FluentdStats) addBlockContent(name string, networkNum types.NetworkNum, blockHash types.SHA256Hash, blockContentInfo *types.BxBlock) {
	if !s.logNetworkContent {
		return
	}
	if name != "GatewayReceivedBlockFromBlockchainNode" && name != "GatewayProcessBlockFromBDN" {
		return
	}
	if blockContentInfo == nil {
		log.Errorf("can't addBlockContent for %v, networkNum %v, event %v - block is nil", blockHash, networkNum, name)
		return
	}

	now := time.Now()

	txs := make([]byte, 0, len(blockContentInfo.Txs))
	for _, tx := range blockContentInfo.Txs {
		txs = append(txs, tx.Content()...)
	}

	record := Record{
		Type: "NetworkEthContentBlock",
		Data: ethBlockContent{
			BlockHash:  blockHash.String(),
			NetworkNum: networkNum,
			Header:     base64.StdEncoding.EncodeToString(blockContentInfo.Header),
			Txs:        base64.StdEncoding.EncodeToString(txs),
			Trailer:    base64.StdEncoding.EncodeToString(blockContentInfo.Trailer),
		},
	}

	s.LogToFluentD(record, now, "network_content.block.stats")
}

// NewStats is used to create transaction STATS logger
func NewStats(fluentDEnabled bool, fluentDHost string, nodeID types.NodeID, networks *sdnmessage.BlockchainNetworks, logNetworkContent bool) Stats {
	if !fluentDEnabled {
		return NoStats{}
	}

	return newStats(fluentDHost, nodeID, networks, logNetworkContent)
}

// LogToFluentD log info to the fluentd
func (s FluentdStats) LogToFluentD(record interface{}, ts time.Time, logName string) {
	d := LogRecord{
		Level:     "STATS",
		Name:      logName,
		Instance:  s.NodeID,
		Msg:       record,
		Timestamp: ts.Format(DateFormat),
	}

	err := s.FluentD.EncodeAndPostData("bx.go.log", ts, d)
	if err != nil {
		log.Errorf("Failed to send STATS to fluentd - %v", err)
	}
}

func newStats(fluentdHost string, nodeID types.NodeID, sdnHTTPNetworks *sdnmessage.BlockchainNetworks, logNetworkContent bool) Stats {
	fluentlogger, err := fluent.New(fluent.Config{
		FluentHost:    fluentdHost,
		FluentPort:    24224,
		MarshalAsJSON: true,
		Async:         true,
	})

	if err != nil {
		log.Panic()
	}

	t := FluentdStats{
		NodeID:            nodeID,
		FluentD:           fluentlogger,
		Networks:          make(map[types.NetworkNum]sdnmessage.BlockchainNetwork),
		Lock:              &sync.RWMutex{},
		logNetworkContent: logNetworkContent,
	}

	for _, network := range *sdnHTTPNetworks {
		t.UpdateBlockchainNetwork(*network)
	}

	return t
}

// UpdateBlockchainNetwork - updates the blockchainNetwork object
func (s FluentdStats) UpdateBlockchainNetwork(network sdnmessage.BlockchainNetwork) {
	s.Lock.Lock()
	s.Networks[network.NetworkNum] = network
	s.Lock.Unlock()
}

func (s FluentdStats) shouldLogEventForTx(txInfo *types.BxTransaction) bool {
	txHashPercentage := s.getLogPercentageByHash(txInfo.NetworkNum())
	if txHashPercentage <= 0 {
		return false
	}
	txHash := txInfo.Hash()
	lastBytesValue := binary.BigEndian.Uint16(txHash[types.SHA256HashLen-TailByteCount:])
	probabilityValue := float64(lastBytesValue) * float64(100) / float64(MaxTailByteValue)
	return probabilityValue <= txHashPercentage
}

func (s FluentdStats) getLogPercentageByHash(networkNum types.NetworkNum) float64 {
	s.Lock.RLock()
	defer s.Lock.RUnlock()
	blockchainNetwork, ok := s.Networks[networkNum]
	if !ok {
		return 0
	}
	return blockchainNetwork.TxPercentToLogByHash
}

// LogSubscribeStats generates a fluentd STATS event
func (s FluentdStats) LogSubscribeStats(subscriptionID *uuid.UUID, accountID types.AccountID, feedName types.FeedType, tierName sdnmessage.AccountTier,
	ip string, networkNum types.NetworkNum, feedInclude []string, feedFilter string, feedProject string) {
	now := time.Now()
	record := subscribeRecord{
		Type:           "subscriptions",
		Event:          "created",
		SubscriptionID: subscriptionID,
		AccountID:      accountID,
		Tier:           tierName,
		IP:             ip,
		NetworkNum:     networkNum,
		FeedName:       feedName,
		FeedInclude:    feedInclude,
		FeedFilters:    feedFilter,
		FeedProject:    feedProject,
	}
	s.LogToFluentD(record, now, "stats.subscriptions.events")
}

// LogUnsubscribeStats generates a fluentd STATS event
func (s FluentdStats) LogUnsubscribeStats(subscriptionID *uuid.UUID, feedName types.FeedType, networkNum types.NetworkNum, accountID types.AccountID, tierName sdnmessage.AccountTier) {
	now := time.Now()
	record := unsubscribeRecord{
		Type:           "subscriptions",
		Event:          "closed",
		SubscriptionID: subscriptionID,
		NetworkNum:     networkNum,
		FeedName:       feedName,
		AccountID:      accountID,
		Tier:           tierName,
	}
	s.LogToFluentD(record, now, "stats.subscriptions.events")
}
