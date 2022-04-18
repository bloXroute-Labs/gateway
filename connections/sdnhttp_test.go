package connections

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/bloXroute-Labs/gateway"
	"github.com/bloXroute-Labs/gateway/logger"
	"github.com/bloXroute-Labs/gateway/sdnmessage"
	"github.com/bloXroute-Labs/gateway/test"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/bloXroute-Labs/gateway/utils"
	"github.com/bloXroute-Labs/gateway/utils/utilmock"
	"github.com/gorilla/mux"
	logrusTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"
)

type handlerArgs struct {
	method  string
	pattern string
	handler func(w http.ResponseWriter, r *http.Request)
}

func cleanupFiles() {
	_ = os.Remove(blockchainNetworksCacheFileName)
	_ = os.Remove(blockchainNetworkCacheFileName)
	_ = os.Remove(nodeModelCacheFileName)
	_ = os.Remove(potentialRelaysFileName)
	_ = os.Remove(accountModelsFileName)
}

func testSDNHTTP() realSDNHTTP {
	return realSDNHTTP{
		relays: sdnmessage.Peers{
			{IP: "1.1.1.1", Port: 1},
			{IP: "2.2.2.2", Port: 2},
		},
		getPingLatencies: func(peers sdnmessage.Peers) []nodeLatencyInfo {
			nodeLatencyInfos := []nodeLatencyInfo{}
			for _, peer := range peers {
				nodeLatencyInfos = append(nodeLatencyInfos, nodeLatencyInfo{
					IP:   peer.IP,
					Port: peer.Port,
				})
			}
			return nodeLatencyInfos
		},
	}
}

func TestRegister_BlockchainNetworkNumberUpdated(t *testing.T) {
	testTable := []struct {
		nodeModel         sdnmessage.NodeModel
		networkNumber     types.NetworkNum
		jsonRespNodeModel string
	}{
		{
			nodeModel:         sdnmessage.NodeModel{NodeID: "35299c61-55ad-4565-85a3-0cd985953fac", ExternalIP: "11.113.164.111", Protocol: "Ethereum", Network: "Mainnet"},
			networkNumber:     5,
			jsonRespNodeModel: `{"node_type": "EXTERNAL_GATEWAY", "external_port": 1801, "non_ssl_port": 0, "external_ip": "11.113.164.111", "online": false, "sdn_connection_alive": false, "network": "Mainnet", "protocol": "Ethereum", "node_id": "35299c61-55ad-4565-85a3-0cd985953fac", "sid_start": null, "sid_end": null, "next_sid_start": null, "next_sid_end": null, "sid_expire_time": 259200, "last_pong_time": 0.0, "is_gateway_miner": false, "is_internal_gateway": false, "source_version": "2.108.3.0", "protocol_version": 24, "blockchain_network_num": 10, "blockchain_ip": "52.221.255.145", "blockchain_port": 3000, "hostname": "MacBook-Pro.attlocal.net", "sdn_id": "1e5c6fda-f775-49d4-bd11-287526c07f0f", "os_version": "darwin", "continent": "NA", "split_relays": true, "country": "United States", "region": null, "idx": null, "has_fully_updated_tx_service": false, "node_start_time": "2021-12-30 12:25:26-0500", "node_public_key": "8720705f39ea1ff2eabb38d424136d545005173943062f92cf9cd1f212392c1c0a2ee7ff44ecb84df17140fa7feeee939f0a2b6b3efd3ae5fda72966d4fc0ac1", "baseline_route_redundancy": 0, "baseline_source_redundancy": 0, "private_ip": null, "csr": "", "cert": null, "platform_provider": null, "account_id": "34ff3406-cc74-4cc7-9d9a-9ef8bdda59b1", "latest_source_version": null, "should_update_source_version": false, "assigning_short_ids": false, "node_privileges": "general", "first_seen_time": "1640720639.40804", "is_docker": true, "using_private_ip_connection": false, "private_node": false, "relay_type": ""}`,
		},
		{
			nodeModel:         sdnmessage.NodeModel{NodeID: "35299c61-55ad-4565-85a3-0cd985953fac", ExternalIP: "11.113.164.112", Protocol: "Ethereum", Network: "Testnet"},
			networkNumber:     23,
			jsonRespNodeModel: `{"node_type": "EXTERNAL_GATEWAY", "external_port": 1801, "non_ssl_port": 0, "external_ip": "11.113.164.112", "online": false, "sdn_connection_alive": false, "network": "Testnet", "protocol": "Ethereum", "node_id": "35299c61-55ad-4565-85a3-0cd985953fac", "sid_start": null, "sid_end": null, "next_sid_start": null, "next_sid_end": null, "sid_expire_time": 259200, "last_pong_time": 0.0, "is_gateway_miner": false, "is_internal_gateway": false, "source_version": "2.108.3.0", "protocol_version": 24, "blockchain_network_num": 10, "blockchain_ip": "52.221.255.145", "blockchain_port": 3000, "hostname": "MacBook-Pro.attlocal.net", "sdn_id": "1e5c6fda-f775-49d4-bd11-287526c07f0f", "os_version": "darwin", "continent": "NA", "split_relays": true, "country": "United States", "region": null, "idx": null, "has_fully_updated_tx_service": false, "node_start_time": "2021-12-30 12:25:26-0500", "node_public_key": "8720705f39ea1ff2eabb38d424136d545005173943062f92cf9cd1f212392c1c0a2ee7ff44ecb84df17140fa7feeee939f0a2b6b3efd3ae5fda72966d4fc0ac1", "baseline_route_redundancy": 0, "baseline_source_redundancy": 0, "private_ip": null, "csr": "", "cert": null, "platform_provider": null, "account_id": "34ff3406-cc74-4cc7-9d9a-9ef8bdda59b1", "latest_source_version": null, "should_update_source_version": false, "assigning_short_ids": false, "node_privileges": "general", "first_seen_time": "1640720639.40804", "is_docker": true, "using_private_ip_connection": false, "private_node": false, "relay_type": ""}`,
		},
	}

	for _, testCase := range testTable {
		t.Run(fmt.Sprint(testCase), func(t *testing.T) {
			handler1 := mockNodesServer(t, testCase.nodeModel.NodeID, testCase.nodeModel.ExternalPort, testCase.nodeModel.ExternalIP, testCase.nodeModel.Protocol, testCase.nodeModel.Network, testCase.networkNumber, "")

			handler2, _ := mockNodeModelServer(t, testCase.jsonRespNodeModel)

			var m []handlerArgs
			m = append(m, handlerArgs{method: "POST", pattern: "/nodes", handler: handler1})
			m = append(m, handlerArgs{method: "GET", pattern: "/nodes/{nodeId}", handler: handler2})

			server := mockRouter(m)
			defer func() {
				server.Close()
			}()
			testCerts := utils.TestCerts()
			s := realSDNHTTP{
				sdnURL:   server.URL,
				sslCerts: &testCerts,
				nodeModel: sdnmessage.NodeModel{
					Protocol: testCase.nodeModel.Protocol,
					Network:  testCase.nodeModel.Network,
				},
			}

			err := s.Register()

			assert.Nil(t, err)
			assert.Equal(t, testCase.nodeModel.Network, s.nodeModel.Network)
			assert.Equal(t, testCase.nodeModel.Protocol, s.nodeModel.Protocol)
			assert.Equal(t, testCase.networkNumber, s.nodeModel.BlockchainNetworkNum)
		})
	}
}

