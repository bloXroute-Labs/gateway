package statistics

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/fluent/fluent-logger-golang/fluent"
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
	LogSubscribeStats(subscriptionID string, accountID types.AccountID, feedName types.FeedType, tierName sdnmessage.AccountTier,
		ip string, networkNum types.NetworkNum, feedInclude []string, feedFilter string, feedProject string)
	AddBundleEvent(name string, source connections.Conn, startTime time.Time, bundleHash string, networkNum types.NetworkNum,
		mevBuilderNames []string, frontrunning bool, uuid string, targetBlockNumber uint64, minTimestamp int, maxTimestamp int, bundlePrice int64, enforcePayout bool, sentPeers int, sentGatewayPeers int)
	AddGatewayBundleEvent(name string, source connections.Conn, startTime time.Time, bundleHash string, networkNum types.NetworkNum,
		mevBuilderNames []string, frontrunning bool, uuid string, targetBlockNumber uint64, minTimestamp int, maxTimestamp int, bundlePrice int64, enforcePayout bool)
	LogUnsubscribeStats(subscriptionID string, feedName types.FeedType, networkNum types.NetworkNum, accountID types.AccountID, tierName sdnmessage.AccountTier)
	LogSDKInfo(blockchain, method, sourceCode, version string, accountID types.AccountID, feed types.FeedConnectionType, start, end time.Time)
	BundleSentToBuilderStats(timestamp time.Time, builderName string, bundleHash string, blockNumber string, uuid string, networkNum types.NetworkNum, accountID types.AccountID, accountTier sdnmessage.AccountTier, builderURL string, statusCode int)
	AddBuilderGatewaySentBundleToMEVBuilderEvent(timestamp time.Time, builderName, bundleHash, blockNumber, uuid, builderURL string,
		networkNum types.NetworkNum, accountID types.AccountID, accountTier sdnmessage.AccountTier, statusCode int)
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

// AddBundleEvent does nothing
func (NoStats) AddBundleEvent(name string, source connections.Conn, startTime time.Time, bundleHash string, networkNum types.NetworkNum, mevBuilderNames []string, frontrunning bool, uuid string, targetBlockNumber uint64, minTimestamp int, maxTimestamp int, bundlePrice int64, enforcePayout bool, sentPeers int, sentGatewayPeers int) {
}

// AddGatewayBundleEvent does nothing
func (NoStats) AddGatewayBundleEvent(name string, source connections.Conn, startTime time.Time, bundleHash string, networkNum types.NetworkNum, mevBuilderNames []string, frontrunning bool, uuid string, targetBlockNumber uint64, minTimestamp int, maxTimestamp int, bundlePrice int64, enforcePayout bool) {
}

// AddTxsByShortIDsEvent does nothing
func (NoStats) AddTxsByShortIDsEvent(name string, source connections.Conn, txInfo *types.BxTransaction,
	shortID types.ShortID, sourceID types.NodeID, sentPeers int, sentGatewayPeers int,
	startTime time.Time, priority bxmessage.SendPriority, debugData interface{}) {
}

// LogSubscribeStats does nothing
func (NoStats) LogSubscribeStats(subscriptionID string, accountID types.AccountID, feedName types.FeedType, tierName sdnmessage.AccountTier,
	ip string, networkNum types.NetworkNum, feedInclude []string, feedFilter string, feedProject string) {
}

// LogUnsubscribeStats does nothing
func (NoStats) LogUnsubscribeStats(subscriptionID string, feedName types.FeedType, networkNum types.NetworkNum, accountID types.AccountID, tierName sdnmessage.AccountTier) {
}

// LogSDKInfo does nothing
func (NoStats) LogSDKInfo(_, _, _, _ string, _ types.AccountID, _ types.FeedConnectionType, _, _ time.Time) {
}

// BundleSentToBuilderStats does nothing
func (NoStats) BundleSentToBuilderStats(_ time.Time, _ string, _ string, _ string, _ string, _ types.NetworkNum, _ types.AccountID, _ sdnmessage.AccountTier, _ string, _ int) {
}

