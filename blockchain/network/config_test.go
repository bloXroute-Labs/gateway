package network

import (
	"fmt"
	"testing"

	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/stretchr/testify/assert"
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

func TestWrongMultiNode(t *testing.T) {
	inputs := map[string]int{
		"enode://313a737a7b3a85963798bbb3ff5cd0fb7cc7e14b53b655700ed4cdc5b83ec8742f7cb16307c4c7b22bf612fe7b696768308f949898f3861eaca7968ae65fcb1a@1.1.1.1:30303+enr:-MK4QCXhv2TKQ7gH5jLM556cG1zHbQz8PjJCwqyO23IpMUIKTK1bVYOc6GEflMu9zBbJgvg_bAbgc_RjB_jyxCgGTiWGAYGmDhATh2F0dG5ldHOIAAAAAAAAAACEZXRoMpA8-jusgAAAcf__________gmlkgnY0gmlwhCzItcmJc2VjcDI1NmsxoQLRFwJXriVehcQyPyjkRZ5ReEL2qqCyviRfkF8vi0ufe4hzeW5jbmV0cwCDdGNwgjLIg3VkcIIu4A+prysm://1.1.1.1:1000":                  0,
		"enode://313a737a7b3a85963798bbb3ff5cd0fb7cc7e14b53b655700ed4cdc5b83ec8742f7cb16307c4c7b22bf612fe7b696768308f949898f3861eaca7968ae65fcb1a@1.1.1.1:30303+ws://1.1.1.1:1111,enr:-MK4QCXhv2TKQ7gH5jLM556cG1zHbQz8PjJCwqyO23IpMUIKTK1bVYOc6GEflMu9zBbJgvg_bAbgc_RjB_jyxCgGTiWGAYGmDhATh2F0dG5ldHOIAAAAAAAAAACEZXRoMpA8-jusgAAAcf__________gmlkgnY0gmlwhCzItcmJc2VjcDI1NmsxoQLRFwJXriVehcQyPyjkRZ5ReEL2qqCyviRfkF8vi0ufe4hzeW5jbmV0cwCDdGNwgjLIg3VkcIIu4A+grpc://1.1.1.1:1000": 1,
	}
	for input, wrongNode := range inputs {
		peerInfo, err := ParseMultiNode(input)
		assert.Nil(t, peerInfo)
		assert.Equal(t, fmt.Sprintf(invalidMultiNodeErrMsg, wrongNode), err.Error())
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
