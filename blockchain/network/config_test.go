package network

import (
	"fmt"
	"testing"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/stretchr/testify/assert"

	"github.com/bloXroute-Labs/gateway/v2/utils"
)

var (
	testIP     = "123.45.6.78"
	testPort   = 1234
	testWSPort = 2345
)

func TestMultiEthWSURIParse(t *testing.T) {
	input, enodes := generateMultiEthWSURIInput(3)
	config := EthConfig{}
	err := config.parseMultiNode(input, bxtypes.Mainnet)
	assert.NoError(t, err)
	peerInfo := config.StaticPeers
	assert.Equal(t, len(peerInfo), 3)
	for i, info := range peerInfo {
		assert.True(t, enodes[i].Pubkey().Equal(info.Enode.Pubkey()))
		assert.Equal(t, fmt.Sprintf("%v:%v", testIP, testPort+i), fmt.Sprintf("%v:%v", info.Enode.IP(), info.Enode.TCP()))
		assert.Equal(t, fmt.Sprintf("ws://%v:%v", testIP, testWSPort+i), info.EthWSURI)
	}
}

func TestMultiNode(t *testing.T) {
	testCases := []struct {
		Name          string
		Multinode     string
		ErrorContains string
	}{
		{
			Name:      "valid enode",
			Multinode: "enode://313a737a7b3a85963798bbb3ff5cd0fb7cc7e14b53b655700ed4cdc5b83ec8742f7cb16307c4c7b22bf612fe7b696768308f949898f3861eaca7968ae65fcb1a@1.1.1.1:30303",
		},
		{
			Name:      "valid ENR",
			Multinode: "enr:-MK4QCXhv2TKQ7gH5jLM556cG1zHbQz8PjJCwqyO23IpMUIKTK1bVYOc6GEflMu9zBbJgvg_bAbgc_RjB_jyxCgGTiWGAYGmDhATh2F0dG5ldHOIAAAAAAAAAACEZXRoMpA8-jusgAAAcf__________gmlkgnY0gmlwhCzItcmJc2VjcDI1NmsxoQLRFwJXriVehcQyPyjkRZ5ReEL2qqCyviRfkF8vi0ufe4hzeW5jbmV0cwCDdGNwgjLIg3VkcIIu4A+prysm://1.1.1.1:1000",
		},
		{
			Name:      "valid multiaddr",
			Multinode: "multiaddr:/ip4/44.200.181.201/tcp/13000/p2p/16Uiu2HAm9VsYAuES1krVUZFQG8JmokMhxeRzvN1wMhB9jWeUouT8+prysm://1.1.1.1:1000",
		},
		{
			Name:      "valid QUIC multiaddr",
			Multinode: "multiaddr:/ip4/172.18.0.2/udp/13000/quic-v1/p2p/16Uiu2HAm9VsYAuES1krVUZFQG8JmokMhxeRzvN1wMhB9jWeUouT8",
		},
		{
			Name:      "valid DNS multiaddr",
			Multinode: "multiaddr:/dns/localhost/tcp/13000/p2p/16Uiu2HAm9VsYAuES1krVUZFQG8JmokMhxeRzvN1wMhB9jWeUouT8+prysm://1.1.1.1:1000",
		},
		{
			Name:      "valid multi beacon api",
			Multinode: "beacon-api://127.0.0.1:8080,beacon-api://8.8.8.8:8080",
		},
		{
			Name:          "invalid beacon api endpoint",
			Multinode:     "beacon-api://127.0.0.1:8080,beacon-api://127.0.1:8080",
			ErrorContains: "invalid beacon-api argument after 1 comma: --beacon-api-uri: invalid IP address",
		},
		{
			Name:          "duplicated beacon api endpoints",
			Multinode:     "beacon-api://127.0.0.1:8080,beacon-api://127.0.0.1:8080",
			ErrorContains: "duplicated beacon-api argument after 1 comma",
		},
		{
			Name:          "reject unknown scheme",
			Multinode:     "enode://313a737a7b3a85963798bbb3ff5cd0fb7cc7e14b53b655700ed4cdc5b83ec8742f7cb16307c4c7b22bf612fe7b696768308f949898f3861eaca7968ae65fcb1a@1.1.1.1:30303+ws://1.1.1.1:1111,enr:-MK4QCXhv2TKQ7gH5jLM556cG1zHbQz8PjJCwqyO23IpMUIKTK1bVYOc6GEflMu9zBbJgvg_bAbgc_RjB_jyxCgGTiWGAYGmDhATh2F0dG5ldHOIAAAAAAAAAACEZXRoMpA8-jusgAAAcf__________gmlkgnY0gmlwhCzItcmJc2VjcDI1NmsxoQLRFwJXriVehcQyPyjkRZ5ReEL2qqCyviRfkF8vi0ufe4hzeW5jbmV0cwCDdGNwgjLIg3VkcIIu4A+grpc://1.1.1.1:1000",
			ErrorContains: fmt.Sprintf(invalidMultiNodeErrMsg, 1),
		},
		{
			Name:          "reject empty DNS",
			Multinode:     "multiaddr:/dns//tcp/13000/p2p/16Uiu2HAm9VsYAuES1krVUZFQG8JmokMhxeRzvN1wMhB9jWeUouT8+prysm://1.1.1.1:1000",
			ErrorContains: "empty dns addr",
		},
		{
			Name:          "reject invalid DNS",
			Multinode:     "multiaddr:/dns/abc/tcp/13000/p2p/16Uiu2HAm9VsYAuES1krVUZFQG8JmokMhxeRzvN1wMhB9jWeUouT8+prysm://1.1.1.1:1000",
			ErrorContains: "DNS address 'abc' is not valid",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			config := EthConfig{}
			err := config.parseMultiNode(tc.Multinode, bxtypes.Mainnet)
			peerInfo := config.StaticPeers
			if tc.ErrorContains != "" {
				assert.ErrorContains(t, err, tc.ErrorContains)
			} else {
				assert.NotNil(t, peerInfo)
				assert.NoError(t, err)
			}
		})
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