// AddBuilderGatewaySentBundleToMEVBuilderEvent does nothing
func (NoStats) AddBuilderGatewaySentBundleToMEVBuilderEvent(_ time.Time, _, _, _, _, _ string,
	_ types.NetworkNum, _ types.AccountID, _ sdnmessage.AccountTier, _ int) {
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
	if txInfo == nil || !s.shouldLogEvent(txInfo.NetworkNum(), txInfo.Hash()) {
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

// AddBundleEvent generates a fluentd STATS event
// Includes sent peers and sent gateway peers
func (s FluentdStats) AddBundleEvent(name string, source connections.Conn, startTime time.Time, bundleHash string, networkNum types.NetworkNum, mevBuilderNames []string, frontrunning bool, uuid string, targetBlockNumber uint64, minTimestamp int, maxTimestamp int, bundlePrice int64, enforcePayout bool, sentPeers int, sentGatewayPeers int) {
	hashInBytes, err := types.NewSHA256HashFromString(bundleHash)
	if err != nil {
		log.Errorf("error converting bundle hash from string to byte: %v", err)
		return
	}
	if !s.shouldLogEvent(networkNum, hashInBytes) {
		return
	}
	now := time.Now()

	record := Record{
		Type: "Bundle",
		Data: bundleRecord{
			EventSubjectID:  bundleHash,
			EventName:       name,
			AccountID:       source.GetAccountID(),
			NodeID:          s.NodeID,
			StartDateTime:   startTime.Format(DateFormat),
			EndDateTime:     now.Format(DateFormat),
			NetworkNum:      networkNum,
			MEVBuilderNames: mevBuilderNames,
			FrontRunning:    frontrunning,
			UUID:            uuid,
			BlockNumber:     targetBlockNumber,
			MinTimestamp:    minTimestamp,
			MaxTimestamp:    maxTimestamp,
			BundlePrice:     bundlePrice,
			EnforcePayout:   enforcePayout,
			ExtraData: bundleExtraData{
				MoreInfo: fmt.Sprintf("source: %v", source),
			},
			SentPeers:        sentPeers,
			SentGatewayPeers: sentGatewayPeers,
		},
	}

	s.LogToFluentD(record, time.Now(), "stats.bundles.events.p")
}

// AddGatewayBundleEvent generates a fluentd STATS event
func (s FluentdStats) AddGatewayBundleEvent(name string, source connections.Conn, startTime time.Time, bundleHash string, networkNum types.NetworkNum, mevBuilderNames []string, frontrunning bool, uuid string, targetBlockNumber uint64, minTimestamp int, maxTimestamp int, bundlePrice int64, enforcePayout bool) {
	now := time.Now()

	record := Record{
		Type: "GatewayBundle",
		Data: bundleRecord{
			EventSubjectID:  bundleHash,
			EventName:       name,
			AccountID:       source.GetAccountID(),
			NodeID:          s.NodeID,
			StartDateTime:   startTime.Format(DateFormat),
			EndDateTime:     now.Format(DateFormat),
			NetworkNum:      networkNum,
			MEVBuilderNames: mevBuilderNames,
			FrontRunning:    frontrunning,
			UUID:            uuid,
			BlockNumber:     targetBlockNumber,
			MinTimestamp:    minTimestamp,
			MaxTimestamp:    maxTimestamp,
			BundlePrice:     bundlePrice,
			EnforcePayout:   enforcePayout,
			ExtraData: bundleExtraData{
				MoreInfo: fmt.Sprintf("source: %v", source),
			},
		},
	}

	s.LogToFluentD(record, time.Now(), "stats.gateway.bundles.events.p")
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

func (s FluentdStats) shouldLogEvent(network types.NetworkNum, hash types.SHA256Hash) bool {
	hashPercentage := s.getLogPercentageByHash(network)
	if hashPercentage <= 0 {
		return false
	}
	lastBytesValue := binary.BigEndian.Uint16(hash[types.SHA256HashLen-TailByteCount:])
	probabilityValue := float64(lastBytesValue) * float64(100) / float64(MaxTailByteValue)
	return probabilityValue <= hashPercentage
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
func (s FluentdStats) LogSubscribeStats(subscriptionID string, accountID types.AccountID, feedName types.FeedType, tierName sdnmessage.AccountTier,
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
func (s FluentdStats) LogUnsubscribeStats(subscriptionID string, feedName types.FeedType, networkNum types.NetworkNum, accountID types.AccountID, tierName sdnmessage.AccountTier) {
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

// LogSDKInfo generates a fluentd STATS event
func (s FluentdStats) LogSDKInfo(blockchain, method, sourceCode, version string, accountID types.AccountID, feed types.FeedConnectionType, start, end time.Time) {
	now := time.Now()
	record := sdkInfoRecord{
		Blockchain: blockchain,
		Method:     method,
		Feed:       string(feed),
		SourceCode: sourceCode,
		Version:    version,
		AccountID:  accountID,
		Start:      start.Format(DateFormat),
		End:        end.Format(DateFormat),
	}
	s.LogToFluentD(record, now, "stats.sdk.events")
}

// BundleSentToBuilderStats generates a fluentd STATS event
func (s FluentdStats) BundleSentToBuilderStats(timestamp time.Time, builderName string, bundleHash string, blockNumber string, uuid string, networkNum types.NetworkNum, accountID types.AccountID, accountTier sdnmessage.AccountTier, builderURL string, statusCode int) {
	blockNumberInt64, err := hex2int64(blockNumber)
	if err != nil {
		log.Errorf("BundleSentToBuilderStats: parse blockNumber: %s", blockNumber)
		return
	}

	record := Record{
		Type: "bundleSentToExternalBuilder",
		Data: bundleSentToBuilderRecord{
			BundleHash:  bundleHash,
			BlockNumber: blockNumberInt64,
			BuilderName: builderName,
			UUID:        uuid,
			NetworkNum:  networkNum,
			AccountID:   accountID,
			AccountTier: accountTier,
			BuilderURL:  builderURL,
			StatusCode:  statusCode,
		},
	}

	s.LogToFluentD(record, timestamp, "stats.bundles_sent_to_external_builder")
}

// hex2int64 takes a hex string and returns the parsed integer value.
// It handles hex strings with or without the "0x" prefix.
func hex2int64(hexStr string) (int64, error) {
	// Ensure the prefix is uniformly lowercase for comparison and remove it if present.
	return strconv.ParseInt(strings.TrimPrefix(strings.ToLower(hexStr), "0x"), 16, 64)
}

// AddBuilderGatewaySentBundleToMEVBuilderEvent generates a fluentd STATS event
func (s FluentdStats) AddBuilderGatewaySentBundleToMEVBuilderEvent(timestamp time.Time,
	builderName, bundleHash, blockNumber, uuid, builderURL string,
	networkNum types.NetworkNum,
	accountID types.AccountID,
	accountTier sdnmessage.AccountTier,
	statusCode int) {

	blockNumberInt64, err := hex2int64(blockNumber)
	if err != nil {
		log.Errorf("AddBuilderGatewaySentBundleToMEVBuilderEvent: parse blockNumber: %s", blockNumber)
		return
	}

	record := Record{
		Type: "BuilderGatewaySentBundleToMEVBuilder",
		Data: bundleSentToBuilderRecord{
			BundleHash:  bundleHash,
			BlockNumber: blockNumberInt64,
			BuilderName: builderName,
			UUID:        uuid,
			NetworkNum:  networkNum,
			AccountID:   accountID,
			AccountTier: accountTier,
			BuilderURL:  builderURL,
			StatusCode:  statusCode,
		},
	}

	s.LogToFluentD(record, timestamp, "stats.builder_gateway.sent_bundle")
}
