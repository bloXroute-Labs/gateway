package bxmessage

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage/utils"
	"github.com/bloXroute-Labs/gateway/v2/types"
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
	Dynamic                         bool
	IsConnected                     bool
	IsBeacon                        bool
	BlockchainNetwork               string
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

	staticConnections  int64
	dynamicConnections int64
	lock               sync.Mutex
}

// NewBDNStats returns a new instance of BDNPerformanceStats
func NewBDNStats(blockchainPeers []types.NodeEndpoint, recommendedPeers map[string]struct{}) *BdnPerformanceStats {
	bdnStats := BdnPerformanceStats{
		intervalStartTime: time.Now(),
		nodeStats:         make(map[string]*BdnPerformanceStatsData),
	}

	for _, endpoint := range blockchainPeers {
		newStatsData := BdnPerformanceStatsData{}
		newStatsData.IsBeacon = endpoint.IsBeacon
		newStatsData.BlockchainNetwork = endpoint.BlockchainNetwork
		// treat recommended peers as dynamic
		if _, ok := recommendedPeers[ipPort(endpoint.IP, endpoint.Port)]; ok {
			newStatsData.Dynamic = true
		}
		newStatsData.Dynamic = endpoint.IsDynamic()
		bdnStats.nodeStats[endpoint.IPPort()] = &newStatsData
	}
	return &bdnStats
}

// GetConnectionsCount return inbound/outbound connections
func (bs *BdnPerformanceStats) GetConnectionsCount() (int64, int64) {
	return bs.dynamicConnections, bs.staticConnections
}

// CloseInterval sets the closing interval end time, starts a new interval with cleared stats, and returns BdnPerformanceStats of a closed interval
func (bs *BdnPerformanceStats) CloseInterval() *BdnPerformanceStats {
	bs.lock.Lock()
	defer bs.lock.Unlock()

	// close interval
	bs.intervalEndTime = time.Now()

	nodeStats := map[string]*BdnPerformanceStatsData{}
	for endpoint, stat := range bs.nodeStats {
		if !stat.Dynamic {
			newStatsData := BdnPerformanceStatsData{}
			newStatsData.IsBeacon = stat.IsBeacon
			newStatsData.IsConnected = stat.IsConnected
			newStatsData.BlockchainNetwork = stat.BlockchainNetwork
			nodeStats[endpoint] = &newStatsData
		}
	}

	// create BDNStats from a closed interval for logging and sending
	prevBDNStats := &BdnPerformanceStats{
		intervalStartTime:              bs.intervalStartTime,
		intervalEndTime:                bs.intervalEndTime,
		memoryUtilizationMb:            bs.memoryUtilizationMb,
		burstLimitedTransactionsPaid:   bs.burstLimitedTransactionsPaid,
		burstLimitedTransactionsUnpaid: bs.burstLimitedTransactionsUnpaid,
		nodeStats:                      bs.nodeStats,
	}

	bs.nodeStats = nodeStats

	// start a new interval
	bs.intervalStartTime = time.Now()

	bs.memoryUtilizationMb = 0
	return prevBDNStats
}

// SetMemoryUtilization sets the memory utilization field of a message
func (bs *BdnPerformanceStats) SetMemoryUtilization(mb int) {
	bs.lock.Lock()
	defer bs.lock.Unlock()

	bs.memoryUtilizationMb = uint16(mb)
}

// LogNewBlockFromNode logs new block from blockchain for specified node; new block seen; from BDN for other nodes
func (bs *BdnPerformanceStats) LogNewBlockFromNode(node types.NodeEndpoint) {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	nodeStats := bs.getOrCreateNodeStats(node)
	nodeStats.NewBlocksReceivedFromBlockchainNode++
	for endpoint, stats := range bs.nodeStats {
		if !stats.IsConnected || (stats.BlockchainNetwork == bxtypes.Mainnet && !stats.IsBeacon) {
			continue
		}
		stats.NewBlocksSeen++
		if endpoint == node.IPPort() || (!stats.IsBeacon && node.BlockchainNetwork == bxtypes.Mainnet) { // if the stats is the source blockchain then counter already updated.
			continue // Or if the stats are for execution layer then no need to update the block counter
		}
		stats.NewBlocksReceivedFromBdn++
	}
}

