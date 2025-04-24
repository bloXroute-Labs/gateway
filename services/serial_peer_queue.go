package services

import (
	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/syncmap"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/utils/hasher"
)

// SerialPeerQueue - queue of messages to process
type SerialPeerQueue struct {
	queues *syncmap.SyncMap[bxtypes.NodeID, *MessageQueue]
}

// NewSerialPeerQueue - create SerialPeerQueue object
func NewSerialPeerQueue() SerialPeerQueue {
	return SerialPeerQueue{queues: syncmap.NewTypedMapOf[bxtypes.NodeID, *MessageQueue](hasher.NodeIDHasher)}
}

// AddPeer - add peer to queue
func (q *SerialPeerQueue) AddPeer(peerID bxtypes.NodeID, callback MessageQueueCallback) {
	if q.queues.Has(peerID) {
		log.Errorf("failed to add peer %s to message queue, message queue for peer already exists", peerID)
		return
	}

	newMessageQueue := NewMsgQueue(1, bxgateway.ParallelQueueChannelSize, callback)
	q.queues.Store(peerID, newMessageQueue)
}

// RemovePeer - remove peer from queue
func (q *SerialPeerQueue) RemovePeer(peerID bxtypes.NodeID) {
	queue, ok := q.queues.Load(peerID)
	if !ok {
		return
	}

	q.queues.Delete(peerID)
	queue.Stop()
}

// AddMessage - add message to queue
func (q *SerialPeerQueue) AddMessage(msg bxmessage.Message, source connections.Conn) error {
	nodeID := source.GetNodeID()
	queue, ok := q.queues.Load(nodeID)
	if !ok {
		log.Errorf("failed to add message to message queue for peer %s, message queue for peer does not exist", nodeID)
		return nil
	}

	return queue.Insert(msg, source)
}
