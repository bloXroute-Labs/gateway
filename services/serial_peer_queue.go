package services

import (
	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
)

// SerialPeerQueue - queue of messages to process
type SerialPeerQueue struct {
	queues *syncmap.SyncMap[types.NodeID, MessageQueue]
}

// NewSerialPeerQueue - create SerialPeerQueue object
func NewSerialPeerQueue() SerialPeerQueue {
	return SerialPeerQueue{queues: syncmap.NewTypedMapOf[types.NodeID, MessageQueue](syncmap.NodeIDHasher)}
}

// AddPeer - add peer to queue
func (q *SerialPeerQueue) AddPeer(peerID types.NodeID, callback MessageQueueCallback) {
	if q.queues.Has(peerID) {
		log.Errorf("message queue for peer %s already exists", peerID)
		return
	}

	newMessageQueue := NewMsgQueue(1, bxgateway.ParallelQueueChannelSize, callback)
	q.queues.Store(peerID, newMessageQueue)
}

// RemovePeer - remove peer from queue
func (q *SerialPeerQueue) RemovePeer(peerID types.NodeID) {
	queue, ok := q.queues.Load(peerID)
	if !ok {
		log.Errorf("message queue for peer %s does not exist", peerID)
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
		log.Errorf("message queue for peer %s does not exist", nodeID)
		return nil
	}

	return queue.Insert(msg, source)
}