// LogNewBlockFromBDN logs a new block from the BDN and new block seen in the stats for all nodes
func (bs *BdnPerformanceStats) LogNewBlockFromBDN(blockchainNetwork string) {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	for _, stats := range bs.nodeStats {
		if !stats.IsBeacon && blockchainNetwork == bxtypes.Mainnet { // no need to update an execution layer block counter
			continue
		}
		if stats.IsConnected {
			stats.NewBlocksSeen++
			stats.NewBlocksReceivedFromBdn++
		}
	}
}

// LogNewBlockMessageFromNode logs a new block message from a blockchain node in the stats for a specified node
func (bs *BdnPerformanceStats) LogNewBlockMessageFromNode(node types.NodeEndpoint) {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	if node.BlockchainNetwork == bxtypes.Mainnet && !node.IsBeacon { // no need to update an execution layer block counter
		return
	}

	nodeStats := bs.getOrCreateNodeStats(node)
	nodeStats.NewBlockMessagesFromBlockchainNode++
}

// LogNewBlockAnnouncementFromNode logs a new block announcement from a blockchain node in the stats for a specified node
func (bs *BdnPerformanceStats) LogNewBlockAnnouncementFromNode(node types.NodeEndpoint) {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	nodeStats := bs.getOrCreateNodeStats(node)
	nodeStats.NewBlockAnnouncementsFromBlockchainNode++
}

// LogNewTxFromNode logs new tx from blockchain in stats for a specified node, from BDN for other nodes
func (bs *BdnPerformanceStats) LogNewTxFromNode(node types.NodeEndpoint) {
	bs.lock.Lock()
	defer bs.lock.Unlock()

	bs.getOrCreateNodeStats(node)
	for endpoint, stats := range bs.nodeStats {
		if (stats.IsBeacon && stats.BlockchainNetwork == bxtypes.Mainnet) || !stats.IsConnected { // do not update tx counter for consensus layer
			continue
		} else if endpoint == node.IPPort() {
			stats.IsConnected = true
			stats.NewTxReceivedFromBlockchainNode++
			continue
		}
		stats.NewTxReceivedFromBdn++
	}
}

// SetBlockchainConnectionStatus logs blockchain connection status
func (bs *BdnPerformanceStats) SetBlockchainConnectionStatus(status blockchain.ConnectionStatus) {
	bs.lock.Lock()
	defer bs.lock.Unlock()

	log.Debugf("Connection status: %v", status)
	nodeStats, err := bs.getNodeStats(status.PeerEndpoint)
	if err == nil {
		nodeStats.IsConnected = status.IsConnected
		return
	}
	nodeIPPort := status.PeerEndpoint.IPPort()

	// if the node is connected and does not exist in the peers, need to add it, else return with an error
	if status.IsConnected {
		stats := &BdnPerformanceStatsData{Dynamic: status.PeerEndpoint.Dynamic, IsConnected: true, IsBeacon: status.PeerEndpoint.IsBeacon, BlockchainNetwork: status.PeerEndpoint.BlockchainNetwork}
		bs.nodeStats[nodeIPPort] = stats
	}
}

// LogNewTxFromBDN logs a new tx from BDN in the stats for all nodes
func (bs *BdnPerformanceStats) LogNewTxFromBDN() {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	for _, stats := range bs.nodeStats {
		if (stats.IsBeacon && stats.BlockchainNetwork == bxtypes.Mainnet) || !stats.IsConnected {
			continue
		}
		stats.NewTxReceivedFromBdn++
	}
}