func TestDirectRelayConnections_IfPingOver40MSLogsWarning(t *testing.T) {
	logger.NonBlocking.AvoidChannel()
	testTable := []struct {
		name       string
		relayCount int
		latencies  []nodeLatencyInfo
		log        string
	}{
		{"Latency 5", 1, []nodeLatencyInfo{{Latency: 5, IP: "1.1.1.0", Port: 40}}, "fastest selected relay 1.1.1.0:40 has a latency of 5 ms"},
		{"Latency 20", 1, []nodeLatencyInfo{{Latency: 20, IP: "1.1.1.1", Port: 41}}, "fastest selected relay 1.1.1.1:41 has a latency of 20 ms"},
		{"Latency 5, 41", 2, []nodeLatencyInfo{{Latency: 5, IP: "1.1.1.2", Port: 42}, {Latency: 41, IP: "1.1.1.3", Port: 43}}, "fastest selected relay 1.1.1.2:42 has a latency of 5 ms"},
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
			s := realSDNHTTP{relays: make([]sdnmessage.Peer, testCase.relayCount), getPingLatencies: getPingLatenciesFunction}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			autoRelayInstructions := make(chan RelayInstruction)
			go func() { <-autoRelayInstructions }()
			err := s.DirectRelayConnections(ctx, "auto", 1, autoRelayInstructions, AutoRelayTimeout)
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

func TestDirectRelayConnections_IncorrectArgs(t *testing.T) {
	testTable := []struct {
		name          string
		relaysString  string
		expectedError error
	}{
		{
			name:          "no args",
			expectedError: fmt.Errorf("no --relays/relay-ip arguments were provided"),
		},
		{
			name:          "empty string",
			relaysString:  " ",
			expectedError: fmt.Errorf("argument to --relays/relay-ip is empty or has an extra comma"),
		},
		{
			name:          "incorrect host",
			relaysString:  "1:2:3",
			expectedError: fmt.Errorf("relay from --relays/relay-ip was given in the incorrect format '1:2:3', should be IP:Port"),
		},
		{
			name:          "no relay before comma",
			relaysString:  ",127.0.0.1",
			expectedError: fmt.Errorf("argument to --relays/relay-ip is empty or has an extra comma"),
		},
		{
			name:          "no relay after comma",
			relaysString:  "127.0.0.1,",
			expectedError: fmt.Errorf("argument to --relays/relay-ip is empty or has an extra comma"),
		},
		{
			name:          "space after comma",
			relaysString:  "127.0.0.1, ",
			expectedError: fmt.Errorf("argument to --relays/relay-ip is empty or has an extra comma"),
		},
	}

	s := testSDNHTTP()

	for _, testCase := range testTable {
		t.Run(fmt.Sprint(testCase.name), func(t *testing.T) {
			err := s.DirectRelayConnections(context.Background(), testCase.relaysString, 2, make(chan RelayInstruction), AutoRelayTimeout)
			assert.Equal(t, testCase.expectedError, err)
		})
	}
}

func TestDirectRelayConnections_RelayLimit2(t *testing.T) {
	testTable := []struct {
		name           string
		relaysString   string
		expectedRelays map[string]int64
		expectedError  error
	}{
		{
			name:           "one auto",
			relaysString:   "auto",
			expectedRelays: map[string]int64{"1.1.1.1": 1},
		},
		{
			name:           "two autos",
			relaysString:   "auto, auto",
			expectedRelays: map[string]int64{"1.1.1.1": 1, "2.2.2.2": 2},
		},
		{
			name:           "an auto and a relay",
			relaysString:   "auto, 1.1.1.1",
			expectedRelays: map[string]int64{"1.1.1.1": 1809, "2.2.2.2": 2},
		},
		{
			name:           "one relay",
			relaysString:   "1.1.1.1",
			expectedRelays: map[string]int64{"1.1.1.1": 1809},
		},
		{
			name:           "two relays",
			relaysString:   "1.1.1.1, 2.2.2.2",
			expectedRelays: map[string]int64{"1.1.1.1": 1809, "2.2.2.2": 1809},
		},
		{
			name:           "two relays, only one has port",
			relaysString:   "1.1.1.1:34, 2.2.2.2",
			expectedRelays: map[string]int64{"1.1.1.1": 34, "2.2.2.2": 1809},
		},
		{
			name:           "two relays, both have ports",
			relaysString:   "1.1.1.1:34, 2.2.2.2:56",
			expectedRelays: map[string]int64{"1.1.1.1": 34, "2.2.2.2": 56},
		},
		{
			name:           "three relays",
			relaysString:   "4.4.4.4, 2.2.2.2:22, 1.1.1.1",
			expectedRelays: map[string]int64{"4.4.4.4": 1809, "2.2.2.2": 22},
		},
		{
			name:          "incorrect port",
			relaysString:  "1.1.1.1, 2.2.2.2:abc",
			expectedError: fmt.Errorf("port provided abc is not valid - strconv.Atoi: parsing \"abc\": invalid syntax"),
		},
		{
			name:          "incorrect host",
			relaysString:  "1:1:1, 1.1.1.1",
			expectedError: fmt.Errorf("relay from --relays/relay-ip was given in the incorrect format '1:1:1', should be IP:Port"),
		},
		{
			name:           "duplicate relay ips",
			relaysString:   "1.1.1.1, 1.1.1.1:34",
			expectedRelays: map[string]int64{"1.1.1.1": 1809},
		},
		{
			name:           "duplicate relay ips #2",
			relaysString:   "1.1.1.1:1, 1.1.1.1:2, 2.2.2.2:3, 2.2.2.2:4",
			expectedRelays: map[string]int64{"1.1.1.1": 1, "2.2.2.2": 3},
		},
		{
			name:           "duplicate relay ips with auto after",
			relaysString:   "1.1.1.1, 1.1.1.1:2, auto",
			expectedRelays: map[string]int64{"1.1.1.1": 1809, "2.2.2.2": 2},
		},
		{
			name:           "auto relay doesn't overlap with configured relay",
			relaysString:   "auto, 1.1.1.1",
			expectedRelays: map[string]int64{"1.1.1.1": 1809, "2.2.2.2": 2},
		},
		{
			name:           "auto relay doesn't overlap with configured relay #2",
			relaysString:   "2.2.2.2, auto, 1.1.1.1",
			expectedRelays: map[string]int64{"2.2.2.2": 1809, "1.1.1.1": 1},
		},
		{
			name:           "three relays - leading 2 autos",
			relaysString:   "auto, auto, 3.3.3.3",
			expectedRelays: map[string]int64{"1.1.1.1": 1, "2.2.2.2": 2},
		},
	}

	s := testSDNHTTP()

	for _, testCase := range testTable {
		t.Run(fmt.Sprint(testCase.name), func(t *testing.T) {
			relayInstructions := make(chan RelayInstruction)
			expectedRelayCount := len(testCase.expectedRelays)
			var wg sync.WaitGroup

			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < expectedRelayCount; i++ {
					timer := time.NewTimer(time.Millisecond)

					select {
					case <-timer.C:
						t.Fail()
						return
					case instruction := <-relayInstructions:
						port, ok := testCase.expectedRelays[instruction.IP]
						if ok && port == instruction.Port {
							delete(testCase.expectedRelays, instruction.IP) // confirming there is an instruction for each expectedRelay
						}
					}
				}

				timer := time.NewTimer(time.Millisecond)
				select {
				case <-timer.C:
					break
				case <-relayInstructions:
					t.Fail()
				}
			}()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go func() {
				err := s.DirectRelayConnections(ctx, testCase.relaysString, 2, relayInstructions, AutoRelayTimeout)
				assert.Equal(t, testCase.expectedError, err)
			}()

			wg.Wait()
			assert.Len(t, testCase.expectedRelays, 0)
		})
	}
}

func TestDirectRelayConnections_RelayLimit1(t *testing.T) {
	testTable := []struct {
		name           string
		relaysString   string
		expectedRelays map[string]int64
		expectedError  error
	}{
		{
			name:           "one auto",
			relaysString:   "auto",
			expectedRelays: map[string]int64{"1.1.1.1": 1},
		},
		{
			name:           "two autos",
			relaysString:   "auto, auto",
			expectedRelays: map[string]int64{"1.1.1.1": 1},
		},
		{
			name:           "an auto and a relay",
			relaysString:   "auto, 1.1.1.1",
			expectedRelays: map[string]int64{"1.1.1.1": 1},
		},
		{
			name:           "one relay",
			relaysString:   "2.2.2.2",
			expectedRelays: map[string]int64{"2.2.2.2": 1809},
		},
		{
			name:           "two relays",
			relaysString:   "3.3.3.3, 4.4.4.4",
			expectedRelays: map[string]int64{"3.3.3.3": 1809},
		},
		{
			name:           "two relays - duplicates",
			relaysString:   "3.3.3.3:14, 3.3.3.3:15",
			expectedRelays: map[string]int64{"3.3.3.3": 14},
		},
		{
			name:           "one relay with port",
			relaysString:   "1.1.1.1:34",
			expectedRelays: map[string]int64{"1.1.1.1": 34},
		},
		{
			name:          "incorrect port",
			relaysString:  "1.1.1.1:abc",
			expectedError: fmt.Errorf("port provided abc is not valid - strconv.Atoi: parsing \"abc\": invalid syntax"),
		},
		{
			name:          "incorrect host",
			relaysString:  "127.0.0.9999",
			expectedError: fmt.Errorf("host provided 127.0.0.9999 is not valid - lookup 127.0.0.9999: no such host"),
		},
		{
			name:          "incorrect host with port",
			relaysString:  "127.0.0.9999:1234",
			expectedError: fmt.Errorf("host provided 127.0.0.9999 is not valid - lookup 127.0.0.9999: no such host"),
		},
	}

	s := testSDNHTTP()
	ticker := utils.RealClock{}.Ticker(time.Millisecond)

	for _, testCase := range testTable {
		t.Run(fmt.Sprint(testCase.name), func(t *testing.T) {
			relayInstructions := make(chan RelayInstruction)
			expectedRelayCount := len(testCase.expectedRelays)
			var wg sync.WaitGroup

			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < expectedRelayCount; i++ {
					select {
					case <-ticker.Alert():
						t.Fail()
						return
					case instruction := <-relayInstructions:
						port, ok := testCase.expectedRelays[instruction.IP]
						if ok && port == instruction.Port {
							delete(testCase.expectedRelays, instruction.IP) // confirming there is an instruction for each expectedRelay
						}
					}
				}

				select {
				case <-ticker.Alert():
					break
				case <-relayInstructions:
					t.Fail()
				}
			}()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := s.DirectRelayConnections(ctx, testCase.relaysString, 1, relayInstructions, AutoRelayTimeout)
			assert.Equal(t, testCase.expectedError, err)

			wg.Wait()
			assert.Len(t, testCase.expectedRelays, 0)
		})
	}
}

