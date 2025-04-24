package ws

import (
	"encoding/json"
	"fmt"
	"time"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/test/fixtures"
)

func (s *wsSuite) TestBlxrTxEnsureNodeValidation() {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s", "node_validation": true}}`, fixtures.LegacyTransaction)
	msg := s.writeMsgToWsAndReadResponse([]byte(reqPayload), nil)
	clientRes := s.getClientResponse(msg)
	res := s.parseBlxrTxResult(clientRes.Result)
	s.Require().Nil(clientRes.Error)
	s.Assert().Equal(fixtures.LegacyTransactionHash[2:], res.TxHash)

	var txSent []string
	for _, wsProvider := range s.nodeWSManager.Providers() {
		txSent = append(txSent, wsProvider.(*eth.MockWSProvider).TxSent...)
		wsProvider.(*eth.MockWSProvider).TxSent = []string{}
	}

	require.Eventually(s.T(), func() bool {
		return len(txSent) == 1
	}, 1*time.Second, 10*time.Millisecond)
	s.Assert().Equal("0x"+fixtures.LegacyTransaction, txSent[0])
}

func (s *wsSuite) TestBlxrTxRequestLegacyTx() {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s"}}`, fixtures.LegacyTransaction)
	msg := s.writeMsgToWsAndReadResponse([]byte(reqPayload), nil)
	clientRes := s.getClientResponse(msg)
	res := s.parseBlxrTxResult(clientRes.Result)
	s.Require().Nil(clientRes.Error)
	s.Assert().Equal(fixtures.LegacyTransactionHash[2:], res.TxHash)
}

func (s *wsSuite) TestBlxrTxsRequestLegacyTx() {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_batch_tx", "params": {"transactions": ["%s"]}}`, fixtures.LegacyTransaction)
	msg := s.writeMsgToWsAndReadResponse([]byte(reqPayload), nil)
	clientRes := s.getClientResponse(msg)

	b, err := json.Marshal(clientRes.Result)
	s.Require().NoError(err)
	var res rpcBatchTxResponse
	err = json.Unmarshal(b, &res)
	s.Require().NoError(err)

	s.Assert().Equal(fixtures.LegacyTransactionHash[2:], res.TxHashes[0])
}

func (s *wsSuite) TestBlxrTxRequestAccessListTx() {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s"}}`, fixtures.AccessListTransactionForRPCInterface)
	msg := s.writeMsgToWsAndReadResponse([]byte(reqPayload), nil)
	clientRes := s.getClientResponse(msg)
	res := s.parseBlxrTxResult(clientRes.Result)
	s.Assert().Equal(fixtures.AccessListTransactionHash[2:], res.TxHash)
}

func (s *wsSuite) TestBlxrTxRequestDynamicFeeTx() {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s"}}`, fixtures.DynamicFeeTransactionForRPCInterface)
	msg := s.writeMsgToWsAndReadResponse([]byte(reqPayload), nil)
	clientRes := s.getClientResponse(msg)
	res := s.parseBlxrTxResult(clientRes.Result)
	s.Assert().Equal(fixtures.DynamicFeeTransactionHash[2:], res.TxHash)
}

func (s *wsSuite) TestBlxrTxRequestTxWithPrefix() {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s"}}`, "0x"+fixtures.DynamicFeeTransactionForRPCInterface)
	msg := s.writeMsgToWsAndReadResponse([]byte(reqPayload), nil)
	clientRes := s.getClientResponse(msg)
	res := s.parseBlxrTxResult(clientRes.Result)
	s.Assert().Equal(fixtures.DynamicFeeTransactionHash[2:], res.TxHash)
}

func (s *wsSuite) TestBlxrTxRequestWithNextValidator() {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s", "next_validator":true}}}`, "0x"+fixtures.LegacyTransaction)
	msg := s.writeMsgToWsAndReadResponse([]byte(reqPayload), nil)
	clientRes := s.getClientResponse(msg)
	s.Require().NotNil(clientRes.Error)
	err, ok := clientRes.Error.(map[string]interface{})
	s.Require().True(ok)
	data, ok := err["data"].(string)
	s.Require().True(ok)
	s.Assert().Contains(data, "next_validator is only supported on BSC network")
}

func (s *wsSuite) TestBlxrBSCTxRequestWithNextValidator() {
	s.TearDownSuite()                  // tear down the suit
	s.setupSuit(bxtypes.BSCMainnetNum) // set up the suit with BSC

	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s", "next_validator":true}}}`, "0x"+fixtures.LegacyTransactionBSC)
	msg := s.writeMsgToWsAndReadResponse([]byte(reqPayload), nil)
	clientRes := s.getClientResponse(msg)
	s.Require().Nil(clientRes.Error)
	res := s.parseBlxrTxResult(clientRes.Result)
	s.Assert().Equal(fixtures.LegacyTransactionBSCHash, res.TxHash)

	s.TearDownSuite() // tear down the suit
	s.SetupSuite()    // reset the suit
}

func (s *wsSuite) TestBlxrTxRequestRLPTx() {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s"}}`, fixtures.RLPTransaction)
	msg := s.writeMsgToWsAndReadResponse([]byte(reqPayload), nil)
	clientRes := s.getClientResponse(msg)
	res := s.parseBlxrTxResult(clientRes.Result)
	s.Require().Nil(clientRes.Error)
	s.Assert().Equal(fixtures.RLPTransactionHash[2:], res.TxHash)
}

func (s *wsSuite) TestBlxrTxWithWrongChainID() {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s"}}`, fixtures.LegacyTransactionBSC)
	msg := s.writeMsgToWsAndReadResponse([]byte(reqPayload), nil)
	clientRes := s.getClientResponse(msg)
	s.Assert().NotNil(clientRes.Error)
	err, ok := clientRes.Error.(map[string]interface{})
	s.Require().True(ok)
	data, ok := err["data"].(string)
	s.Require().True(ok)
	s.Assert().Contains(data, "chainID mismatch")
}

func (s *wsSuite) parseBlxrTxResult(rpcResponse interface{}) (tr rpcTxResponse) {
	b, err := json.Marshal(rpcResponse)
	s.Require().NoError(err)

	var res rpcTxResponse
	err = json.Unmarshal(b, &res)
	s.Require().NoError(err)

	return res
}