// LogTxSentToAllNodesExceptSourceNode logs a tx sent to all blockchain nodes expect a source if applicable
func (bs *BdnPerformanceStats) LogTxSentToAllNodesExceptSourceNode(sourceNode types.NodeEndpoint) {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	for endpoint, stats := range bs.nodeStats {
		if endpoint == sourceNode.IPPort() || !stats.IsConnected || (stats.IsBeacon && stats.BlockchainNetwork == bxtypes.Mainnet) {
			continue
		}
		stats.TxSentToNode++
	}
}

// LogDuplicateTxFromNode logs a duplicate tx from a blockchain node in the stats for a specified node
func (bs *BdnPerformanceStats) LogDuplicateTxFromNode(node types.NodeEndpoint) {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	nodeStats := bs.getOrCreateNodeStats(node)
	nodeStats.DuplicateTxFromNode++
}

// LogBurstLimitedTransactionsPaid logs a tx count exceeded paid transactions limit in the stats
func (bs *BdnPerformanceStats) LogBurstLimitedTransactionsPaid() {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	bs.burstLimitedTransactionsPaid++
}

// LogBurstLimitedTransactionsUnpaid logs a tx count exceeded paid transactions limit in the stats
func (bs *BdnPerformanceStats) LogBurstLimitedTransactionsUnpaid() {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	bs.burstLimitedTransactionsUnpaid++
}

// StartTime returns the start time of the current stat interval
func (bs *BdnPerformanceStats) StartTime() time.Time {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	return bs.intervalStartTime
}

// EndTime returns the start time of the current stat interval
func (bs *BdnPerformanceStats) EndTime() time.Time {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	return bs.intervalEndTime
}

// Memory returns memory utilization stat
func (bs *BdnPerformanceStats) Memory() uint16 {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	return bs.memoryUtilizationMb
}

// BurstLimitedTransactionsPaid returns relay burst limited transactions paid stat
func (bs *BdnPerformanceStats) BurstLimitedTransactionsPaid() uint16 {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	return bs.burstLimitedTransactionsPaid
}

// BurstLimitedTransactionsUnpaid returns relay burst limited transactions unpaid stat
func (bs *BdnPerformanceStats) BurstLimitedTransactionsUnpaid() uint16 {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	return bs.burstLimitedTransactionsUnpaid
}

// NodeStats returns the bdn stats data for all nodes
func (bs *BdnPerformanceStats) NodeStats() map[string]*BdnPerformanceStatsData {
	bs.lock.Lock()
	defer bs.lock.Unlock()
	return bs.nodeStats
}

// SetNodeStats sets the bdn stats data
// NOTE: use only in the tests
func (bs *BdnPerformanceStats) SetNodeStats(nodeIPPort string, stats *BdnPerformanceStatsData) {
	bs.lock.Lock()
	defer bs.lock.Unlock()

	bs.nodeStats[nodeIPPort] = stats
}

func (bs *BdnPerformanceStats) getNodeStats(node types.NodeEndpoint) (*BdnPerformanceStatsData, error) {
	stats, ok := bs.nodeStats[node.IPPort()]
	if ok {
		return stats, nil
	}
	return nil, fmt.Errorf("unable to find BDN stats for node %v", node.IPPort())
}

func (bs *BdnPerformanceStats) getOrCreateNodeStats(node types.NodeEndpoint) *BdnPerformanceStatsData {
	stats, ok := bs.nodeStats[node.IPPort()]
	if !ok {
		// right now we are not supporting beacon inbound connections, so for ETH inbound will be false and for BSC it will be true
		stats = &BdnPerformanceStatsData{Dynamic: true, IsConnected: true, IsBeacon: node.IsBeacon, BlockchainNetwork: node.BlockchainNetwork}
		if !node.Dynamic {
			log.Debugf("override the dynamic to true for node %v", node.IPPort())
		}
		bs.nodeStats[node.IPPort()] = stats
	}
	stats.IsConnected = true
	return stats
}