func TestSDNHTTP_CacheFiles_ServiceUnavailable_SDN_BlockchainNetworks(t *testing.T) {
	testCase := struct {
		nodeModel                  sdnmessage.NodeModel
		networkNumber              types.NetworkNum
		jsonRespServiceUnavailable string
	}{
		nodeModel:                  sdnmessage.NodeModel{ExternalIP: "172.0.0.1"},
		networkNumber:              5,
		jsonRespServiceUnavailable: `{"message": "503 Service Unavailable" }`,
	}
	t.Run(fmt.Sprint(testCase), func(t *testing.T) {
		defer cleanupFiles()
		// using bad certificate so get/post to bxapi will fail
		sslCerts := utils.SSLCerts{}

		handler1 := mockServiceError(t, 503, testCase.jsonRespServiceUnavailable)
		var m []handlerArgs
		m = append(m, handlerArgs{method: "GET", pattern: "/blockchain-networks/{networkNum}", handler: handler1})

		server := mockRouter(m)
		defer func() {
			server.Close()
		}()

		utils.IPResolverHolder = &utilmock.MockIPResolver{IP: "11.111.111.111"}
		// using bad sdn url so get/post to bxapi will fail
		sdn := NewSDNHTTP(&sslCerts, server.URL, testCase.nodeModel, "").(*realSDNHTTP)
		url := fmt.Sprintf("%v/blockchain-networks/%v", sdn.SDNURL(), testCase.networkNumber)

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
	})
}

