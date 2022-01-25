package connections

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/bloXroute-Labs/gateway"
	"github.com/bloXroute-Labs/gateway/config"
	"github.com/bloXroute-Labs/gateway/sdnmessage"
	"github.com/bloXroute-Labs/gateway/test"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/bloXroute-Labs/gateway/utils"
	"github.com/gorilla/mux"
	logrusTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestRegister_BlockchainNetworkNumberUpdated(t *testing.T) {
	testTable := []struct {
		protocol      string
		network       string
		networkNumber types.NetworkNum
	}{
		{"Ethereum", "Mainnet", 5},
		{"Ethereum", "Testnet", 23},
	}

	for _, testCase := range testTable {
		t.Run(fmt.Sprint(testCase), func(t *testing.T) {
			server := createNodesServer(t, testCase.protocol, testCase.network, testCase.networkNumber)
			defer func() {
				server.Close()
			}()
			testCerts := utils.TestCerts()
			s := SDNHTTP{
				SdnURL:   server.URL,
				sslCerts: &testCerts,
				nodeModel: sdnmessage.NodeModel{
					Protocol: testCase.protocol,
					Network:  testCase.network,
				},
			}

			err := s.Register()

			assert.Nil(t, err)
			assert.Equal(t, testCase.network, s.nodeModel.Network)
			assert.Equal(t, testCase.protocol, s.nodeModel.Protocol)
			assert.Equal(t, testCase.networkNumber, s.nodeModel.BlockchainNetworkNum)
		})
	}
}

func createNodesServer(t *testing.T, protocol string, network string, blockchainNetworkNum types.NetworkNum) *httptest.Server {
	router := mux.NewRouter()
	handler := func(w http.ResponseWriter, r *http.Request) {
		requestBytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.FailNow()
		}

		var requestNodeModel sdnmessage.NodeModel
		err = json.Unmarshal(requestBytes, &requestNodeModel)
		if err != nil || requestNodeModel.Protocol != protocol || requestNodeModel.Network != network {
			t.FailNow()
		}

		if requestNodeModel.BlockchainNetworkNum == 0 {
			requestNodeModel.BlockchainNetworkNum = blockchainNetworkNum
		}
		responseNodeModel := sdnmessage.NodeModel{
			Protocol:             protocol,
			Network:              network,
			BlockchainNetworkNum: requestNodeModel.BlockchainNetworkNum,
		}

		responseBytes, err := json.Marshal(responseNodeModel)
		if err != nil {
			t.FailNow()
		}

		_, err = w.Write(responseBytes)
		if err != nil {
			t.FailNow()
		}
	}
	pattern := "/nodes"
	router.HandleFunc(pattern, handler).Methods("POST")
	server := httptest.NewServer(router)
	return server
}

func TestBestRelay_IfPingOver40MSLogsWarning(t *testing.T) {
	testTable := []struct {
		name       string
		relayCount int
		latencies  []nodeLatencyInfo
		log        string
	}{
		{"Latency 5", 1, []nodeLatencyInfo{{Latency: 5, IP: "1.1.1.0", Port: 40}}, "selected relay 1.1.1.0:40 with latency 5 ms"},
		{"Latency 20", 1, []nodeLatencyInfo{{Latency: 20, IP: "1.1.1.1", Port: 41}}, "selected relay 1.1.1.1:41 with latency 20 ms"},
		{"Latency 5, 41", 2, []nodeLatencyInfo{{Latency: 5, IP: "1.1.1.2", Port: 42}, {Latency: 41, IP: "1.1.1.3", Port: 43}}, "selected relay 1.1.1.2:42 with latency 5 ms"},
		{"Latency 41", 1, []nodeLatencyInfo{{Latency: 41, IP: "1.1.1.3", Port: 43}},
			"ping latency of the fastest relay 1.1.1.3:43 is 41 ms, which is more than 40 ms"},
		{"Latency 1000, 2000", 2, []nodeLatencyInfo{{Latency: 1000, IP: "1.1.1.4", Port: 44}, {Latency: 2000, IP: "1.1.1.5", Port: 45}},
			"ping latency of the fastest relay 1.1.1.4:44 is 1000 ms, which is more than 40 ms"},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			globalHook := logrusTest.NewGlobal()
			getPingLatenciesFunction := func(peers sdnmessage.Peers) []nodeLatencyInfo {
				return testCase.latencies
			}
			s := SDNHTTP{relays: make([]sdnmessage.Peer, testCase.relayCount), getPingLatencies: getPingLatenciesFunction}
			b := config.Bx{}

			b.OverrideRelay = false
			_, _, err := s.BestRelay(&b)
			assert.Nil(t, err)

			logs := globalHook.Entries
			if testCase.log == "" {
				assert.Nil(t, logs)
			} else {
				if len(logs) == 0 {
					t.Fail()
				}
				firstLog := logs[0]
				assert.Equal(t, testCase.log, firstLog.Message)
			}
		})
	}
}

