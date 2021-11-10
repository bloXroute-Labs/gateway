package connections

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/config"
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/sdnmessage"
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/types"
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/utils"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"os/exec"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"sync"
)

// SDN Http type constants
const (
	PingTimeout = 2000.0
	TimeRegEx   = "= ([^/]*)"
)

// SDNHTTP is a connection to the bloxroute API
type SDNHTTP struct {
	sslCerts         *utils.SSLCerts
	SdnURL           string
	NodeID           types.NodeID
	nodeModel        sdnmessage.NodeModel
	relays           sdnmessage.Peers
	Networks         sdnmessage.BlockchainNetworks
	accountModel     sdnmessage.Account
	getPingLatencies func(peers sdnmessage.Peers) []nodeLatencyInfo
}

// nodeLatencyInfo contains ping results with host and latency info
type nodeLatencyInfo struct {
	IP      string
	Port    int64
	Latency float64
}

// NewSDNHTTP creates a new connection to the bloxroute API
func NewSDNHTTP(sslCerts *utils.SSLCerts, sdnURL string, nodeModel sdnmessage.NodeModel) *SDNHTTP {
	if nodeModel.ExternalIP == "" {
		var err error
		nodeModel.ExternalIP, err = utils.GetPublicIP()
		if err != nil {
			panic(fmt.Errorf("could not determine node's public ip: %v. consider specifying an --external-ip address", err))
		}
		log.Infof("no external ip address was provided, using autodiscovered ip address %v", nodeModel.ExternalIP)
	}
	sdn := &SDNHTTP{
		sslCerts:         sslCerts,
		SdnURL:           sdnURL,
		nodeModel:        nodeModel,
		getPingLatencies: getPingLatencies,
	}
	return sdn
}

// GetBlockchainNetworks fetches list of blockchain networks from the sdn
func (s *SDNHTTP) GetBlockchainNetworks() error {
	err := s.getBlockchainNetworks()
	if err != nil {
		return err
	}
	return nil
}

// InitGateway fetches all necessary information over HTTP from the SDN
func (s *SDNHTTP) InitGateway(protocol string, network string) error {
	err := s.GetBlockchainNetworks()
	if err != nil {
		return err
	}
	err = s.reduceToNetwork(protocol, network)
	if err != nil {
		return err
	}
	err = s.Register()
	if err != nil {
		return err
	}
	if err = s.getRelays(s.nodeModel.NodeID, s.nodeModel.BlockchainNetworkNum); err != nil {
		return err
	}
	err = s.getAccountModel(s.nodeModel.AccountID)
	if err != nil {
		return err
	}
	return nil
}

// BestRelay finds the recommended relay from the SDN
func (s SDNHTTP) BestRelay(bxConfig *config.Bx) (ip string, port int64, err error) {
	if bxConfig.OverrideRelay {
		return bxConfig.OverrideRelayHost, 1809, nil
	}

	if len(s.relays) == 0 {
		return "", 0, errors.New("no relays are available")
	}

	relayLatencies := s.getPingLatencies(s.relays)
	if len(relayLatencies) == 0 {
		return "", 0, errors.New("no latencies were acquired for the relays")
	}

	lowestLatencyRelay := relayLatencies[0]
	if lowestLatencyRelay.Latency > 40 {
		log.Warnf("ping latency of the fastest relay %v:%v is %v ms, which is more than 40 ms",
			lowestLatencyRelay.IP, lowestLatencyRelay.Port, lowestLatencyRelay.Latency)
	}

	log.Debugf("selected relay %v:%v with latency %v ms", lowestLatencyRelay.IP, lowestLatencyRelay.Port, lowestLatencyRelay.Latency)
	return lowestLatencyRelay.IP, lowestLatencyRelay.Port, nil
}

// NodeModel returns the node model returned by the SDN
func (s SDNHTTP) NodeModel() *sdnmessage.NodeModel {
	return &s.nodeModel
}

// AccountTier returns the account tier name
func (s SDNHTTP) AccountTier() sdnmessage.AccountTier {
	return s.accountModel.TierName
}

// AccountModel returns the account model
func (s SDNHTTP) AccountModel() sdnmessage.Account {
	return s.accountModel
}

// NetworkNum returns the registered network number of the node model
func (s SDNHTTP) NetworkNum() types.NetworkNum {
	return s.nodeModel.BlockchainNetworkNum
}

func (s SDNHTTP) httpClient() (*http.Client, error) {
	var tlsConfig *tls.Config
	var err error
	if s.sslCerts.NeedsPrivateCert() {
		tlsConfig, err = s.sslCerts.LoadRegistrationConfig()
	} else {
		tlsConfig, err = s.sslCerts.LoadPrivateConfig()
	}
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}
	return client, nil
}

// Register submits a registration request to bxapi. This will return private certificates for the node
// and assign a node ID.
func (s *SDNHTTP) Register() error {
	if s.sslCerts.NeedsPrivateCert() {
		log.Debug("new private certificate needed, appending csr to node registration")
		csr, err := s.sslCerts.CreateCSR()
		if err != nil {
			return err
		}
		s.nodeModel.Csr = string(csr)
	} else {
		nodeID, err := s.sslCerts.GetNodeID()
		if err != nil {
			return err
		}
		s.NodeID = nodeID
	}
	client, err := s.httpClient()
	if err != nil {
		return err
	}
	resp, err := client.Post(s.SdnURL+"/nodes", "application/json", bytes.NewBuffer(s.nodeModel.Pack()))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(resp.Body)
		errMsg, err := strconv.Unquote(string(body))
		if err != nil {
			errMsg = string(body)
		}
		return fmt.Errorf("could not register with bxapi: %v", errMsg)

	}
	defer func() {
		s.close(err, resp)
	}()
	//fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &s.nodeModel); err != nil {
		return err
	}
	s.NodeID = s.nodeModel.NodeID
	if s.sslCerts.NeedsPrivateCert() {
		err := s.sslCerts.SavePrivateCert(s.nodeModel.Cert)
		// should pretty much never happen unless there are SDN problems, in which
		// case just abort on startup
		if err != nil {
			debug.PrintStack()
			panic(err)
		}
	}
	return nil
}

