package bxmessage

import (
	"encoding/binary"
	"github.com/bloXroute-Labs/gateway"
	"github.com/bloXroute-Labs/gateway/bxmessage/utils"
	log "github.com/bloXroute-Labs/gateway/logger"
	"github.com/bloXroute-Labs/gateway/types"
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
func NewBDNStats() *BdnPerformanceStats {
	bdnStats := BdnPerformanceStats{
		intervalStartTime: time.Now(),
		nodeStats:         make(map[string]*BdnPerformanceStatsData),
	}
	return &bdnStats
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
	for endpoint := range prevBDNStats.nodeStats {
		newStatsData := BdnPerformanceStatsData{}
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
	nodeStats := bs.getNodeStats(node)
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
	nodeStats := bs.getNodeStats(node)
	nodeStats.NewBlockMessagesFromBlockchainNode++
}

// LogNewBlockAnnouncementFromNode logs a new block announcement from blockchain node in the stats for specified node
func (bs *BdnPerformanceStats) LogNewBlockAnnouncementFromNode(node types.NodeEndpoint) {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	nodeStats := bs.getNodeStats(node)
	nodeStats.NewBlockAnnouncementsFromBlockchainNode++
}

// LogNewTxFromNode logs new tx from blockchain in stats for specified node, from BDN for other nodes
func (bs *BdnPerformanceStats) LogNewTxFromNode(node types.NodeEndpoint) {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	nodeStats := bs.getNodeStats(node)
	nodeStats.NewTxReceivedFromBlockchainNode++
	for endpoint, stats := range bs.nodeStats {
		if endpoint == node.IPPort() {
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

// LogTxSentToNode logs a tx sent to all blockchain nodes
func (bs *BdnPerformanceStats) LogTxSentToNode() {
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
	nodeStats := bs.getNodeStats(node)
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

func (bs *BdnPerformanceStats) getNodeStats(node types.NodeEndpoint) *BdnPerformanceStatsData {
	stats, ok := bs.nodeStats[node.IPPort()]
	if ok {
		return stats
	}
	newStatsData := BdnPerformanceStatsData{}
	bs.nodeStats[node.IPPort()] = &newStatsData
	return &newStatsData
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
	}

	if protocol >= FullTxTimeStampProtocol {
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

		bs.nodeStats[endpoint.IPPort()] = &singleNodeStats
	}

	if protocol >= FullTxTimeStampProtocol {
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
	total := bs.Header.Size() + uint32((types.UInt64Len*2)+(types.UInt16Len*2)+(len(bs.nodeStats)*nodeStatsSize))
	if protocol >= FullTxTimeStampProtocol {
		total += types.UInt16Len * 2
	}
	return total
}