func TestSDNHTTP_CacheFiles(t *testing.T) {
	defer func() {
		os.Remove(blockchainNetworksCacheFileName)
		os.Remove(nodeModelCacheFileName)
		os.Remove(potentialRelaysFileName)
		os.Remove(accountModelsFileName)
	}()
	// using bad certificate so get/post to bxapi will fail
	sslCerts := utils.SSLCerts{}

	// using bad sdn url so get/post to bxapi will fail
	sdn := NewSDNHTTP(&sslCerts, "https://xxxxxxxxx.com", sdnmessage.NodeModel{}, "")
	url := fmt.Sprintf("%v/blockchain-networks", sdn.SdnURL)

	networks := generateNetworks()
	// generate blockchainNetworks.json file which contains networks using UpdateCacheFile method
	writeToFile(t, networks, blockchainNetworksCacheFileName)

	// calling to httpWithCache -> tying to get blockchain networks from bxapi
	// bxapi is not responsive
	// -> trying to load the blockchain networks from cache file
	resp, err := sdn.httpWithCache(url, bxgateway.GetMethod, blockchainNetworksCacheFileName, nil)
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	cachedNetwork := []*sdnmessage.BlockchainNetwork{}
	assert.Nil(t, json.Unmarshal(resp, &cachedNetwork))
	assert.Equal(t, networks, cachedNetwork)

	nodeModel := generateNodeModel()
	// generate nodemodel.json file which contains nodeModel using UpdateCacheFile method
	writeToFile(t, nodeModel, nodeModelCacheFileName)

	// calling to httpWithCache -> tying to get node model from bxapi
	// bxapi is not responsive
	// -> trying to load the node model from cache file
	resp, err = sdn.httpWithCache(sdn.SdnURL+"/nodes", bxgateway.PostMethod, nodeModelCacheFileName, bytes.NewBuffer(sdn.NodeModel().Pack()))
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	cachedNodeModel := &sdnmessage.NodeModel{}
	assert.Nil(t, json.Unmarshal(resp, &cachedNodeModel))
	assert.Equal(t, nodeModel, cachedNodeModel)

	url = fmt.Sprintf("%v/nodes/%v/%v/potential-relays", sdn.SdnURL, sdn.NodeModel().NodeID, sdn.NodeModel().BlockchainNetworkNum)
	peers := generatePeers()
	// generate potentialrelays.json file which contains peers using UpdateCacheFile method
	writeToFile(t, peers, potentialRelaysFileName)

	// calling to httpWithCache -> tying to get peers from bxapi
	// bxapi is not responsive
	// -> trying to load the peers from cache file
	resp, err = sdn.httpWithCache(url, bxgateway.GetMethod, potentialRelaysFileName, nil)
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	cachedPeers := sdnmessage.Peers{}
	assert.Nil(t, json.Unmarshal(resp, &cachedPeers))
	assert.Equal(t, peers, cachedPeers)

	accountModel := generateAccountModel()
	// generate accountmodel.json file which contains accountModel using UpdateCacheFile method
	writeToFile(t, accountModel, accountModelsFileName)
	url = fmt.Sprintf("%v/%v/%v", sdn.SdnURL, "account", sdn.NodeModel().AccountID)

	// calling to httpWithCache -> tying to get account model from bxapi
	// bxapi is not responsive
	// -> trying to load the account model from cache file
	resp, err = sdn.httpWithCache(url, bxgateway.GetMethod, accountModelsFileName, nil)
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	cachedAccountModel := sdnmessage.Account{}
	assert.Nil(t, json.Unmarshal(resp, &cachedAccountModel))
	assert.Equal(t, accountModel, cachedAccountModel)
}

