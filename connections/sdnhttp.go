package connections

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bloXroute-Labs/gateway"
	"github.com/bloXroute-Labs/gateway/config"
	"github.com/bloXroute-Labs/gateway/sdnmessage"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/bloXroute-Labs/gateway/utils"
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"net/http"
	"os/exec"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"sync"
	"time"
)

// ErrSDNUnavailable - represents sdn service unavailable
var ErrSDNUnavailable = errors.New("SDN service unavailable")

// SDN Http type constants
const (
	PingTimeout                     = 2000.0
	TimeRegEx                       = "= ([^/]*)"
	blockchainNetworksCacheFileName = "blockchainNetworks.json"
	blockchainNetworkCacheFileName  = "blockchainNetwork.json"
	nodeModelCacheFileName          = "nodemodel.json"
	potentialRelaysFileName         = "potentialrelays.json"
	accountModelsFileName           = "accountmodel.json"
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
	dataDir          string
}

// nodeLatencyInfo contains ping results with host and latency info
type nodeLatencyInfo struct {
	IP      string
	Port    int64
	Latency float64
}

func init() {
	utils.IPResolverHolder = &utils.PublicIPResolver{}
}

// NewSDNHTTP creates a new connection to the bloxroute API
func NewSDNHTTP(sslCerts *utils.SSLCerts, sdnURL string, nodeModel sdnmessage.NodeModel, dataDir string) *SDNHTTP {
	if nodeModel.ExternalIP == "" {
		var err error
		nodeModel.ExternalIP, err = utils.IPResolverHolder.GetPublicIP()
		if err != nil {
			panic(fmt.Errorf("could not determine node's public ip: %v. consider specifying an --external-ip address", err))
		}
		if nodeModel.ExternalIP == "" {
			panic(fmt.Errorf("could not determine node's public ip. consider specifying an --external-ip address"))
		}
		log.Infof("no external ip address was provided, using autodiscovered ip address %v", nodeModel.ExternalIP)
	}
	sdn := &SDNHTTP{
		sslCerts:         sslCerts,
		SdnURL:           sdnURL,
		nodeModel:        nodeModel,
		getPingLatencies: getPingLatencies,
		dataDir:          dataDir,
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

// GetBlockchainNetwork fetches a blockchain network given the blockchain number of the model registered with SDN
func (s *SDNHTTP) GetBlockchainNetwork() error {
	networkNum := s.NetworkNum()
	url := fmt.Sprintf("%v/blockchain-networks/%v", s.SdnURL, networkNum)
	resp, err := s.httpWithCache(url, bxgateway.GetMethod, blockchainNetworkCacheFileName, nil)
	if err != nil {
		return err
	}
	prev, ok := s.Networks[networkNum]
	if !ok {
		s.Networks[networkNum] = new(sdnmessage.BlockchainNetwork)
	}
	if err = json.Unmarshal(resp, s.Networks[networkNum]); err != nil {
		return fmt.Errorf("could not deserialize blockchain network (previously cached as: %v) for networkNum %v, because : %v", prev, networkNum, err)
	}
	if prev != nil && s.Networks[networkNum].MinTxAgeSeconds != prev.MinTxAgeSeconds {
		log.Debugf("The MinTxAgeSeconds changed from %v seconds to %v seconds after the update", prev.MinTxAgeSeconds, s.Networks[networkNum].MinTxAgeSeconds)
	}
	return nil
}

// InitGateway fetches all necessary information over HTTP from the SDN
func (s *SDNHTTP) InitGateway(protocol string, network string) error {
	var err error
	s.nodeModel.Network = network
	s.nodeModel.Protocol = protocol
	s.Networks = make(sdnmessage.BlockchainNetworks)

	if err = s.Register(); err != nil {
		return err
	}
	if err = s.GetBlockchainNetwork(); err != nil {
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

	log.Infof("selected relay %v:%v with latency %v ms", lowestLatencyRelay.IP, lowestLatencyRelay.Port, lowestLatencyRelay.Latency)
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

	resp, err := s.httpWithCache(s.SdnURL+"/nodes", bxgateway.PostMethod, nodeModelCacheFileName, bytes.NewBuffer(s.nodeModel.Pack()))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(resp, &s.nodeModel); err != nil {
		return fmt.Errorf("could not deserialize node model: %v", err)
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

func (s *SDNHTTP) close(resp *http.Response) {
	err := resp.Body.Close()
	if err != nil {
		log.Error(fmt.Errorf("unable to close response body %v error %v", resp.Body, err))
	}
}

func (s *SDNHTTP) getAccountModelWithEndpoint(accountID types.AccountID, endpoint string) (sdnmessage.Account, error) {
	url := fmt.Sprintf("%v/%v/%v", s.SdnURL, endpoint, accountID)
	accountModel := sdnmessage.Account{}
	// for accounts endpoint we do no want to use the cache file.
	// in case of SDN error, we set default enterprise account for the customer
	var resp []byte
	var err error
	switch endpoint {
	case "accounts":
		resp, err = s.http(url, bxgateway.GetMethod, nil)
	case "account":
		resp, err = s.httpWithCache(url, bxgateway.GetMethod, accountModelsFileName, nil)
	default:
		log.Panicf("getAccountModelWithEndpoint called with unsuppored endpoint %v", endpoint)
	}

	if err != nil {
		return accountModel, err
	}

	if err = json.Unmarshal(resp, &accountModel); err != nil {
		return accountModel, fmt.Errorf("could not deserialize account model: %v", err)
	}
	return accountModel, err
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
	url := fmt.Sprintf("%v/nodes/%v/%v/potential-relays", s.SdnURL, nodeID, networkNum)
	resp, err := s.httpWithCache(url, bxgateway.GetMethod, potentialRelaysFileName, nil)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(resp, &s.relays); err != nil {
		return fmt.Errorf("could not deserialize potential relays: %v", err)
	}
	return nil
}

func (s *SDNHTTP) httpWithCache(uri string, method string, fileName string, body io.Reader) ([]byte, error) {
	var err error
	data, httpErr := s.http(uri, method, body)
	if httpErr != nil {
		if httpErr == ErrSDNUnavailable {
			// we can't get the data from http - try to read from cache file
			data, err = utils.LoadCacheFile(s.dataDir, fileName)
			if err != nil {
				return nil, fmt.Errorf("got error from http request: %v and can't load cache file %v: %v", httpErr, fileName, err)
			}
			// we managed to read the data from cache file - issue a warning
			log.Warnf("got error from http request: %v but loaded cache file %v", httpErr, fileName)
			return data, nil
		}
		return nil, httpErr
	}

	err = utils.UpdateCacheFile(s.dataDir, fileName, data)
	if err != nil {
		log.Warnf("can not update cache file %v with data %s. error %v", fileName, data, err)
	}
	return data, nil
}

func (s *SDNHTTP) http(uri string, method string, body io.Reader) ([]byte, error) {
	client, err := s.httpClient()
	if err != nil {
		return nil, err
	}
	var resp *http.Response
	defer func() {
		if resp != nil {
			s.close(resp)
		}
	}()
	switch method {
	case bxgateway.GetMethod:
		resp, err = client.Get(uri)
	case bxgateway.PostMethod:
		resp, err = client.Post(uri, "application/json", body)
	}
	if err != nil {
		return nil, err
	}
	if resp != nil && resp.StatusCode != 200 {
		if resp.StatusCode == bxgateway.ServiceUnavailable {
			log.Debugf("got error from http request: sdn is down")
			return nil, ErrSDNUnavailable
		}
		if resp.Body != nil {
			b, errMsg := ioutil.ReadAll(resp.Body)
			if errMsg != nil {
				return nil, fmt.Errorf("%v on %v could not read response %v, error %v", method, uri, resp.Status, errMsg.Error())
			}
			var errorMessage sdnmessage.ErrorMessage
			if err := json.Unmarshal(b, &errorMessage); err != nil {
				return nil, fmt.Errorf("could not deserialize: %v", err)
			}
			err = fmt.Errorf("%v to %v received a [%v]: %v", method, uri, resp.Status, errorMessage.Details)
		} else {
			err = fmt.Errorf("%v on %v recv and error %v", method, uri, resp.Status)
		}
		return nil, err
	}

	b, errMsg := ioutil.ReadAll(resp.Body)
	if errMsg != nil {
		return nil, fmt.Errorf("%v on %v could not read response %v, error %v", method, uri, resp.Status, errMsg.Error())

	}
	return b, nil
}

func (s *SDNHTTP) getBlockchainNetworks() error {
	url := fmt.Sprintf("%v/blockchain-networks", s.SdnURL)
	resp, err := s.httpWithCache(url, bxgateway.GetMethod, blockchainNetworksCacheFileName, nil)
	if err != nil {
		return err
	}
	var networks []*sdnmessage.BlockchainNetwork
	if err = json.Unmarshal(resp, &networks); err != nil {
		return fmt.Errorf("could not deserialize blockchain networks: %v", err)
	}
	s.Networks = sdnmessage.BlockchainNetworks{}
	for _, network := range networks {
		s.Networks[network.NetworkNum] = network
	}
	return nil
}

// FindNetwork finds a BlockchainNetwork instance by its number and allow update
func (s *SDNHTTP) FindNetwork(networkNum types.NetworkNum) (*sdnmessage.BlockchainNetwork, error) {
	return s.Networks.FindNetwork(networkNum)
}

// GetMinTxAge returns MinTxAge for the current blockchain number the node model registered
func (s *SDNHTTP) GetMinTxAge() time.Duration {
	blockchainNetwork, err := s.FindNetwork(s.NetworkNum())
	if err != nil {
		log.Debugf("could not get blockchainNetwork: %v, returning default 2 seconds for MinTxAgeSecond", err)
		return 2 * time.Second
	}
	return time.Duration(float64(time.Second) * blockchainNetwork.MinTxAgeSeconds)
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
	log.Infof("latency results for potential relays: %v", pingResults)
	return pingResults
}
