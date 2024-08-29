package ws

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/test/fixtures"
)

func (s *wsSuite) TestPing() {
	s.handlePingRequest()
}

func (s *wsSuite) handlePingRequest() {
	timeClientSendsRequest := time.Now().UTC()
	msg := s.writeMsgToWsAndReadResponse([]byte(`{"id": "1", "method": "ping"}`), nil)
	timeClientReceivesResponse := time.Now().UTC()

	clientRes := s.getClientResponse(msg)
	b, err := json.Marshal(clientRes.Result)
	s.Require().NoError(err)
	var res rpcPingResponse
	err = json.Unmarshal(b, &res)
	s.Require().NoError(err)

	timeServerReceivesRequest, err := time.Parse(bxgateway.MicroSecTimeFormat, res.Pong)
	s.Require().NoError(err)
	s.Assert().True(timeClientReceivesResponse.After(timeServerReceivesRequest))
	s.Assert().True(timeServerReceivesRequest.After(timeClientSendsRequest))
}

func (s *wsSuite) TestQuotaUsage() {
	msg := s.writeMsgToWsAndReadResponse([]byte(`{"id": "1", "method": "quota_usage"}`), nil)
	clientRes := s.getClientResponse(msg)

	b, err := json.Marshal(clientRes.Result)
	s.Require().NoError(err)

	var res connections.QuotaResponseBody
	err = json.Unmarshal(b, &res)
	s.Require().NoError(err)
	s.Assert().Equal("gw", res.AccountID)
	s.Assert().Equal(1, res.QuotaFilled)
	s.Assert().Equal(2, res.QuotaLimit)
}

func (s *wsSuite) TestNonBloxrouteRPCMethods() {
	s.Require().True(s.nodeWSManager.Synced())
	ws1, _ := s.nodeWSManager.Provider(&s.blockchainPeers[0])
	ws2, _ := s.nodeWSManager.Provider(&s.blockchainPeers[1])
	ws3, _ := s.nodeWSManager.Provider(&s.blockchainPeers[2])
	s.Require().Equal(blockchain.Synced, ws1.SyncStatus())
	ws2.UpdateSyncStatus(blockchain.Unsynced)
	ws3.UpdateSyncStatus(blockchain.Unsynced)

	request := `{"jsonrpc": "2.0", "id": "1", "method": "eth_getBalance", "params": ["0xAABCf4f110F06aFd82A7696f4fb79AE4a41D0f81", "latest"]}`
	response := s.writeMsgToWsAndReadResponse([]byte(request), nil)
	clientRes := s.getClientResponse(response)

	s.Assert().Equal(1, ws1.(*eth.MockWSProvider).NumRPCCalls())
	s.Assert().Equal("response", clientRes.Result)
	s.markAllPeersWithSyncStatus(blockchain.Synced)
}

func (s *wsSuite) TestNonBloxrouteSendTxMethod() {
	s.Require().True(s.nodeWSManager.Synced())
	ws1, _ := s.nodeWSManager.Provider(&s.blockchainPeers[0])
	ws2, _ := s.nodeWSManager.Provider(&s.blockchainPeers[1])
	ws3, _ := s.nodeWSManager.Provider(&s.blockchainPeers[2])
	s.Require().Equal(blockchain.Synced, ws1.SyncStatus())
	ws2.UpdateSyncStatus(blockchain.Unsynced)
	ws3.UpdateSyncStatus(blockchain.Unsynced)

	request := fmt.Sprintf(`{"jsonrpc": "2.0", "id": "1", "method": "eth_sendRawTransaction", "params": ["0x%v"]}`, fixtures.DynamicFeeTransaction)
	response := s.writeMsgToWsAndReadResponse([]byte(request), nil)
	clientRes := s.getClientResponse(response)
	hashRes := clientRes.Result.(string)
	s.Assert().Equal("0x"+fixtures.DynamicFeeTransactionHash[2:], hashRes)
	s.Assert().Nil(clientRes.Error)
	s.markAllPeersWithSyncStatus(blockchain.Synced)
}