func TestSDNHTTP_CacheFiles_ServiceUnavailable_SDN_Node(t *testing.T) {
	testCase := struct {
		nodeModel                  sdnmessage.NodeModel
		jsonRespServiceUnavailable string
	}{
		nodeModel:                  sdnmessage.NodeModel{ExternalIP: "172.0.0.1"},
		jsonRespServiceUnavailable: `{"message": "503 Service Unavailable" }`,
	}
	t.Run(fmt.Sprint(testCase), func(t *testing.T) {
		defer cleanupFiles()
		// using bad certificate so get/post to bxapi will fail
		sslCerts := utils.SSLCerts{}

		handler1 := mockServiceError(t, 503, testCase.jsonRespServiceUnavailable)
		var m []handlerArgs
		m = append(m, handlerArgs{method: "POST", pattern: "/nodes", handler: handler1})

		server := mockRouter(m)
		defer func() {
			server.Close()
		}()

		utils.IPResolverHolder = &utilmock.MockIPResolver{IP: "11.111.111.111"}
		// using bad sdn url so get/post to bxapi will fail
		sdn := NewSDNHTTP(&sslCerts, server.URL, testCase.nodeModel, "").(*realSDNHTTP)

		nodeModel := generateNodeModel()
		// generate nodemodel.json file which contains nodeModel using UpdateCacheFile method
		writeToFile(t, nodeModel, nodeModelCacheFileName)

		// calling to httpWithCache -> tying to get node model from bxapi
		// bxapi is not responsive
		// -> trying to load the node model from cache file
		resp, err := sdn.httpWithCache(sdn.sdnURL+"/nodes", bxgateway.PostMethod, nodeModelCacheFileName, bytes.NewBuffer(sdn.NodeModel().Pack()))
		assert.Nil(t, err)
		assert.NotNil(t, resp)
		cachedNodeModel := &sdnmessage.NodeModel{}
		assert.Nil(t, json.Unmarshal(resp, &cachedNodeModel))
		assert.Equal(t, nodeModel, cachedNodeModel)
	})
}

