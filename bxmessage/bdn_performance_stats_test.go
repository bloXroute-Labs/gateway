package bxmessage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

var BDNStatsMsgBytes = []byte("\xff\xfe\xfd\xfcbdnstats\x00\x00\x00\x00\x83\x00\x00\x00W\xe8\xf6\xde\x005\xd8A`\xea\xf6\xde\x005\xd8Ad\x00\x02\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\x7f\x00\x00\x01A\x1f\x14\x00\x1e\x00(\x00\x00\x002\x00\x00\x00\x0a\x00\x00\x00\x0a\x00\x00\x00\x14\x00\x00\x00d\x00\x00\x002\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\x7f\x00\x00\x01B\x1f\x15\x00\x1f\x00)\x00\x00\x003\x00\x00\x00\x0b\x00\x00\x00\x0b\x00\x00\x00\x15\x00\x00\x00e\x00\x00\x003\x00\x00\x00\x01\x01\x01\x00\x00\x14\x00\x01")

func TestBdnPerformanceStats_Unpack(t *testing.T) {
	bdnStats := BdnPerformanceStats{}
	err := bdnStats.Unpack(BDNStatsMsgBytes, MinProtocol)
	assert.NoError(t, err)
	assert.Equal(t, uint16(100), bdnStats.memoryUtilizationMb)
	assert.Equal(t, 2, len(bdnStats.nodeStats))
	ipEndpoint := types.NodeEndpoint{
		IP:   "127.0.0.1",
		Port: 8001,
	}
	inbound, outbound := bdnStats.GetConnectionsCount()
	assert.Equal(t, int64(1), inbound)
	assert.Equal(t, int64(1), outbound)

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
	assert.Equal(t, uint16(0), bdnStats.burstLimitedTransactionsPaid)
	assert.Equal(t, uint16(20), bdnStats.burstLimitedTransactionsUnpaid)
}

func TestBdnPerformanceStats_Pack(t *testing.T) {
	bdnStatsFromBytes := BdnPerformanceStats{}
	err := bdnStatsFromBytes.Unpack(BDNStatsMsgBytes, MinProtocol)
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
	bdnStats.burstLimitedTransactionsPaid = bdnStatsFromBytes.burstLimitedTransactionsPaid
	bdnStats.burstLimitedTransactionsUnpaid = bdnStatsFromBytes.burstLimitedTransactionsUnpaid

	packed, err := bdnStats.Pack(MinProtocol)
	require.NoError(t, err)

	err = bdnStatsFromBytes.Unpack(packed, MinProtocol)
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
	assert.Equal(t, uint16(0), bdnStats.burstLimitedTransactionsPaid)
	assert.Equal(t, uint16(20), bdnStats.burstLimitedTransactionsUnpaid)
}
