package ws

import (
	"time"
)

func (s *wsSuite) TestSubscribe() {
	unsubscribeMessage, subscriptionID := s.assertSubscribe(`{"id": "1", "method": "subscribe", "params": ["newTxs", {"include": ["tx_hash"]}]}`)
	s.writeMsgToWsAndReadResponse([]byte(unsubscribeMessage), nil)
	time.Sleep(time.Millisecond)
	s.Assert().False(s.feedManager.SubscriptionExists(subscriptionID))
	s.handlePingRequest()
}

func (s *wsSuite) TestTxReceiptsSubscribe() {
	unsubscribeMessage, subscriptionID := s.assertSubscribe(`{"id": "1", "method": "subscribe", "params": ["txReceipts", {"include": []}]}`)
	s.writeMsgToWsAndReadResponse([]byte(unsubscribeMessage), nil)
	time.Sleep(time.Millisecond)
	s.Assert().False(s.feedManager.SubscriptionExists(subscriptionID))
	s.handlePingRequest()
}

func (s *wsSuite) TestInvalidSubscribe() {
	subscribeMsg := s.writeMsgToWsAndReadResponse([]byte(`{"id": "1", "method": "subscribe", "para": ["txReceipts", {"include": []}]}`), nil)
	clientRes := s.getClientResponse(subscribeMsg)
	s.Assert().Nil(clientRes.Result)
	s.Assert().NotNil(clientRes.Error)
	time.Sleep(time.Millisecond)
	s.handlePingRequest()
}