func TestSDNHTTP_CacheFiles_ServiceUnavailable_SDN_Relays(t *testing.T) {
	testCase := struct {
		nodeModel                  sdnmessage.NodeModel
		jsonRespServiceUnavailable string
	}{
		nodeModel:                  sdnmessage.NodeModel{NodeID: "35299c61-55ad-4565-85a3-0cd985953fac", BlockchainNetworkNum: 5},
		jsonRespServiceUnavailable: `{"message": "503 Service Unavailable" }`,
	}
	t.Run(fmt.Sprint(testCase), func(t *testing.T) {
		defer cleanupFiles()
		// using bad certificate so get/post to bxapi will fail
		sslCerts := utils.SSLCerts{}

		handler1 := mockServiceError(t, 503, testCase.jsonRespServiceUnavailable)
		var m []handlerArgs
		m = append(m, handlerArgs{method: "GET", pattern: "/nodes/{nodeID}/{networkNum}/potential-relays", handler: handler1})

		server := mockRouter(m)
		defer func() {
			server.Close()
		}()

		utils.IPResolverHolder = &utilmock.MockIPResolver{IP: "11.111.111.111"}
		// using bad sdn url so get/post to bxapi will fail
		sdn := NewSDNHTTP(&sslCerts, server.URL, testCase.nodeModel, "").(*realSDNHTTP)
		url := fmt.Sprintf("%v/nodes/%v/%v/potential-relays", sdn.SDNURL(), sdn.NodeModel().NodeID, sdn.NodeModel().BlockchainNetworkNum)
		peers := generatePeers()
		// generate potentialrelays.json file which contains peers using UpdateCacheFile method
		writeToFile(t, peers, potentialRelaysFileName)

		// calling to httpWithCache -> tying to get peers from bxapi
		// bxapi is not responsive
		// -> trying to load the peers from cache file
		resp, err := sdn.httpWithCache(url, bxgateway.GetMethod, potentialRelaysFileName, nil)
		assert.Nil(t, err)
		assert.NotNil(t, resp)
		cachedPeers := sdnmessage.Peers{}
		assert.Nil(t, json.Unmarshal(resp, &cachedPeers))
		assert.Equal(t, peers, cachedPeers)
	})
}

func TestSDNHTTP_CacheFiles_ServiceUnavailable_SDN_Account(t *testing.T) {
	testCase := struct {
		nodeModel                  sdnmessage.NodeModel
		jsonRespServiceUnavailable string
	}{
		nodeModel:                  sdnmessage.NodeModel{AccountID: "e64yrte6547"},
		jsonRespServiceUnavailable: `{"message": "503 Service Unavailable" }`,
	}
	t.Run(fmt.Sprint(testCase), func(t *testing.T) {
		defer cleanupFiles()
		// using bad certificate so get/post to bxapi will fail
		sslCerts := utils.SSLCerts{}

		handler1 := mockServiceError(t, 503, testCase.jsonRespServiceUnavailable)
		var m []handlerArgs
		m = append(m, handlerArgs{method: "GET", pattern: "/account/{accountID}", handler: handler1})

		server := mockRouter(m)
		defer func() {
			server.Close()
		}()

		utils.IPResolverHolder = &utilmock.MockIPResolver{IP: "11.111.111.111"}
		// using bad sdn url so get/post to bxapi will fail
		sdn := NewSDNHTTP(&sslCerts, server.URL, testCase.nodeModel, "").(*realSDNHTTP)

		accountModel := generateAccountModel()
		// generate accountmodel.json file which contains accountModel using UpdateCacheFile method
		writeToFile(t, accountModel, accountModelsFileName)
		url := fmt.Sprintf("%v/%v/%v", sdn.SDNURL(), "account", sdn.NodeModel().AccountID)

		// calling to httpWithCache -> tying to get account model from bxapi
		// bxapi is not responsive
		// -> trying to load the account model from cache file
		resp, err := sdn.httpWithCache(url, bxgateway.GetMethod, accountModelsFileName, nil)
		assert.Nil(t, err)
		assert.NotNil(t, resp)

		cachedAccountModel := sdnmessage.Account{}
		assert.Nil(t, json.Unmarshal(resp, &cachedAccountModel))
		assert.Equal(t, accountModel, cachedAccountModel)
	})
}

