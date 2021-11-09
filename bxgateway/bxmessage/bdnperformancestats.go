package bxmessage

import (
	"encoding/binary"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/bxmessage/utils"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/types"
	cmap "github.com/orcaman/concurrent-map"
	log "github.com/sirupsen/logrus"
	"math"
	"time"
)

// BdnPerformanceStatsData - represent the bdn stat data struct sent in BdnPerformanceStats
type BdnPerformanceStatsData struct {
	BlockchainNodeIPEndpoint            types.NodeEndpoint
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
	nodeStats           cmap.ConcurrentMap
	prevStats           cmap.ConcurrentMap // not sent in message
}

// NewBDNStats returns a new instance of BDNPerformanceStats
func NewBDNStats() *BdnPerformanceStats {
	nodeStatsMap := cmap.New()
	bdnStats := BdnPerformanceStats{
		intervalStartTime: time.Now(),
		nodeStats:         nodeStatsMap,
		prevStats:         nil,
	}
	return &bdnStats
}

// StartNewInterval clears the previous interval and sets start time of new interval
func (bs *BdnPerformanceStats) StartNewInterval() {
	bs.intervalStartTime = time.Now()
	bs.memoryUtilizationMb = 0
	bs.resetIntervalStats()
}

func (bs *BdnPerformanceStats) resetIntervalStats() {
	newNodeStatsMap := cmap.New()
	for elem := range bs.nodeStats.IterBuffered() {
		stats := elem.Val.(*BdnPerformanceStatsData)
		newStatsData := BdnPerformanceStatsData{BlockchainNodeIPEndpoint: stats.BlockchainNodeIPEndpoint}
		newNodeStatsMap.Set(stats.BlockchainNodeIPEndpoint.String(), &newStatsData)
	}
	bs.prevStats = bs.nodeStats
	bs.nodeStats = newNodeStatsMap
}

// CloseInterval sets the interval end time
func (bs *BdnPerformanceStats) CloseInterval() {
	bs.intervalEndTime = time.Now()
	if bs.prevStats != nil {
		bs.prevStats.Clear()
	}
}

// SetMemoryUtilization sets the memory utilization field of message
func (bs *BdnPerformanceStats) SetMemoryUtilization(mb int) {
	bs.memoryUtilizationMb = uint16(mb)
}

// LogNewBlockFromNode logs new block from blockchain for specified node; new block seen; from BDN for other nodes
func (bs *BdnPerformanceStats) LogNewBlockFromNode(node types.NodeEndpoint) {
	nodeStats := bs.getNodeStats(node)
	nodeStats.NewBlocksReceivedFromBlockchainNode++
	for elem := range bs.nodeStats.IterBuffered() {
		stats := elem.Val.(*BdnPerformanceStatsData)
		stats.NewBlocksSeen++
		if stats.BlockchainNodeIPEndpoint.IP == node.IP && stats.BlockchainNodeIPEndpoint.Port == node.Port {
			continue
		}
		stats.NewBlocksReceivedFromBdn++
	}
}

// LogNewBlockFromBDN logs a new block from the BDN and new block seen in the stats for all nodes
func (bs *BdnPerformanceStats) LogNewBlockFromBDN() {
	for elem := range bs.nodeStats.IterBuffered() {
		stats := elem.Val.(*BdnPerformanceStatsData)
		stats.NewBlocksSeen++
		stats.NewBlocksReceivedFromBdn++
	}
}

// LogNewBlockMessageFromNode logs a new block message from blockchain node in the stats for specified node
func (bs *BdnPerformanceStats) LogNewBlockMessageFromNode(node types.NodeEndpoint) {
	nodeStats := bs.getNodeStats(node)
	nodeStats.NewBlockMessagesFromBlockchainNode++
}

// LogNewBlockAnnouncementFromNode logs a new block announcement from blockchain node in the stats for specified node
func (bs *BdnPerformanceStats) LogNewBlockAnnouncementFromNode(node types.NodeEndpoint) {
	nodeStats := bs.getNodeStats(node)
	nodeStats.NewBlockAnnouncementsFromBlockchainNode++
}

// LogNewTxFromNode logs new tx from blockchain in stats for specified node, from BDN for other nodes
func (bs *BdnPerformanceStats) LogNewTxFromNode(node types.NodeEndpoint) {
	nodeStats := bs.getNodeStats(node)
	nodeStats.NewTxReceivedFromBlockchainNode++
	for elem := range bs.nodeStats.IterBuffered() {
		stats := elem.Val.(*BdnPerformanceStatsData)
		if stats.BlockchainNodeIPEndpoint.IP == node.IP && stats.BlockchainNodeIPEndpoint.Port == node.Port {
			continue
		}
		stats.NewTxReceivedFromBdn++
	}
}