// Pack serializes a BdnPerformanceStats into a buffer for sending
func (bs *BdnPerformanceStats) Pack(_ Protocol) ([]byte, error) {
	bs.lock.Lock()
	defer bs.lock.Unlock()

	bufLen := bs.size()
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
		log.Tracef("sending bdnperformancestats with %v, %v, %v", endpoint, nodeStats.IsConnected, nodeStats.Dynamic)
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

		if nodeStats.IsBeacon {
			copy(buf[offset:], []uint8{1})
		} else {
			copy(buf[offset:], []uint8{0})
		}
		offset += IsBeaconLen
		if nodeStats.IsConnected {
			copy(buf[offset:], []uint8{1})
		} else {
			copy(buf[offset:], []uint8{0})
		}
		offset++

		if nodeStats.Dynamic {
			copy(buf[offset:], []uint8{1})
		} else {
			copy(buf[offset:], []uint8{0})
		}
		offset++
	}
	binary.LittleEndian.PutUint16(buf[offset:], bs.burstLimitedTransactionsPaid)
	offset += types.UInt16Len
	binary.LittleEndian.PutUint16(buf[offset:], bs.burstLimitedTransactionsUnpaid)
	offset += types.UInt16Len
	if err := checkBuffEnd(&buf, int(offset)); err != nil {
		return nil, err
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

	nodeStatsSize := int(nodesStatsLen)*((utils.IPAddrSizeInBytes)+(types.UInt32Len*7)+(types.UInt16Len*3)) + IsBeaconLen + (int(nodesStatsLen) * 2)
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
		singleNodeStats.IsBeacon = int(buf[offset : offset+IsBeaconLen][0]) != 0
		offset += IsBeaconLen
		singleNodeStats.IsConnected = int(buf[offset]) != 0
		offset++

		singleNodeStats.Dynamic = int(buf[offset]) != 0
		if singleNodeStats.Dynamic && singleNodeStats.IsConnected {
			bs.dynamicConnections++
		} else if !singleNodeStats.IsBeacon && singleNodeStats.IsConnected {
			bs.staticConnections++
		}
		offset++

		if endpoint.IPPort() != emptyEndpoint.IPPort() {
			bs.nodeStats[endpoint.IPPort()] = &singleNodeStats
		}
	}
	if err := checkBufSize(&buf, int(offset), 2*types.UInt16Len); err != nil {
		return err
	}
	bs.burstLimitedTransactionsPaid = binary.LittleEndian.Uint16(buf[offset:])
	offset += types.UInt16Len
	bs.burstLimitedTransactionsUnpaid = binary.LittleEndian.Uint16(buf[offset:])
	return bs.Header.Unpack(buf, protocol)
}

// Log logs stats
func (bs *BdnPerformanceStats) Log() {
	for endpoint, nodeStats := range bs.NodeStats() {
		log.Infof("%v [%v - %v]: Received %v blocks and %v transactions from the BDN. Received transactions from node: %v, is connected %v", endpoint, bs.StartTime().Format(bxgateway.TimeLayoutISO), bs.EndTime().Format(bxgateway.TimeLayoutISO), nodeStats.NewBlocksReceivedFromBdn, nodeStats.NewTxReceivedFromBdn, nodeStats.NewTxReceivedFromBlockchainNode, nodeStats.IsConnected)
	}
}

func (bs *BdnPerformanceStats) size() uint32 {
	nodeStatsSize := utils.IPAddrSizeInBytes + (types.UInt32Len * 7) + (types.UInt16Len * 3) + IsBeaconLen + IsConnectedLen + IsDynamicLen
	total := bs.Header.Size() + uint32((types.UInt64Len*2)+(types.UInt16Len*2)+(len(bs.nodeStats)*nodeStatsSize))
	total += types.UInt16Len * 2
	return total
}

func ipPort(ip string, port int) string {
	return fmt.Sprintf("%v:%v", ip, port)
}
