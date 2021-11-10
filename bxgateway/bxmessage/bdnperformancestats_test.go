package bxmessage

import (
	"github.com/bloXroute-Labs/gateway/bxgateway/types"
	"github.com/stretchr/testify/assert"
	"testing"
)

// msg bytes generated by python gateway BDNPerformanceStatsMessage serialization
var BDNStatsMsgBytes = []byte("\xff\xfe\xfd\xfcbdnstats\x00\x00\x00\x00y\x00\x00\x00W\xe8\xf6\xde\x005\xd8A`\xea\xf6\xde\x005\xd8Ad\x00\x02\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\x7f\x00\x00\x01A\x1f\x14\x00\x1e\x00(\x00\x00\x002\x00\x00\x00\n\x00\x00\x00\n\x00\x00\x00\x14\x00\x00\x00d\x00\x00\x002\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\xac\x11\x00\x01B\x1f\x15\x00\x1f\x00)\x00\x00\x003\x00\x00\x00\x0b\x00\x00\x00\x0b\x00\x00\x00\x15\x00\x00\x00e\x00\x00\x003\x00\x00\x00\x01")

func TestBdnPerformanceStats_Unpack(t *testing.T) {
	bdnStats := BdnPerformanceStats{}
	err := bdnStats.Unpack(BDNStatsMsgBytes, 0)
	assert.Nil(t, err)
	assert.Equal(t, uint16(100), bdnStats.memoryUtilizationMb)
	assert.Equal(t, 2, bdnStats.nodeStats.Count())
	ipEndpoint := types.NodeEndpoint{
		IP:   "127.0.0.1",
		Port: 8001,
	}

	val, _ := bdnStats.nodeStats.Get(ipEndpoint.String())
	nodeStats := val.(*BdnPerformanceStatsData)
	assert.Equal(t, ipEndpoint, nodeStats.BlockchainNodeIPEndpoint)
	assert.Equal(t, uint16(20), nodeStats.NewBlocksReceivedFromBlockchainNode)
	assert.Equal(t, uint16(30), nodeStats.NewBlocksReceivedFromBdn)
	assert.Equal(t, uint32(40), nodeStats.NewTxReceivedFromBlockchainNode)
	assert.Equal(t, uint32(50), nodeStats.NewTxReceivedFromBdn)
	assert.Equal(t, uint32(10), nodeStats.NewBlocksSeen)
	assert.Equal(t, uint32(10), nodeStats.NewBlockMessagesFromBlockchainNode)
	assert.Equal(t, uint32(20), nodeStats.NewBlockAnnouncementsFromBlockchainNode)
	assert.Equal(t, uint32(100), nodeStats.TxSentToNode)
	assert.Equal(t, uint32(50), nodeStats.DuplicateTxFromNode)

	ipEndpoint2 := types.NodeEndpoint{
		IP:   "172.17.0.1",
		Port: 8002,
	}
	val2, _ := bdnStats.nodeStats.Get(ipEndpoint2.String())
	nodeStats2 := val2.(*BdnPerformanceStatsData)
	assert.Equal(t, ipEndpoint2, nodeStats2.BlockchainNodeIPEndpoint)
	assert.Equal(t, uint16(21), nodeStats2.NewBlocksReceivedFromBlockchainNode)
	assert.Equal(t, uint16(31), nodeStats2.NewBlocksReceivedFromBdn)
	assert.Equal(t, uint32(41), nodeStats2.NewTxReceivedFromBlockchainNode)
	assert.Equal(t, uint32(51), nodeStats2.NewTxReceivedFromBdn)
	assert.Equal(t, uint32(11), nodeStats2.NewBlocksSeen)
	assert.Equal(t, uint32(11), nodeStats2.NewBlockMessagesFromBlockchainNode)
	assert.Equal(t, uint32(21), nodeStats2.NewBlockAnnouncementsFromBlockchainNode)
	assert.Equal(t, uint32(101), nodeStats2.TxSentToNode)
	assert.Equal(t, uint32(51), nodeStats2.DuplicateTxFromNode)
}

func TestBdnPerformanceStats_Pack(t *testing.T) {
	bdnStatsFromBytes := BdnPerformanceStats{}
	err := bdnStatsFromBytes.Unpack(BDNStatsMsgBytes, 0)

	bdnStats := NewBDNStats()
	bdnStats.intervalStartTime = bdnStatsFromBytes.intervalStartTime
	bdnStats.intervalEndTime = bdnStatsFromBytes.intervalEndTime
	bdnStats.memoryUtilizationMb = 100
	nodeStats1 := BdnPerformanceStatsData{
		BlockchainNodeIPEndpoint:                types.NodeEndpoint{IP: "127.0.0.1", Port: 8001},
		NewBlocksReceivedFromBlockchainNode:     20,
		NewBlocksReceivedFromBdn:                30,
		NewBlocksSeen:                           10,
		NewBlockMessagesFromBlockchainNode:      10,
		NewBlockAnnouncementsFromBlockchainNode: 20,
		NewTxReceivedFromBlockchainNode:         40,
		NewTxReceivedFromBdn:                    50,
		TxSentToNode:                            100,
		DuplicateTxFromNode:                     50,
	}
	bdnStats.nodeStats.Set(nodeStats1.BlockchainNodeIPEndpoint.String(), &nodeStats1)

	nodeStats2 := BdnPerformanceStatsData{
		BlockchainNodeIPEndpoint:                types.NodeEndpoint{IP: "172.17.0.1", Port: 8002},
		NewBlocksReceivedFromBlockchainNode:     21,
		NewBlocksReceivedFromBdn:                31,
		NewBlocksSeen:                           11,
		NewBlockMessagesFromBlockchainNode:      11,
		NewBlockAnnouncementsFromBlockchainNode: 21,
		NewTxReceivedFromBlockchainNode:         41,
		NewTxReceivedFromBdn:                    51,
		TxSentToNode:                            101,
		DuplicateTxFromNode:                     51,
	}
	bdnStats.nodeStats.Set(nodeStats2.BlockchainNodeIPEndpoint.String(), &nodeStats2)
	packed, err := bdnStats.Pack(0)

	assert.Equal(t, BDNStatsMsgBytes, packed)
	assert.Nil(t, err)
}

func TestBdnPerformanceStats_UnpackBadBuffer(t *testing.T) {
	bdnStats := BdnPerformanceStats{}
	// truncated msg bytes
	var bdnStatsMsgBytes = []byte("\xff\xfe\xfd\xfcbdnstats\x00\x00\x00\x00y\x00\x00\x00W\xe8\xf6\xde\x005\xd8A`\xea\xf6\xde\x005\xd8Ad\x00\x02\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\x7f\x00\x00\x01A\x1f\x14\x00\x1e\x00(\x00\x00\x002\x00\x00\x00\n\x00\x00\x00\n\x00\x00\x00\x14\x00\x00\x00d\x00\x00\x002\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\xac\x11\x00\x01B\x1f\x15\x00\x1f\x00)\x00\x00\x003\x00\x00\x00\x0b\x00\x00\x00\x0b\x00\x00\x00\x15\x00\x00\x00e\x00")
	err := bdnStats.Unpack(bdnStatsMsgBytes, 0)
	assert.NotNil(t, err)
}
