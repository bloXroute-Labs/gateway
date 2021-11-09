package utils

import (
	"container/heap"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/bxmessage"
	"sync"
	"time"
)

// MsgPriorityQueue hold messages by message priority
type MsgPriorityQueue struct {
	lock             sync.Mutex
	callBack         func(bxmessage.Message)
	callBackInterval time.Duration
	storage          items
	lastPopTime      time.Time
}

// PriorityQueueItem holds message to be sent by priority and time that message is pushed onto queue
type PriorityQueueItem struct {
	msg          bxmessage.Message
	timeReceived time.Time
}

// NewMsgPriorityQueue creates an empty, initialized priorityQueue heap for messages
func NewMsgPriorityQueue(callBack func(bxmessage.Message), callBackInterval time.Duration) *MsgPriorityQueue {
	pq := &MsgPriorityQueue{
		callBack:         callBack,
		callBackInterval: callBackInterval,
		storage:          make(items, 0),
	}
	heap.Init(&pq.storage)
	return pq
}

// Len provides the number of messages in the priority queue
func (pq *MsgPriorityQueue) Len() int {
	pq.lock.Lock()
	defer pq.lock.Unlock()
	return pq.len()
}

func (pq *MsgPriorityQueue) len() int {
	// should be called with lock held
	return pq.storage.Len()
}
func (pq *MsgPriorityQueue) push(item PriorityQueueItem) {
	// should be called with lock held
	heap.Push(&pq.storage, item)
}

// Push adds new message to the priority queue
func (pq *MsgPriorityQueue) Push(msg bxmessage.Message) {
	pq.lock.Lock()
	defer pq.lock.Unlock()

	item := PriorityQueueItem{msg, time.Now()}

	// if no callback needed, place on PQ and leave
	if pq.callBack == nil || pq.callBackInterval == 0 {
		pq.push(item)
		return
	}
	// if pq is empty we might skip the priority queue
	if pq.len() == 0 && time.Now().Sub(pq.lastPopTime) > pq.callBackInterval {
		go pq.callBack(item.msg)
		pq.lastPopTime = time.Now()
		return
	}
	pq.push(item)
	// once there is something on pq we need a gorouting to clean it
	if pq.len() == 1 {
		go func() {
			ticker := time.NewTicker(pq.callBackInterval)
			for {
				select {
				case <-ticker.C:
					pq.lock.Lock()
					msg := pq.pop()
					pq.lastPopTime = time.Now()
					empty := pq.len() == 0
					pq.lock.Unlock()
					go pq.callBack(msg)
					// if pq becomes empty we quite
					if empty {
						ticker.Stop()
						return
					}
				}
			}
		}()
	}
}

func (pq *MsgPriorityQueue) pop() bxmessage.Message {
	// should be called with lock held
	if pq.storage.Len() == 0 {
		return nil
	}
	return heap.Pop(&pq.storage).(PriorityQueueItem).msg
}

// Pop extract the best message from the priority queue
func (pq *MsgPriorityQueue) Pop() bxmessage.Message {
	pq.lock.Lock()
	defer pq.lock.Unlock()
	return pq.pop()
}

// items holds message to be sent by priority and time received
type items []PriorityQueueItem

// Len is the size of the queue
func (items items) Len() int { return len(items) }

// Less provides the lower priority entry
func (items items) Less(i, j int) bool {
	// We want Pop to give us the highest priority item (lower number) that was received first
	if items[i].msg.GetPriority() != items[j].msg.GetPriority() {
		return items[i].msg.GetPriority() < items[j].msg.GetPriority()
	}
	return items[i].timeReceived.Before(items[j].timeReceived)
}

// Pop provides the current lowest priority
func (items *items) Pop() interface{} {
	old := *items
	n := len(old)
	item := old[n-1]
	*items = old[0 : n-1]
	return item
}

// Push is used to add new entry
func (items *items) Push(x interface{}) {
	item := x.(PriorityQueueItem)
	*items = append(*items, item)
}

// Swap is used to swp too entries
func (items items) Swap(i, j int) {
	items[i], items[j] = items[j], items[i]
}
