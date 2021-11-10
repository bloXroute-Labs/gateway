package utils

import (
	"github.com/bloXroute-Labs/gateway/bxgateway/bxmessage"
	"testing"
)

func TestPriorityQueue(t *testing.T) {
	var listItems []bxmessage.Message
	priorities := []bxmessage.SendPriority{10, 20, 15, 100, 1, 0, 15}
	// create by priorities
	for _, priority := range priorities {
		item := &bxmessage.Tx{}
		item.SetPriority(priority)
		listItems = append(listItems, item)
	}

	msgPQ := NewMsgPriorityQueue(nil, 0)
	for _, item := range listItems {
		msgPQ.Push(item)
	}
	for msgPQ.len() > 0 {
		msg := msgPQ.pop()
		t.Logf("Priority: %d", msg.GetPriority())
	}

	//goland:noinspection GoNilness
	msgPQ.Push(listItems[0])
	for i := 1; i < len(listItems)-1; i++ {
		msg := msgPQ.Pop()
		min := msg.GetPriority()
		msgPQ.Push(msg)
		msgPQ.Push(listItems[i])
		if listItems[i].GetPriority() < min {
			min = listItems[i].GetPriority()
		}
		msg = msgPQ.Pop()
		t.Logf("got %v, min %v", msg.GetPriority(), min)
		if msg.GetPriority() != min {
			t.Errorf("Got wrong entry ")
		}
	}
	msg := msgPQ.Pop()
	if msg.GetPriority() != 100 {
		t.Errorf("Got wrong last entry")
	}
}
