package bxmessage

import (
	"testing"

	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// msg supports IsBeacon flag
var BDNStatsMsgBytesForIsBeacon = []byte("\xff\xfe\xfd\xfcbdnstats\x00\x00\x00\x00y\x00\x00\x00W\xe8\xf6\xde\x005\xd8A`\xea\xf6\xde\x005\xd8Ad\x00\x02\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\x7f\x00\x00\x01A\x1f\x14\x00\x1e\x00(\x00\x00\x002\x00\x00\x00\n\x00\x00\x00\n\x00\x00\x00\x14\x00\x00\x00d\x00\x00\x002\x00\x00\x00\x00\x01\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\xac\x11\x00\x01B\x1f\x15\x00\x1f\x00)\x00\x00\x003\x00\x00\x00\x0b\x00\x00\x00\x0b\x00\x00\x00\x15\x00\x00\x00e\x00\x00\x003\x00\x00\x00\x003\x00\x00\x14\x00\x00\x01")

func TestBdnPerformanceStats_Unpack(t *testing.T) {
	bdnStats := BdnPerformanceStats{}
	err := bdnStats.Unpack(BDNStatsMsgBytesForIsBeacon, MinProtocol)
	assert.NoError(t, err)
	assert.Equal(t, uint16(100), bdnStats.memoryUtilizationMb)
	assert.Equal(t, 2, len(bdnStats.nodeStats))
	ipEndpoint := types.NodeEndpoint{
		IP:   "127.0.0.1",
		Port: 8001,
	}
	inbound, outbound := bdnStats.GetConnectionsCount()
	assert.Equal(t, int64(0), inbound)
	assert.Equal(t, int64(2), outbound)

	nodeStats, _ := bdnStats.nodeStats[ipEndpoint.IPPort()]
	assert.Equal(t, uint16(20), nodeStats.NewBlocksReceivedFromBlockchainNode)
	assert.Equal(t, uint16(30), nodeStats.NewBlocksReceivedFromBdn)
	assert.Equal(t, uint32(40), nodeStats.NewTxReceivedFromBlockchainNode)
	assert.Equal(t, uint32(50), nodeStats.NewTxReceivedFromBdn)
	assert.Equal(t, uint32(10), nodeStats.NewBlocksSeen)
	assert.Equal(t, uint32(10), nodeStats.NewBlockMessagesFromBlockchainNode)
	assert.Equal(t, uint32(20), nodeStats.NewBlockAnnouncementsFromBlockchainNode)
	assert.Equal(t, uint32(100), nodeStats.TxSentToNode)
	assert.Equal(t, uint32(50), nodeStats.DuplicateTxFromNode)
	assert.Equal(t, uint16(13056), bdnStats.burstLimitedTransactionsPaid)
	assert.Equal(t, uint16(0), bdnStats.burstLimitedTransactionsUnpaid)
}

func TestBdnPerformanceStats_Pack_GatewayInboundConnections(t *testing.T) {
	bdnStatsFromBytes := BdnPerformanceStats{}
	err := bdnStatsFromBytes.Unpack(BDNStatsMsgBytesForIsBeacon, MinProtocol)
	require.NoError(t, err)
	inbound, outbound := bdnStatsFromBytes.GetConnectionsCount()
	assert.Equal(t, int64(0), inbound)
	assert.Equal(t, int64(2), outbound)

	endpoint1 := types.NodeEndpoint{IP: "127.0.0.1", Port: 8001}
	endpoint2 := types.NodeEndpoint{IP: "127.0.0.1", Port: 8002}
	endpoint3 := types.NodeEndpoint{IP: "127.0.0.1", Port: 8003}
	endpoint4 := types.NodeEndpoint{IP: "127.0.0.1", Port: 8004}
	blockchainPeers := []types.NodeEndpoint{endpoint1}

	bdnStats := NewBDNStats(blockchainPeers, make(map[string]struct{}))
	bdnStats.intervalStartTime = bdnStatsFromBytes.intervalStartTime
	bdnStats.intervalEndTime = bdnStatsFromBytes.intervalEndTime
	bdnStats.memoryUtilizationMb = 100
	nodeStats1 := BdnPerformanceStatsData{
		NewBlocksReceivedFromBlockchainNode:     20,
		NewBlocksReceivedFromBdn:                30,
		NewBlocksSeen:                           10,
		NewBlockMessagesFromBlockchainNode:      10,
		NewBlockAnnouncementsFromBlockchainNode: 20,
		NewTxReceivedFromBlockchainNode:         40,
		NewTxReceivedFromBdn:                    50,
		TxSentToNode:                            100,
		DuplicateTxFromNode:                     50,
		Dynamic:                                 true,
		IsConnected:                             true,
	}
	bdnStats.nodeStats[endpoint1.IPPort()] = &nodeStats1
	nodeStats3 := BdnPerformanceStatsData{
		NewBlocksReceivedFromBlockchainNode:     20,
		NewBlocksReceivedFromBdn:                30,
		NewBlocksSeen:                           10,
		NewBlockMessagesFromBlockchainNode:      10,
		NewBlockAnnouncementsFromBlockchainNode: 20,
		NewTxReceivedFromBlockchainNode:         40,
		NewTxReceivedFromBdn:                    50,
		TxSentToNode:                            100,
		DuplicateTxFromNode:                     50,
		Dynamic:                                 true,
		IsConnected:                             true,
	}
	bdnStats.nodeStats[endpoint3.IPPort()] = &nodeStats3
	nodeStats2 := BdnPerformanceStatsData{
		NewBlocksReceivedFromBlockchainNode:     21,
		NewBlocksReceivedFromBdn:                31,
		NewBlocksSeen:                           11,
		NewBlockMessagesFromBlockchainNode:      11,
		NewBlockAnnouncementsFromBlockchainNode: 21,
		NewTxReceivedFromBlockchainNode:         41,
		NewTxReceivedFromBdn:                    51,
		TxSentToNode:                            101,
		DuplicateTxFromNode:                     51,
		Dynamic:                                 false,
		IsConnected:                             true,
	}
	bdnStats.nodeStats[endpoint2.IPPort()] = &nodeStats2
	nodeStats4 := BdnPerformanceStatsData{
		NewBlocksReceivedFromBlockchainNode:     21,
		NewBlocksReceivedFromBdn:                31,
		NewBlocksSeen:                           11,
		NewBlockMessagesFromBlockchainNode:      11,
		NewBlockAnnouncementsFromBlockchainNode: 21,
		NewTxReceivedFromBlockchainNode:         41,
		NewTxReceivedFromBdn:                    51,
		TxSentToNode:                            101,
		DuplicateTxFromNode:                     51,
		Dynamic:                                 false,
		IsConnected:                             true,
	}
	bdnStats.nodeStats[endpoint4.IPPort()] = &nodeStats4

	// Inbound connections should be included
	packed, err := bdnStats.Pack(GatewayInboundConnections)
	require.NoError(t, err)

	bdnStatsFromBytes = BdnPerformanceStats{}
	err = bdnStatsFromBytes.Unpack(packed, GatewayInboundConnections)
	assert.NoError(t, err)
	inbound, outbound = bdnStatsFromBytes.GetConnectionsCount()

	assert.Equal(t, int64(2), inbound)
	assert.Equal(t, int64(2), outbound)
	assert.Equal(t, 4, len(bdnStatsFromBytes.nodeStats))
	assert.Equal(t, int64(2), bdnStatsFromBytes.dynamicConnections)
	assert.True(t, bdnStatsFromBytes.nodeStats[endpoint1.IPPort()].Dynamic)
	assert.False(t, bdnStatsFromBytes.nodeStats[endpoint2.IPPort()].Dynamic)

	// Inbound connections should be excluded, lower protocol version, in this case there are no inbound connection
	delete(bdnStats.nodeStats, endpoint1.IPPort())
	packed, err = bdnStats.Pack(GatewayInboundConnections)
	require.NoError(t, err)

	bdnStatsFromBytes = BdnPerformanceStats{}
	err = bdnStatsFromBytes.Unpack(packed, GatewayInboundConnections)
	assert.NoError(t, err)
	inbound, outbound = bdnStatsFromBytes.GetConnectionsCount()
	assert.Equal(t, int64(1), inbound)
	assert.Equal(t, int64(2), outbound)
	assert.Equal(t, 3, len(bdnStatsFromBytes.nodeStats))
	assert.False(t, bdnStatsFromBytes.nodeStats[endpoint2.IPPort()].Dynamic)
}

func TestBdnPerformanceStats_Pack_IsConnectedToGateway(t *testing.T) {
	bdnStatsFromBytes := BdnPerformanceStats{}
	err := bdnStatsFromBytes.Unpack(BDNStatsMsgBytesForIsBeacon, MinProtocol)
	require.NoError(t, err)

	endpoint1 := types.NodeEndpoint{IP: "127.0.0.1", Port: 8001}
	endpoint2 := types.NodeEndpoint{IP: "127.0.0.1", Port: 8002}
	blockchainPeers := []types.NodeEndpoint{endpoint1}

	bdnStats := NewBDNStats(blockchainPeers, make(map[string]struct{}))
	bdnStats.intervalStartTime = bdnStatsFromBytes.intervalStartTime
	bdnStats.intervalEndTime = bdnStatsFromBytes.intervalEndTime
	bdnStats.memoryUtilizationMb = 100
	nodeStats1 := BdnPerformanceStatsData{
		NewBlocksReceivedFromBlockchainNode:     20,
		NewBlocksReceivedFromBdn:                30,
		NewBlocksSeen:                           10,
		NewBlockMessagesFromBlockchainNode:      10,
		NewBlockAnnouncementsFromBlockchainNode: 20,
		NewTxReceivedFromBlockchainNode:         40,
		NewTxReceivedFromBdn:                    50,
		TxSentToNode:                            100,
		DuplicateTxFromNode:                     50,
		Dynamic:                                 false,
		IsConnected:                             true,
	}
	bdnStats.nodeStats[endpoint1.IPPort()] = &nodeStats1
	nodeStats2 := BdnPerformanceStatsData{
		NewBlocksReceivedFromBlockchainNode:     21,
		NewBlocksReceivedFromBdn:                31,
		NewBlocksSeen:                           11,
		NewBlockMessagesFromBlockchainNode:      11,
		NewBlockAnnouncementsFromBlockchainNode: 21,
		NewTxReceivedFromBlockchainNode:         41,
		NewTxReceivedFromBdn:                    51,
		TxSentToNode:                            101,
		DuplicateTxFromNode:                     51,
		Dynamic:                                 false,
		IsConnected:                             false,
	}
	bdnStats.nodeStats[endpoint2.IPPort()] = &nodeStats2

	// isConnected connections should be included
	packed, err := bdnStats.Pack(IsConnectedToGateway)
	require.NoError(t, err)

	bdnStatsFromBytes = BdnPerformanceStats{}
	err = bdnStatsFromBytes.Unpack(packed, IsConnectedToGateway)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(bdnStatsFromBytes.nodeStats))
	assert.True(t, bdnStatsFromBytes.nodeStats[endpoint1.IPPort()].IsConnected)
	assert.False(t, bdnStatsFromBytes.nodeStats[endpoint2.IPPort()].IsConnected)

	// IsConnected connections should be excluded, lower protocol version
	packed, err = bdnStats.Pack(IsConnectedToGateway)
	require.NoError(t, err)

	bdnStatsFromBytes = BdnPerformanceStats{}
	err = bdnStatsFromBytes.Unpack(packed, MinProtocol)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(bdnStatsFromBytes.nodeStats))
	assert.True(t, bdnStatsFromBytes.nodeStats[endpoint1.IPPort()].IsConnected)
	_, exits := bdnStatsFromBytes.nodeStats[endpoint2.IPPort()]
	assert.False(t, exits)
}