func TestSDNHTTP_InitGateway(t *testing.T) {
	testCase := struct {
		nodeModel          sdnmessage.NodeModel
		networkNumber      types.NetworkNum
		jsonRespNetwork    string
		jsonRespRelays     string
		jsonAccount        string
		expectedRelayLimit int
	}{
		nodeModel:          sdnmessage.NodeModel{NodeID: "35299c61-55ad-4565-85a3-0cd985953fac", ExternalIP: "11.113.164.111", Protocol: "Ethereum", Network: "Mainnet", AccountID: "e64yrte6547"},
		networkNumber:      5,
		jsonRespNetwork:    `{"min_tx_age_seconds":0,"min_tx_network_fee":0, "network":"Mainnet", "network_num":5,"protocol":"Ethereum"}`,
		jsonRespRelays:     `[{"ip":"8.208.101.30", "port":1809}, {"ip":"47.90.133.153", "port":1809}]`,
		jsonAccount:        `{"account_id":"e64yrte6547","blockchain_protocol":"","blockchain_network":"","tier_name":"", "relay_limit":{"expire_date":"", "msg_quota": {"limit":0}}}`,
		expectedRelayLimit: 1,
	}
	t.Run(fmt.Sprint(testCase), func(t *testing.T) {
		defer cleanupFiles()

		sslCerts := utils.NewSSLCertsPrivateKey(test.PrivateKey)
		sslCerts.SavePrivateCert(test.PrivateCert)

		handler1 := mockNodesServer(t, testCase.nodeModel.NodeID, testCase.nodeModel.ExternalPort, testCase.nodeModel.ExternalIP, testCase.nodeModel.Protocol, testCase.nodeModel.Network, testCase.networkNumber, testCase.nodeModel.AccountID)
		handler2, _ := mockBlockchainNetworkServer(t, testCase.jsonRespNetwork)
		handler3, _ := mockRelaysServer(t, testCase.jsonRespRelays)
		handler4, _ := mockAccountServer(t, testCase.jsonAccount)

		var m []handlerArgs
		m = append(m, handlerArgs{method: "POST", pattern: "/nodes", handler: handler1})
		m = append(m, handlerArgs{method: "GET", pattern: "/blockchain-networks/{networkNum}", handler: handler2})
		m = append(m, handlerArgs{method: "GET", pattern: "/nodes/{nodeID}/{networkNum}/potential-relays", handler: handler3})
		m = append(m, handlerArgs{method: "GET", pattern: "/account/{accountID}", handler: handler4})

		server := mockRouter(m)
		defer func() {
			server.Close()
		}()

		defer func() {
			server.Close()
		}()

		utils.IPResolverHolder = &utilmock.MockIPResolver{IP: "11.111.111.111"}
		sdn := NewSDNHTTP(sslCerts, server.URL, sdnmessage.NodeModel{}, "").(*realSDNHTTP)

		assert.Nil(t, sdn.InitGateway(bxgateway.Ethereum, "Mainnet"))
		assert.Equal(t, testCase.expectedRelayLimit, sdn.accountModel.RelayLimit.MsgQuota.Limit)
	})
}

func TestSDNHTTP_InitGateway_Fail(t *testing.T) {
	testCase := struct {
		nodeModel                  sdnmessage.NodeModel
		networkNumber              types.NetworkNum
		jsonRespServiceUnavailable string
	}{
		nodeModel:                  sdnmessage.NodeModel{NodeID: "35299c61-55ad-4565-85a3-0cd985953fac", ExternalIP: "11.113.164.111", Protocol: "Ethereum", Network: "Mainnet", AccountID: "e64yrte6547"},
		networkNumber:              5,
		jsonRespServiceUnavailable: `{"message": "503 Service Unavailable" }`,
	}
	t.Run(fmt.Sprint(testCase), func(t *testing.T) {

		sslCerts := utils.NewSSLCertsPrivateKey(test.PrivateKey)
		sslCerts.SavePrivateCert(test.PrivateCert)

		handler1 := mockServiceError(t, 503, testCase.jsonRespServiceUnavailable)
		var m []handlerArgs
		m = append(m, handlerArgs{method: "POST", pattern: "/nodes", handler: handler1})

		server := mockRouter(m)
		defer func() {
			server.Close()
		}()

		utils.IPResolverHolder = &utilmock.MockIPResolver{IP: "11.111.111.111"}
		sdn := NewSDNHTTP(sslCerts, server.URL, sdnmessage.NodeModel{}, "").(*realSDNHTTP)

		os.Remove(nodeModelCacheFileName)
		assert.NotNil(t, sdn.InitGateway(bxgateway.Ethereum, "Mainnet"))
	})
}