// NeedsRegistration indicates whether proxy must register with the SDN to run
func (s *SDNHTTP) NeedsRegistration() bool {
	return s.NodeID == "" || s.sslCerts.NeedsPrivateCert()
}

func (s *SDNHTTP) close(err error, resp *http.Response) {
	err = resp.Body.Close()
	if err != nil {
		log.Error(fmt.Errorf("unable to close response body %v error %v", resp.Body, err))
	}
}

func (s *SDNHTTP) getAccountModelWithEndpoint(accountID types.AccountID, endpoint string) (sdnmessage.Account, error) {
	client, err := s.httpClient()
	if err != nil {
		return sdnmessage.Account{}, err
	}
	url := fmt.Sprintf("%v/%v/%v", s.SdnURL, endpoint, accountID)
	resp, err := client.Get(url)
	defer func() {
		s.close(err, resp)
	}()
	if err != nil {
		return sdnmessage.Account{}, err
	}
	if resp.StatusCode != 200 {
		return sdnmessage.Account{}, errors.New(resp.Status)
	}

	body, _ := ioutil.ReadAll(resp.Body)
	accountModel := sdnmessage.Account{}
	if err := json.Unmarshal(body, &accountModel); err != nil {
		return sdnmessage.Account{}, fmt.Errorf("could not deserialize account model: %v", err)
	}
	return accountModel, nil
}

func (s *SDNHTTP) getAccountModel(accountID types.AccountID) error {
	accountModel, err := s.getAccountModelWithEndpoint(accountID, "account")
	s.accountModel = accountModel
	return err
}

// GetCustomerAccountModel get customer account model
func (s *SDNHTTP) GetCustomerAccountModel(accountID types.AccountID) (sdnmessage.Account, error) {
	return s.getAccountModelWithEndpoint(accountID, "accounts")
}

func (s *SDNHTTP) getRelays(nodeID types.NodeID, networkNum types.NetworkNum) error {
	client, err := s.httpClient()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%v/nodes/%v/%v/potential-relays", s.SdnURL, nodeID, networkNum)
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return errors.New(resp.Status)
	}
	defer func() {
		s.close(err, resp)
	}()
	body, _ := ioutil.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &s.relays); err != nil {
		return err
	}
	return nil
}

func (s *SDNHTTP) getBlockchainNetworks() error {
	client, err := s.httpClient()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%v/blockchain-networks", s.SdnURL)
	resp, err := client.Get(url)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return errors.New(resp.Status)
	}
	defer func() {
		s.close(err, resp)
	}()
	//fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	var networks []*sdnmessage.BlockchainNetwork
	if err := json.Unmarshal(body, &networks); err != nil {
		return fmt.Errorf("could not deserialize blockchain networks: %v", err)
	}
	s.Networks = sdnmessage.BlockchainNetworks{}
	for _, network := range networks {
		s.Networks[network.NetworkNum] = network
	}
	return nil
}

func (s *SDNHTTP) reduceToNetwork(protocol string, network string) (err error) {
	for networkNum, bcn := range s.Networks {
		if bcn.Network == network && bcn.Protocol == protocol {
			s.Networks = sdnmessage.BlockchainNetworks{networkNum: bcn}
			s.nodeModel.BlockchainNetworkNum = bcn.NetworkNum
			return nil
		}
	}

	return fmt.Errorf("No blockchain network found for protocol %v and network %v", protocol, network)
}

// FindNetwork finds a BlockchainNetwork instance by its number and allow update
func (s *SDNHTTP) FindNetwork(networkNum types.NetworkNum) (*sdnmessage.BlockchainNetwork, error) {
	return s.Networks.FindNetwork(networkNum)
}

// getPingLatencies pings list of SDN peers and returns sorted list of nodeLatencyInfo for each successful peer ping
func getPingLatencies(peers sdnmessage.Peers) []nodeLatencyInfo {
	potentialRelaysCount := len(peers)
	pingResults := make([]nodeLatencyInfo, potentialRelaysCount)
	var wg sync.WaitGroup
	wg.Add(potentialRelaysCount)

	for peerCount, peer := range peers {
		pingResults[peerCount] = nodeLatencyInfo{peer.IP, peer.Port, PingTimeout}
		go func(pingResult *nodeLatencyInfo) {
			defer wg.Done()
			cmd := exec.Command("ping", (*pingResult).IP, "-c1", "-W2")
			var out bytes.Buffer
			var stderr bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				log.Errorf("error executing (%v) %v: %v", cmd, err, stderr)
				return
			}
			log.Tracef("ping results from %v : %v", (*pingResult).IP, out)
			re := regexp.MustCompile(TimeRegEx)
			latencyTimeList := re.FindStringSubmatch(out.String())
			if len(latencyTimeList) > 0 {
				latencyTime, _ := strconv.ParseFloat(latencyTimeList[1], 64)
				if latencyTime > 0 {
					(*pingResult).Latency = latencyTime
				}
			}
		}(&pingResults[peerCount])
	}
	wg.Wait()

	sort.Slice(pingResults, func(i int, j int) bool { return pingResults[i].Latency < pingResults[j].Latency })
	log.Debugf("latency results for potential relays: %v", pingResults)
	return pingResults
}