func TestBdnPerformanceStats_UnpackBadBuffer(t *testing.T) {
	bdnStats := BdnPerformanceStats{}
	// truncated msg bytes
	var bdnStatsMsgBytes = []byte("\xff\xfe\xfd\xfcbdnstats\x00\x00\x00\x00y\x00\x00\x00W\xe8\xf6\xde\x005\xd8A`\xea\xf6\xde\x005\xd8Ad\x00\x02\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\x7f\x00\x00\x01A\x1f\x14\x00\x1e\x00(\x00\x00\x002\x00\x00\x00\n\x00\x00\x00\n\x00\x00\x00\x14\x00\x00\x00d\x00\x00\x002\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\xac\x11\x00\x01B\x1f\x15\x00\x1f\x00)\x00\x00\x003\x00\x00\x00\x0b\x00\x00\x00\x0b\x00\x00\x00\x15\x00\x00\x00e\x00")
	err := bdnStats.Unpack(bdnStatsMsgBytes, 0)
	assert.NotNil(t, err)
}

func TestBdnPerformanceStats_Pack_IsBeaconProtocol(t *testing.T) {
	bdnStatsFromBytes := BdnPerformanceStats{}
	err := bdnStatsFromBytes.Unpack(BDNStatsMsgBytesForIsBeacon, IsBeaconProtocol)
	require.NoError(t, err)

	endpoint1 := types.NodeEndpoint{IP: "127.0.0.1", Port: 8001}
	endpoint2 := types.NodeEndpoint{IP: "127.0.0.1", Port: 8002}
	blockchainPeers := []types.NodeEndpoint{endpoint1}

	bdnStats := NewBDNStats(blockchainPeers, make(map[string]struct{}))
	bdnStats.intervalStartTime = bdnStatsFromBytes.intervalStartTime
	bdnStats.intervalEndTime = bdnStatsFromBytes.intervalEndTime
	bdnStats.memoryUtilizationMb = 100
	nodeStats1 := BdnPerformanceStatsData{
		NewBlocksReceivedFromBlockchainNode:     20,
		NewBlocksReceivedFromBdn:                30,
		NewBlocksSeen:                           10,
		NewBlockMessagesFromBlockchainNode:      10,
		NewBlockAnnouncementsFromBlockchainNode: 20,
		NewTxReceivedFromBlockchainNode:         40,
		NewTxReceivedFromBdn:                    50,
		TxSentToNode:                            100,
		DuplicateTxFromNode:                     50,
		Dynamic:                                 false,
		IsConnected:                             true,
		IsBeacon:                                true,
	}
	bdnStats.nodeStats[endpoint1.IPPort()] = &nodeStats1
	nodeStats2 := BdnPerformanceStatsData{
		NewBlocksReceivedFromBlockchainNode:     21,
		NewBlocksReceivedFromBdn:                31,
		NewBlocksSeen:                           11,
		NewBlockMessagesFromBlockchainNode:      11,
		NewBlockAnnouncementsFromBlockchainNode: 21,
		NewTxReceivedFromBlockchainNode:         41,
		NewTxReceivedFromBdn:                    51,
		TxSentToNode:                            101,
		DuplicateTxFromNode:                     51,
		Dynamic:                                 false,
		IsConnected:                             true,
		IsBeacon:                                false,
	}
	bdnStats.nodeStats[endpoint2.IPPort()] = &nodeStats2
	bdnStats.burstLimitedTransactionsPaid = 51
	bdnStats.burstLimitedTransactionsUnpaid = 20

	packed, err := bdnStats.Pack(IsBeaconProtocol)
	require.NoError(t, err)

	err = bdnStatsFromBytes.Unpack(packed, IsBeaconProtocol)
	assert.NoError(t, err)
	assert.Equal(t, bdnStats.intervalStartTime, bdnStatsFromBytes.intervalStartTime)
	assert.Equal(t, bdnStats.intervalEndTime, bdnStatsFromBytes.intervalEndTime)
	assert.Equal(t, bdnStats.memoryUtilizationMb, bdnStatsFromBytes.memoryUtilizationMb)
	numNodeStats := 0
	for endpoint, nodeStats := range bdnStatsFromBytes.nodeStats {
		if endpoint == endpoint1.IPPort() {
			numNodeStats++
			assert.Equal(t, endpoint, endpoint1.IPPort())
			assert.Equal(t, nodeStats.NewBlocksReceivedFromBlockchainNode, nodeStats1.NewBlocksReceivedFromBlockchainNode)
			assert.Equal(t, nodeStats.NewBlocksReceivedFromBdn, nodeStats1.NewBlocksReceivedFromBdn)
			assert.Equal(t, nodeStats.NewBlocksSeen, nodeStats1.NewBlocksSeen)
			assert.Equal(t, nodeStats.NewBlockMessagesFromBlockchainNode, nodeStats1.NewBlockMessagesFromBlockchainNode)
			assert.Equal(t, nodeStats.NewBlockAnnouncementsFromBlockchainNode, nodeStats1.NewBlockAnnouncementsFromBlockchainNode)
			assert.Equal(t, nodeStats.NewTxReceivedFromBlockchainNode, nodeStats1.NewTxReceivedFromBlockchainNode)
			assert.Equal(t, nodeStats.NewTxReceivedFromBdn, nodeStats1.NewTxReceivedFromBdn)
			assert.Equal(t, nodeStats.TxSentToNode, nodeStats1.TxSentToNode)
			assert.Equal(t, nodeStats.DuplicateTxFromNode, nodeStats1.DuplicateTxFromNode)
			assert.True(t, nodeStats.IsBeacon)
		}
		if endpoint == endpoint2.IPPort() {
			numNodeStats++
			assert.Equal(t, endpoint, endpoint2.IPPort())
			assert.Equal(t, nodeStats.NewBlocksReceivedFromBlockchainNode, nodeStats2.NewBlocksReceivedFromBlockchainNode)
			assert.Equal(t, nodeStats.NewBlocksReceivedFromBdn, nodeStats2.NewBlocksReceivedFromBdn)
			assert.Equal(t, nodeStats.NewBlocksSeen, nodeStats2.NewBlocksSeen)
			assert.Equal(t, nodeStats.NewBlockMessagesFromBlockchainNode, nodeStats2.NewBlockMessagesFromBlockchainNode)
			assert.Equal(t, nodeStats.NewBlockAnnouncementsFromBlockchainNode, nodeStats2.NewBlockAnnouncementsFromBlockchainNode)
			assert.Equal(t, nodeStats.NewTxReceivedFromBlockchainNode, nodeStats2.NewTxReceivedFromBlockchainNode)
			assert.Equal(t, nodeStats.NewTxReceivedFromBdn, nodeStats2.NewTxReceivedFromBdn)
			assert.Equal(t, nodeStats.TxSentToNode, nodeStats2.TxSentToNode)
			assert.Equal(t, nodeStats.DuplicateTxFromNode, nodeStats2.DuplicateTxFromNode)
			assert.False(t, nodeStats.IsBeacon)
		}
	}
	assert.Equal(t, 2, numNodeStats)
	assert.Equal(t, uint16(51), bdnStats.burstLimitedTransactionsPaid)
	assert.Equal(t, uint16(20), bdnStats.burstLimitedTransactionsUnpaid)
}
