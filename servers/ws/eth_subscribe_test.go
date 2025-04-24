package ws

import (
	"encoding/json"
	"fmt"
	"time"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	"github.com/stretchr/testify/require"

	"github.com/ethereum/go-ethereum/common"

	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// newHeadsResponseParams - response of the jsonrpc params
type newHeadsResponseParams struct {
	Subscription string              `json:"subscription"`
	Result       types.NewHeadsBlock `json:"result"`
}

func (s *wsSuite) TestEthSubscribe() {
	wsProvider, ok := s.nodeWSManager.Provider(&s.blockchainPeers[0])
	s.Require().True(ok)
	s.Assert().Equal(wsProvider.BlockchainPeerEndpoint(), s.blockchainPeers[0])

	unsubscribeFilter, subscriptionID := s.assertEthSubscribe(`{"id": "1", "method": "eth_subscribe", "params": ["newHeads"]}`)
	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	feedNotification, _ := types.NewEthBlockNotification(ethBlock.Hash(), ethBlock, nil, false)
	feedNotification.SetNotificationType(types.NewBlocksFeed)
	sourceEndpoint := types.NodeEndpoint{IP: s.blockchainPeers[0].IP, Port: s.blockchainPeers[0].Port, BlockchainNetwork: bxtypes.Mainnet}
	feedNotification.SetSource(&sourceEndpoint)
	s.Assert().True(s.nodeWSManager.Synced())

	s.feedManager.Notify(feedNotification)

	time.Sleep(time.Millisecond)

	params := s.getClientSubscribeResponseParams()

	var m newHeadsResponseParams
	err := json.Unmarshal(params, &m)
	s.Require().NoError(err)
	s.Assert().Equal(m.Result.BlockHash.String(), ethBlock.Hash().String())

	s.writeMsgToWsAndReadResponse([]byte(unsubscribeFilter), nil)
	time.Sleep(time.Millisecond)

	s.Assert().False(s.feedManager.SubscriptionExists(subscriptionID))
	s.handlePingRequest()
}

func (s *wsSuite) assertEthSubscribe(filter string) (string, string) {
	subscribeMsg := s.writeMsgToWsAndReadResponse([]byte(filter), nil)
	clientRes := s.getClientResponse(subscribeMsg)
	subscriptionID := fmt.Sprintf("%v", clientRes.Result)

	require.Eventually(s.T(), func() bool {
		return s.feedManager.SubscriptionExists(subscriptionID)
	}, 1*time.Second, 10*time.Millisecond)
	return fmt.Sprintf(
		`{"id": 1, "method": "eth_unsubscribe", "params": ["%v"]}`,
		subscriptionID,
	), subscriptionID
}