// LogNewTxFromBDN logs a new tx from BDN in the stats for all nodes
func (bs *BdnPerformanceStats) LogNewTxFromBDN() {
	for elem := range bs.nodeStats.IterBuffered() {
		stats := elem.Val.(*BdnPerformanceStatsData)
		stats.NewTxReceivedFromBdn++
	}
}

// LogTxSentToNode logs a tx sent to all blockchain nodes
func (bs *BdnPerformanceStats) LogTxSentToNode() {
	for elem := range bs.nodeStats.IterBuffered() {
		stats := elem.Val.(*BdnPerformanceStatsData)
		stats.TxSentToNode++
	}
}

// LogDuplicateTxFromNode logs a duplicate tx from blockchain node in the stats for specified node
func (bs *BdnPerformanceStats) LogDuplicateTxFromNode(node types.NodeEndpoint) {
	nodeStats := bs.getNodeStats(node)
	nodeStats.DuplicateTxFromNode++
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

// NodeStats returns the bdn stats data for all nodes
func (bs *BdnPerformanceStats) NodeStats() cmap.ConcurrentMap {
	return bs.nodeStats
}

func (bs *BdnPerformanceStats) getNodeStats(node types.NodeEndpoint) *BdnPerformanceStatsData {
	val, ok := bs.nodeStats.Get(node.String())
	if ok {
		stats := val.(*BdnPerformanceStatsData)
		return stats
	}
	newStatsData := BdnPerformanceStatsData{BlockchainNodeIPEndpoint: node}
	bs.nodeStats.Set(node.String(), &newStatsData)
	return &newStatsData
}

// Pack serializes a BdnPerformanceStats into a buffer for sending
func (bs *BdnPerformanceStats) Pack(_ Protocol) ([]byte, error) {
	bufLen := bs.size()
	buf := make([]byte, bufLen)
	offset := uint32(HeaderLen)
	binary.LittleEndian.PutUint64(buf[offset:], math.Float64bits(float64(bs.intervalStartTime.UnixNano())/float64(1e9)))
	offset += TimestampLen
	binary.LittleEndian.PutUint64(buf[offset:], math.Float64bits(float64(bs.intervalEndTime.UnixNano())/float64(1e9)))
	offset += TimestampLen
	binary.LittleEndian.PutUint16(buf[offset:], bs.memoryUtilizationMb)
	offset += types.UInt16Len
	nodeStatsLen := bs.nodeStats.Count()
	binary.LittleEndian.PutUint16(buf[offset:], uint16(nodeStatsLen))
	offset += types.UInt16Len

	for elem := range bs.nodeStats.IterBuffered() {
		nodeStats := elem.Val.(*BdnPerformanceStatsData)
		utils.PackIPPort(buf[offset:], nodeStats.BlockchainNodeIPEndpoint.IP, uint16(nodeStats.BlockchainNodeIPEndpoint.Port))
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
	buf[bufLen-1] = ControlByte
	bs.Header.Pack(&buf, BDNPerformanceStatsType)
	return buf, nil
}

// Unpack deserializes a BdnPerformanceStats from a buffer
func (bs *BdnPerformanceStats) Unpack(buf []byte, _ Protocol) error {
	nodeStatsMap := cmap.New()
	bs.nodeStats = nodeStatsMap

	nodeStatsOffset := (types.UInt64Len * 2) + (types.UInt16Len * 2)
	if err := checkBufSize(&buf, HeaderLen, nodeStatsOffset); err != nil {
		return err
	}
	offset := uint32(HeaderLen)
	startTimestamp := math.Float64frombits(binary.LittleEndian.Uint64(buf[offset:]))
	startNanoseconds := int64(float64(startTimestamp) * float64(1e9))
	bs.intervalStartTime = time.Unix(0, startNanoseconds)
	offset += TimestampLen
	endTimestamp := math.Float64frombits(binary.LittleEndian.Uint64(buf[offset:]))
	endNanoseconds := int64(float64(endTimestamp) * float64(1e9))
	bs.intervalEndTime = time.Unix(0, endNanoseconds)
	offset += TimestampLen
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
		singleNodeStats.BlockchainNodeIPEndpoint = types.NodeEndpoint{IP: ip, Port: int(port)}
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
		bs.nodeStats.Set(singleNodeStats.BlockchainNodeIPEndpoint.String(), &singleNodeStats)
	}
	return nil
}

func (bs *BdnPerformanceStats) size() uint32 {
	nodeStatsSize := utils.IPAddrSizeInBytes + (types.UInt32Len * 7) + (types.UInt16Len * 3)
	return uint32(HeaderLen + (types.UInt64Len * 2) + (types.UInt16Len * 2) + (bs.nodeStats.Count() * nodeStatsSize) + ControlByteLen)
}
