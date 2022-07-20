package network

import (
	"fmt"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/stretchr/testify/assert"
	"testing"
)

var testIP = "123.45.6.78"
var testPort = 1234
var testWSPort = 2345

func TestMultiEthWSURIParse(t *testing.T) {
	input, enodes := generateMultiEthWSURIInput(3)
	peerInfo, err := ParseMultiNode(input)
	assert.Nil(t, err)
	assert.Equal(t, len(peerInfo), 3)
	for i, info := range peerInfo {
		assert.True(t, enodes[i].Pubkey().Equal(info.Enode.Pubkey()))
		assert.Equal(t, fmt.Sprintf("%v:%v", testIP, testPort+i), fmt.Sprintf("%v:%v", info.Enode.IP(), info.Enode.TCP()))
		assert.Equal(t, fmt.Sprintf("ws://%v:%v", testIP, testWSPort+i), info.EthWSURI)
	}
}

func generateMultiEthWSURIInput(numPeers int) (string, []*enode.Node) {
	var input string
	enodes := make([]*enode.Node, 0, numPeers)
	for i := 0; i < numPeers; i++ {
		enode := utils.GenerateValidEnode(testIP, testPort+i, testPort+i)
		enodes = append(enodes, enode)
		publicKey := enode.Pubkey()
		enodeWithEndpoint := fmt.Sprintf("enode://%v@%v:%v", hexutil.Encode(crypto.FromECDSAPub(publicKey))[4:], testIP, testPort+i)
		input += enodeWithEndpoint
		input += "+"
		ethWSURI := fmt.Sprintf("ws://%v:%v", testIP, testWSPort+i)
		input += ethWSURI
		if i != numPeers-1 {
			input += ","
		}
	}
	return input, enodes
}
