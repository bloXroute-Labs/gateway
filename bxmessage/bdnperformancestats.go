package bxmessage

import (
	"encoding/binary"
	"fmt"
	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage/utils"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

// BdnPerformanceStatsData - represent the bdn stat data struct sent in BdnPerformanceStats
type BdnPerformanceStatsData struct {
	NewBlocksReceivedFromBlockchainNode uint16
	NewBlocksReceivedFromBdn            uint16
	NewBlocksSeen                       uint32

	// block_messages vs. block_announcements might not be a distinction that
	// exists in all blockchains. For example, in Ethereum this is the
	// distinction between NewBlock and NewBlockHashes messages
	NewBlockMessagesFromBlockchainNode      uint32
	NewBlockAnnouncementsFromBlockchainNode uint32

	NewTxReceivedFromBlockchainNode uint32
	NewTxReceivedFromBdn            uint32
	TxSentToNode                    uint32
	DuplicateTxFromNode             uint32
	IsBeacon                        bool
}

// BdnPerformanceStats - represent the "bdnstats" message
type BdnPerformanceStats struct {
	Header
	intervalStartTime   time.Time
	intervalEndTime     time.Time
	memoryUtilizationMb uint16
	nodeStats           map[string]*BdnPerformanceStatsData

	burstLimitedTransactionsPaid   uint16
	burstLimitedTransactionsUnpaid uint16

	lock sync.Mutex
}

// NewBDNStats returns a new instance of BDNPerformanceStats
func NewBDNStats(blockchainPeers []types.NodeEndpoint) *BdnPerformanceStats {
	bdnStats := BdnPerformanceStats{
		intervalStartTime: time.Now(),
		nodeStats:         make(map[string]*BdnPerformanceStatsData),
	}
	for _, endpoint := range blockchainPeers {
		newStatsData := BdnPerformanceStatsData{}
		newStatsData.IsBeacon = endpoint.IsBeacon
		bdnStats.nodeStats[endpoint.IPPort()] = &newStatsData
	}
	return &bdnStats
}

// IsGatewayAllow checks if gateway connected with too many peers
func (bs *BdnPerformanceStats) IsGatewayAllow(minAllowedNodes uint64, maxAllowedNodes uint64, protocol Protocol) bool {
	var countEnodes uint64

	switch {
	case protocol < IsBeaconProtocol:
		// TODO when we upgrade all go gateways less than `IsBeaconProtocol`, can remove this block
		for _, nodeStats := range bs.nodeStats {
			if nodeStats.NewBlocksReceivedFromBlockchainNode > 0 || nodeStats.NewTxReceivedFromBlockchainNode > 0 {
				countEnodes++
			}
		}
	default:
		for _, nodeStats := range bs.nodeStats {
			if !nodeStats.IsBeacon {
				countEnodes++
			}
		}
	}

	return countEnodes >= minAllowedNodes && countEnodes <= maxAllowedNodes
}

// CloseInterval sets the closing interval end time, starts new interval with cleared stats, and returns BdnPerformanceStats of closed interval
func (bs *BdnPerformanceStats) CloseInterval() BdnPerformanceStats {
	bs.lock.Lock()

	// close interval
	bs.intervalEndTime = time.Now()

	// create BDNStats from closed interval for logging and sending
	prevBDNStats := BdnPerformanceStats{
		intervalStartTime:              bs.intervalStartTime,
		intervalEndTime:                bs.intervalEndTime,
		memoryUtilizationMb:            bs.memoryUtilizationMb,
		nodeStats:                      bs.nodeStats,
		burstLimitedTransactionsPaid:   bs.burstLimitedTransactionsPaid,
		burstLimitedTransactionsUnpaid: bs.burstLimitedTransactionsUnpaid,
	}

	// create fresh map with existing nodes for new interval
	bs.nodeStats = make(map[string]*BdnPerformanceStatsData)
	for endpoint, oldNodeStats := range prevBDNStats.nodeStats {
		newStatsData := BdnPerformanceStatsData{}
		newStatsData.IsBeacon = oldNodeStats.IsBeacon
		bs.nodeStats[endpoint] = &newStatsData
	}

	// start new interval
	bs.intervalStartTime = time.Now()

	bs.lock.Unlock()

	bs.memoryUtilizationMb = 0
	return prevBDNStats
}

// SetMemoryUtilization sets the memory utilization field of message
func (bs *BdnPerformanceStats) SetMemoryUtilization(mb int) {
	bs.memoryUtilizationMb = uint16(mb)
}

// LogNewBlockFromNode logs new block from blockchain for specified node; new block seen; from BDN for other nodes
func (bs *BdnPerformanceStats) LogNewBlockFromNode(node types.NodeEndpoint) {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	nodeStats, err := bs.getNodeStats(node)
	if err != nil {
		log.Error(err)
		return
	}
	nodeStats.NewBlocksReceivedFromBlockchainNode++
	for endpoint, stats := range bs.nodeStats {
		stats.NewBlocksSeen++
		if endpoint == node.IPPort() {
			continue
		}
		stats.NewBlocksReceivedFromBdn++
	}
}

// LogNewBlockFromBDN logs a new block from the BDN and new block seen in the stats for all nodes
func (bs *BdnPerformanceStats) LogNewBlockFromBDN() {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	for _, stats := range bs.nodeStats {
		stats.NewBlocksSeen++
		stats.NewBlocksReceivedFromBdn++
	}
}

// LogNewBlockMessageFromNode logs a new block message from blockchain node in the stats for specified node
func (bs *BdnPerformanceStats) LogNewBlockMessageFromNode(node types.NodeEndpoint) {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	nodeStats, err := bs.getNodeStats(node)
	if err != nil {
		log.Error(err)
		return
	}
	nodeStats.NewBlockMessagesFromBlockchainNode++
}

// LogNewBlockAnnouncementFromNode logs a new block announcement from blockchain node in the stats for specified node
func (bs *BdnPerformanceStats) LogNewBlockAnnouncementFromNode(node types.NodeEndpoint) {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	nodeStats, err := bs.getNodeStats(node)
	if err != nil {
		log.Error(err)
		return
	}
	nodeStats.NewBlockAnnouncementsFromBlockchainNode++
}

// LogNewTxFromNode logs new tx from blockchain in stats for specified node, from BDN for other nodes
func (bs *BdnPerformanceStats) LogNewTxFromNode(node types.NodeEndpoint) {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	for endpoint, stats := range bs.nodeStats {
		if endpoint == node.IPPort() {
			stats.NewTxReceivedFromBlockchainNode++
			continue
		}
		stats.NewTxReceivedFromBdn++
	}
}

// LogNewTxFromBDN logs a new tx from BDN in the stats for all nodes
func (bs *BdnPerformanceStats) LogNewTxFromBDN() {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	for _, stats := range bs.nodeStats {
		stats.NewTxReceivedFromBdn++
	}
}

// LogTxSentToAllNodesExceptSourceNode logs a tx sent to all blockchain nodes expect source if applicable
func (bs *BdnPerformanceStats) LogTxSentToAllNodesExceptSourceNode(sourceNode types.NodeEndpoint) {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	for endpoint, stats := range bs.nodeStats {
		if endpoint == sourceNode.IPPort() {
			continue
		}
		stats.TxSentToNode++
	}
}

// LogTxSentToNodes logs a tx sent to all blockchain node
func (bs *BdnPerformanceStats) LogTxSentToNodes() {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	for _, stats := range bs.nodeStats {
		stats.TxSentToNode++
	}
}

// LogDuplicateTxFromNode logs a duplicate tx from blockchain node in the stats for specified node
func (bs *BdnPerformanceStats) LogDuplicateTxFromNode(node types.NodeEndpoint) {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	nodeStats, err := bs.getNodeStats(node)
	if err != nil {
		log.Error(err)
		return
	}
	nodeStats.DuplicateTxFromNode++
}

// LogBurstLimitedTransactionsPaid logs a tx count exceeded limit paid transactions in the stats
func (bs *BdnPerformanceStats) LogBurstLimitedTransactionsPaid() {
	bs.burstLimitedTransactionsPaid++
}

// LogBurstLimitedTransactionsUnpaid logs a tx count exceeded limit paid transactions in the stats
func (bs *BdnPerformanceStats) LogBurstLimitedTransactionsUnpaid() {
	bs.burstLimitedTransactionsUnpaid++
}

// StartTime returns the start time of the current stat interval
func (bs *BdnPerformanceStats) StartTime() time.Time {
	return bs.intervalStartTime
}

// EndTime returns the start time of the current stat interval
func (bs *BdnPerformanceStats) EndTime() time.Time {
	return bs.intervalEndTime
}

// Memory returns memory utilization stat
func (bs *BdnPerformanceStats) Memory() uint16 {
	return bs.memoryUtilizationMb
}

// BurstLimitedTransactionsPaid returns relay burst limited transactions paid stat
func (bs *BdnPerformanceStats) BurstLimitedTransactionsPaid() uint16 {
	return bs.burstLimitedTransactionsPaid
}

// BurstLimitedTransactionsUnpaid returns relay burst limited transactions unpaid stat
func (bs *BdnPerformanceStats) BurstLimitedTransactionsUnpaid() uint16 {
	return bs.burstLimitedTransactionsUnpaid
}

// NodeStats returns the bdn stats data for all nodes
func (bs *BdnPerformanceStats) NodeStats() map[string]*BdnPerformanceStatsData {
	return bs.nodeStats
}

func (bs *BdnPerformanceStats) getNodeStats(node types.NodeEndpoint) (*BdnPerformanceStatsData, error) {
	stats, ok := bs.nodeStats[node.IPPort()]
	if ok {
		return stats, nil
	}
	return nil, fmt.Errorf("unable to find BDN stats for node %v", node.IPPort())

}

// Pack serializes a BdnPerformanceStats into a buffer for sending
func (bs *BdnPerformanceStats) Pack(protocol Protocol) ([]byte, error) {
	bs.lock.Lock()
	defer bs.lock.Unlock()

	bufLen := bs.size(protocol)
	buf := make([]byte, bufLen)
	offset := uint32(HeaderLen)
	binary.LittleEndian.PutUint64(buf[offset:], math.Float64bits(float64(bs.intervalStartTime.UnixNano())/float64(1e9)))
	offset += types.UInt64Len
	binary.LittleEndian.PutUint64(buf[offset:], math.Float64bits(float64(bs.intervalEndTime.UnixNano())/float64(1e9)))
	offset += types.UInt64Len
	binary.LittleEndian.PutUint16(buf[offset:], bs.memoryUtilizationMb)
	offset += types.UInt16Len

	nodeStatsLen := len(bs.nodeStats)
	binary.LittleEndian.PutUint16(buf[offset:], uint16(nodeStatsLen))
	offset += types.UInt16Len
	for endpoint, nodeStats := range bs.nodeStats {
		ipPort := strings.Split(endpoint, " ")
		port, err := strconv.Atoi(ipPort[1])
		if err != nil {
			return nil, err
		}
		utils.PackIPPort(buf[offset:], ipPort[0], uint16(port))
		offset += utils.IPAddrSizeInBytes + types.UInt16Len
		binary.LittleEndian.PutUint16(buf[offset:], nodeStats.NewBlocksReceivedFromBlockchainNode)
		offset += types.UInt16Len
		binary.LittleEndian.PutUint16(buf[offset:], nodeStats.NewBlocksReceivedFromBdn)
		offset += types.UInt16Len
		binary.LittleEndian.PutUint32(buf[offset:], nodeStats.NewTxReceivedFromBlockchainNode)
		offset += types.UInt32Len
		binary.LittleEndian.PutUint32(buf[offset:], nodeStats.NewTxReceivedFromBdn)
		offset += types.UInt32Len
		binary.LittleEndian.PutUint32(buf[offset:], nodeStats.NewBlocksSeen)
		offset += types.UInt32Len
		binary.LittleEndian.PutUint32(buf[offset:], nodeStats.NewBlockMessagesFromBlockchainNode)
		offset += types.UInt32Len
		binary.LittleEndian.PutUint32(buf[offset:], nodeStats.NewBlockAnnouncementsFromBlockchainNode)
		offset += types.UInt32Len
		binary.LittleEndian.PutUint32(buf[offset:], nodeStats.TxSentToNode)
		offset += types.UInt32Len
		binary.LittleEndian.PutUint32(buf[offset:], nodeStats.DuplicateTxFromNode)
		offset += types.UInt32Len

		switch {
		case protocol < IsBeaconProtocol:
		default:
			if nodeStats.IsBeacon {
				copy(buf[offset:], []uint8{1})
			} else {
				copy(buf[offset:], []uint8{0})
			}
			offset += IsBeaconLen
		}
	}

	switch {
	case protocol < FullTxTimeStampProtocol:
	default:
		binary.LittleEndian.PutUint16(buf[offset:], bs.burstLimitedTransactionsPaid)
		offset += types.UInt16Len
		binary.LittleEndian.PutUint16(buf[offset:], bs.burstLimitedTransactionsUnpaid)
	}
	bs.Header.Pack(&buf, BDNPerformanceStatsType)
	return buf, nil
}

// Unpack deserializes a BdnPerformanceStats from a buffer
func (bs *BdnPerformanceStats) Unpack(buf []byte, protocol Protocol) error {
	bs.nodeStats = make(map[string]*BdnPerformanceStatsData)
	nodeStatsOffset := (types.UInt64Len * 2) + (types.UInt16Len * 2)
	if err := checkBufSize(&buf, HeaderLen, nodeStatsOffset); err != nil {
		return err
	}
	offset := uint32(HeaderLen)
	startTimestamp := math.Float64frombits(binary.LittleEndian.Uint64(buf[offset:]))
	startNanoseconds := int64(float64(startTimestamp) * float64(1e9))
	bs.intervalStartTime = time.Unix(0, startNanoseconds)
	offset += types.UInt64Len
	endTimestamp := math.Float64frombits(binary.LittleEndian.Uint64(buf[offset:]))
	endNanoseconds := int64(float64(endTimestamp) * float64(1e9))
	bs.intervalEndTime = time.Unix(0, endNanoseconds)
	offset += types.UInt64Len
	bs.memoryUtilizationMb = binary.LittleEndian.Uint16(buf[offset:])
	offset += types.UInt16Len
	nodesStatsLen := binary.LittleEndian.Uint16(buf[offset:])
	offset += types.UInt16Len

	nodeStatsSize := int(nodesStatsLen) * ((utils.IPAddrSizeInBytes) + (types.UInt32Len * 7) + (types.UInt16Len * 3))
	if err := checkBufSize(&buf, int(offset), nodeStatsSize); err != nil {
		return err
	}

	emptyEndpoint := types.NodeEndpoint{IP: "0.0.0.0"}
	for i := 0; i < int(nodesStatsLen); i++ {
		var singleNodeStats BdnPerformanceStatsData
		ip, port, err := utils.UnpackIPPort(buf[offset:])
		if err != nil {
			log.Errorf("unable to parse ip and port from BDNPerformanceStats message: %v", err)
		}
		endpoint := types.NodeEndpoint{IP: ip, Port: int(port)}
		offset += utils.IPAddrSizeInBytes + types.UInt16Len
		singleNodeStats.NewBlocksReceivedFromBlockchainNode = binary.LittleEndian.Uint16(buf[offset:])
		offset += types.UInt16Len
		singleNodeStats.NewBlocksReceivedFromBdn = binary.LittleEndian.Uint16(buf[offset:])
		offset += types.UInt16Len
		singleNodeStats.NewTxReceivedFromBlockchainNode = binary.LittleEndian.Uint32(buf[offset:])
		offset += types.UInt32Len
		singleNodeStats.NewTxReceivedFromBdn = binary.LittleEndian.Uint32(buf[offset:])
		offset += types.UInt32Len
		singleNodeStats.NewBlocksSeen = binary.LittleEndian.Uint32(buf[offset:])
		offset += types.UInt32Len
		singleNodeStats.NewBlockMessagesFromBlockchainNode = binary.LittleEndian.Uint32(buf[offset:])
		offset += types.UInt32Len
		singleNodeStats.NewBlockAnnouncementsFromBlockchainNode = binary.LittleEndian.Uint32(buf[offset:])
		offset += types.UInt32Len
		singleNodeStats.TxSentToNode = binary.LittleEndian.Uint32(buf[offset:])
		offset += types.UInt32Len
		singleNodeStats.DuplicateTxFromNode = binary.LittleEndian.Uint32(buf[offset:])
		offset += types.UInt32Len

		if endpoint.IPPort() != emptyEndpoint.IPPort() {
			bs.nodeStats[endpoint.IPPort()] = &singleNodeStats
		}

		switch {
		case protocol < IsBeaconProtocol:
		default:
			singleNodeStats.IsBeacon = int(buf[offset : offset+IsBeaconLen][0]) != 0
			offset += IsBeaconLen
		}
	}
	switch {
	case protocol < FullTxTimeStampProtocol:
	default:
		if err := checkBufSize(&buf, int(offset), 2*types.UInt16Len); err != nil {
			return err
		}
		bs.burstLimitedTransactionsPaid = binary.LittleEndian.Uint16(buf[offset:])
		offset += types.UInt16Len
		bs.burstLimitedTransactionsUnpaid = binary.LittleEndian.Uint16(buf[offset:])
	}
	return bs.Header.Unpack(buf, protocol)
}

// Log logs stats
func (bs *BdnPerformanceStats) Log() {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	for endpoint, nodeStats := range bs.NodeStats() {
		log.Infof("%v [%v - %v]: Processed %v blocks and %v transactions from the BDN", endpoint, bs.StartTime().Format(bxgateway.TimeLayoutISO), bs.EndTime().Format(bxgateway.TimeLayoutISO), nodeStats.NewBlocksReceivedFromBdn, nodeStats.NewTxReceivedFromBdn)
	}
}

func (bs *BdnPerformanceStats) size(protocol Protocol) uint32 {
	nodeStatsSize := utils.IPAddrSizeInBytes + (types.UInt32Len * 7) + (types.UInt16Len * 3)
	if protocol >= IsBeaconProtocol {
		nodeStatsSize += IsBeaconLen
	}
	total := bs.Header.Size() + uint32((types.UInt64Len*2)+(types.UInt16Len*2)+(len(bs.nodeStats)*nodeStatsSize))
	if protocol >= FullTxTimeStampProtocol {
		total += types.UInt16Len * 2
	}
	return total
}