func generateAccountModel() sdnmessage.Account {
	accountModel := sdnmessage.Account{SecretHash: "1234"}
	return accountModel
}

func generatePeers() sdnmessage.Peers {
	peers := sdnmessage.Peers{}
	peers = append(peers, sdnmessage.Peer{IP: "8.208.101.30", Port: 1809})
	peers = append(peers, sdnmessage.Peer{IP: "47.90.133.153", Port: 1809})
	return peers
}

func generateNodeModel() *sdnmessage.NodeModel {
	nodeModel := &sdnmessage.NodeModel{NodeType: "EXTERNAL_GATEWAY", ExternalPort: 1809, IsDocker: true}
	return nodeModel
}

func generateNetworks() []*sdnmessage.BlockchainNetwork {
	var networks []*sdnmessage.BlockchainNetwork
	network1 := &sdnmessage.BlockchainNetwork{AllowGasPriceChangeReuseSenderNonce: 1.1, AllowedFromTier: "Developer", SendCrossGeo: true, Network: "Mainnet", Protocol: "Ethereum", NetworkNum: 5}
	network2 := &sdnmessage.BlockchainNetwork{AllowGasPriceChangeReuseSenderNonce: 1.1, AllowedFromTier: "Enterprise", SendCrossGeo: true, Network: "BSC-Mainnet", Protocol: "Ethereum", NetworkNum: 10}
	networks = append(networks, network1)
	networks = append(networks, network2)
	return networks
}

func generateTestNetwork() *sdnmessage.BlockchainNetwork {
	return &sdnmessage.BlockchainNetwork{AllowGasPriceChangeReuseSenderNonce: 1.1, AllowedFromTier: "Developer", SendCrossGeo: true, Network: "TestNetwork", Protocol: "TestProtocol", NetworkNum: 0}
}

func TestSDNHTTP_InitGateway(t *testing.T) {
	sslCerts := utils.SSLCerts{}
	sslCerts.SavePrivateCert(test.PrivateCert)
	sdn := NewSDNHTTP(&sslCerts, "https://bdn-api.testnet.blxrbdn.com", sdnmessage.NodeModel{}, "")

	network := generateTestNetwork()
	writeToFile(t, network, blockchainNetworkCacheFileName)
	nodeModel := generateNodeModel()
	writeToFile(t, nodeModel, nodeModelCacheFileName)
	peers := generatePeers()
	writeToFile(t, peers, potentialRelaysFileName)
	accountModel := generateAccountModel()
	writeToFile(t, accountModel, accountModelsFileName)

	assert.Nil(t, sdn.InitGateway(bxgateway.Ethereum, "Mainnet"))

	os.Remove(blockchainNetworkCacheFileName)

	assert.NotNil(t, sdn.InitGateway(bxgateway.Ethereum, "Mainnet"))

	os.Remove(nodeModelCacheFileName)
	os.Remove(potentialRelaysFileName)
	os.Remove(accountModelsFileName)
}

func writeToFile(t *testing.T, data interface{}, fileName string) {
	value, err := json.Marshal(data)
	if err != nil {
		t.FailNow()
	}

	if utils.UpdateCacheFile("", fileName, value) != nil {
		t.FailNow()
	}
}