func TestSDNHTTP_HttpPostBadRequestDetailsResponse(t *testing.T) {
	sslCerts := utils.SSLCerts{}

	utils.IPResolverHolder = &utilmock.MockIPResolver{IP: "11.111.111.111"}
	sdn := NewSDNHTTP(&sslCerts, "", sdnmessage.NodeModel{ExternalIP: "localhost"}, "").(*realSDNHTTP)
	testCase := struct {
		nodeModel         sdnmessage.NodeModel
		jsonRespNodeModel string
	}{

		nodeModel:         sdnmessage.NodeModel{NodeType: "FOO"},
		jsonRespNodeModel: `{"message": "Bad Request", "details": "Foo not a valid type"}`,
	}

	t.Run(fmt.Sprint(testCase), func(t *testing.T) {
		router := mux.NewRouter()
		handler := func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(400)
			_, err := w.Write([]byte(testCase.jsonRespNodeModel))
			if err != nil {
				t.FailNow()
			}

		}
		pattern := "/nodes"
		router.HandleFunc(pattern, handler).Methods("POST")
		server := httptest.NewServer(router)
		defer func() {
			server.Close()
		}()

		url := fmt.Sprintf("%v/nodes", server.URL)
		sdn.nodeModel.NodeType = testCase.nodeModel.NodeType
		resp, err := sdn.http(url, bxgateway.PostMethod, bytes.NewBuffer(sdn.NodeModel().Pack()))
		assert.NotNil(t, err)
		assert.Nil(t, resp)
	})
}

func TestSDNHTTP_HttpGetBadRequestDetailsResponse(t *testing.T) {
	sslCerts := utils.SSLCerts{}
	utils.IPResolverHolder = &utilmock.MockIPResolver{IP: "11.111.111.111"}
	sdn := NewSDNHTTP(&sslCerts, "", sdnmessage.NodeModel{ExternalIP: "localhost"}, "").(*realSDNHTTP)
	testCase := struct {
		nodeModel         sdnmessage.NodeModel
		jsonRespNodeModel string
	}{

		nodeModel:         sdnmessage.NodeModel{NodeType: "FOO", NodeID: "0f54c509-06f0-4bdd-8fc0-3bdf1ac119ed"},
		jsonRespNodeModel: `{"message": "Bad Request", "details": "Foo not a valid type"}`,
	}

	t.Run(fmt.Sprint(testCase), func(t *testing.T) {
		router := mux.NewRouter()
		handler := func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(400)
			_, err := w.Write([]byte(testCase.jsonRespNodeModel))
			if err != nil {
				t.FailNow()
			}

		}
		pattern := "/nodes/{nodeId}"
		router.HandleFunc(pattern, handler).Methods("GET")
		server := httptest.NewServer(router)
		defer func() {
			server.Close()
		}()

		url := fmt.Sprintf("%v/nodes/%v", server.URL, testCase.nodeModel.NodeID)
		sdn.nodeModel.NodeType = testCase.nodeModel.NodeType
		resp, err := sdn.http(url, bxgateway.GetMethod, bytes.NewBuffer(sdn.NodeModel().Pack()))
		assert.NotNil(t, err)
		assert.Nil(t, resp)
	})
}

func TestSDNHTTP_HttpPostBodyError(t *testing.T) {
	testCase := struct {
		nodeModel         sdnmessage.NodeModel
		networkNumber     types.NetworkNum
		jsonRespNodeModel string
	}{

		nodeModel:         sdnmessage.NodeModel{NodeType: "TEST"},
		jsonRespNodeModel: `{"message": "Bad Request", "details": "TEST not a valid type"}`,
	}

	t.Run(fmt.Sprint(testCase), func(t *testing.T) {

		router := mux.NewRouter()
		handler := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "1")
			w.WriteHeader(502)
		}
		pattern := "/nodes"
		router.HandleFunc(pattern, handler).Methods("POST")
		server := httptest.NewServer(router)
		defer func() {
			server.Close()
		}()

		testCerts := utils.TestCerts()
		sdn := realSDNHTTP{
			sdnURL:   server.URL,
			sslCerts: &testCerts,
			nodeModel: sdnmessage.NodeModel{
				NodeType: testCase.nodeModel.NodeType,
			},
		}

		url := fmt.Sprintf("%v/nodes", sdn.SDNURL())
		resp, err := sdn.http(url, bxgateway.PostMethod, bytes.NewBuffer(sdn.NodeModel().Pack()))
		assert.NotNil(t, err)
		assert.Nil(t, resp)
	})
}

func TestSDNHTTP_HttpPostUnmarshallError(t *testing.T) {
	testCase := struct {
		nodeModel         sdnmessage.NodeModel
		networkNumber     types.NetworkNum
		jsonRespNodeModel string
	}{

		nodeModel:         sdnmessage.NodeModel{NodeType: "TEST"},
		jsonRespNodeModel: `{"message": 3}`,
	}

	t.Run(fmt.Sprint(testCase), func(t *testing.T) {

		router := mux.NewRouter()
		handler := func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(400)
			_, err := w.Write([]byte(testCase.jsonRespNodeModel))
			if err != nil {
				t.FailNow()
			}
		}
		pattern := "/nodes"
		router.HandleFunc(pattern, handler).Methods("POST")
		server := httptest.NewServer(router)
		defer func() {
			server.Close()
		}()

		testCerts := utils.TestCerts()
		sdn := realSDNHTTP{
			sdnURL:   server.URL,
			sslCerts: &testCerts,
			nodeModel: sdnmessage.NodeModel{
				NodeType: testCase.nodeModel.NodeType,
			},
		}

		url := fmt.Sprintf("%v/nodes", sdn.SDNURL())
		resp, err := sdn.http(url, bxgateway.PostMethod, bytes.NewBuffer(sdn.NodeModel().Pack()))
		assert.NotNil(t, err)
		assert.Nil(t, resp)
	})
}

func TestSDNHTTP_FillInAccountDefaults(t *testing.T) {
	targetAccount := sdnmessage.DefaultEnterpriseAccount
	tp := reflect.TypeOf(targetAccount)
	numFields := tp.NumField()
	for i := 0; i < numFields; i++ {
		reflect.ValueOf(&targetAccount).Elem().FieldByName(tp.Field(i).Name).Set(reflect.Zero(tp.Field(i).Type))
	}

	sdnhttp := testSDNHTTP()

	targetAccount, error := sdnhttp.fillInAccountDefaults(&targetAccount)

	assert.Nil(t, error)
	assert.Equal(t, sdnmessage.DefaultEnterpriseAccount, targetAccount)

}

func mockNodesServer(t *testing.T, nodeID types.NodeID, externalPort int64, externalIP, protocol, network string, blockchainNetworkNum types.NetworkNum, accountID types.AccountID) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
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
			NodeID:               nodeID,
			ExternalIP:           externalIP,
			ExternalPort:         externalPort,
			Protocol:             protocol,
			Network:              network,
			BlockchainNetworkNum: requestNodeModel.BlockchainNetworkNum,
			AccountID:            accountID,
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
}

func mockServiceError(t *testing.T, statusCode int, unavailableJSON string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		_, err := w.Write([]byte(unavailableJSON))
		if err != nil {
			t.FailNow()
		}
	}
}

func mockNodeModelServer(t *testing.T, nodeModel string) (func(w http.ResponseWriter, r *http.Request), sdnmessage.NodeModel) {

	var requestNodeModel sdnmessage.NodeModel
	err := json.Unmarshal([]byte(nodeModel), &requestNodeModel)
	if err != nil {
		fmt.Println(err.Error())
	}
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(nodeModel))
		if err != nil {
			t.FailNow()
		}
	}, requestNodeModel
}

func mockBlockchainNetworkServer(t *testing.T, nodeModel string) (func(w http.ResponseWriter, r *http.Request), sdnmessage.BlockchainNetwork) {
	var network sdnmessage.BlockchainNetwork
	err := json.Unmarshal([]byte(nodeModel), &network)
	if err != nil {
		fmt.Println(err.Error())
	}
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(nodeModel))
		if err != nil {
			t.FailNow()
		}
	}, network
}

func mockRelaysServer(t *testing.T, nodeModel string) (func(w http.ResponseWriter, r *http.Request), sdnmessage.Peers) {
	var relays sdnmessage.Peers
	err := json.Unmarshal([]byte(nodeModel), &relays)
	if err != nil {
		fmt.Println(err.Error())
	}

	return func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(nodeModel))
		if err != nil {
			t.FailNow()
		}
	}, relays
}

func mockAccountServer(t *testing.T, nodeModel string) (func(w http.ResponseWriter, r *http.Request), sdnmessage.Account) {
	var account sdnmessage.Account
	err := json.Unmarshal([]byte(nodeModel), &account)
	if err != nil {
		fmt.Println(err.Error())
	}

	return func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(nodeModel))
		if err != nil {
			t.FailNow()
		}
	}, account
}

func mockRouter(handlerArgs []handlerArgs) *httptest.Server {
	router := mux.NewRouter()
	for _, args := range handlerArgs {
		router.HandleFunc(args.pattern, args.handler).Methods(args.method)
	}
	server := httptest.NewServer(router)
	return server
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

func writeToFile(t *testing.T, data interface{}, fileName string) {
	value, err := json.Marshal(data)
	if err != nil {
		t.FailNow()
	}

	if utils.UpdateCacheFile("", fileName, value) != nil {
		t.FailNow()
	}
}
