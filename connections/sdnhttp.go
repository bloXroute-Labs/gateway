package connections

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bloXroute-Labs/gateway/v2"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/jinzhu/copier"
	"io"
	"io/ioutil"
	"math"
	"math/big"
	"net/http"
	"os/exec"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:generate mockgen -destination ../../bxgateway/test/sdnhttpmock/mock_sdnhttp.go -package mock_connections . SDNHTTP

// ErrSDNUnavailable - represents sdn service unavailable
var ErrSDNUnavailable = errors.New("SDN service unavailable")

// SDN Http type constants
const (
	PingTimeout                     = 2000.0
	TimeRegEx                       = "= ([^/]*)"
	AutoRelayTimeout                = time.Hour
	blockchainNetworksCacheFileName = "blockchainNetworks.json"
	blockchainNetworkCacheFileName  = "blockchainNetwork.json"
	nodeModelCacheFileName          = "nodemodel.json"
	potentialRelaysFileName         = "potentialrelays.json"
	accountModelsFileName           = "accountmodel.json"
)

// SDNHTTP is the interface for realSDNHTTP type
type SDNHTTP interface {
	SDNURL() string
	NodeID() types.NodeID
	Networks() *sdnmessage.BlockchainNetworks
	SetNetworks(networks sdnmessage.BlockchainNetworks)
	FetchAllBlockchainNetworks() error
	FetchBlockchainNetwork() error
	InitGateway(protocol string, network string) error
	NodeModel() *sdnmessage.NodeModel
	AccountTier() sdnmessage.AccountTier
	AccountModel() sdnmessage.Account
	NetworkNum() types.NetworkNum
	Register() error
	NeedsRegistration() bool
	FetchCustomerAccountModel(accountID types.AccountID) (sdnmessage.Account, error)
	DirectRelayConnections(ctx context.Context, relayHosts string, relayLimit uint64, relayInstructions chan<- RelayInstruction, autoRelayTimeout time.Duration) error
	FindNetwork(networkNum types.NetworkNum) (*sdnmessage.BlockchainNetwork, error)
	MinTxAge() time.Duration
	SendNodeEvent(event sdnmessage.NodeEvent, id types.NodeID)
	Get(endpoint string, requestBody []byte) ([]byte, error)
	GetQuotaUsage(accountID string) (*QuotaResponseBody, error)
}

// realSDNHTTP is a connection to the bloxroute API
type realSDNHTTP struct {
	sslCerts         *utils.SSLCerts
	getPingLatencies func(peers sdnmessage.Peers) []nodeLatencyInfo
	networks         sdnmessage.BlockchainNetworks
	accountModel     *sdnmessage.Account
	nodeID           types.NodeID
	accountID        types.AccountID
	sdnURL           string
	dataDir          string
	nodeModel        *sdnmessage.NodeModel
	relays           sdnmessage.Peers
}

// relayMap maps a relay's IP to its port
type relayMap map[string]int64

// nodeLatencyInfo contains ping results with host and latency info
type nodeLatencyInfo struct {
	IP      string
	Port    int64
	Latency float64
}

// RelayInstruction specifies whether to connect or disconnect to the relay at an IP:Port
type RelayInstruction struct {
	IP   string
	Type ConnInstructionType
	Port int64
}

// ConnInstructionType specifies connection or disconnection
type ConnInstructionType int

type quotaRequestBody struct {
	AccountID string `json:"account_id"`
}

// QuotaResponseBody quota usage response body
type QuotaResponseBody struct {
	AccountID   string `json:"account_id"`
	QuotaFilled int    `json:"quota_filled"`
	QuotaLimit  int    `json:"quota_limit"`
}

const (
	// Connect is the instruction to connect to a relay
	Connect ConnInstructionType = iota
	// Disconnect is the instruction to disconnect from a relay
	Disconnect
)

func init() {
	utils.IPResolverHolder = &utils.PublicIPResolver{}
}

// NewSDNHTTP creates a new connection to the bloxroute API
func NewSDNHTTP(sslCerts *utils.SSLCerts, sdnURL string, nodeModel sdnmessage.NodeModel, dataDir string) SDNHTTP {
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
	sdn := &realSDNHTTP{
		sslCerts:         sslCerts,
		sdnURL:           sdnURL,
		nodeModel:        &nodeModel,
		getPingLatencies: getPingLatencies,
		dataDir:          dataDir,
	}
	return sdn
}

// FetchAllBlockchainNetworks fetches list of blockchain networks from the sdn
func (s *realSDNHTTP) FetchAllBlockchainNetworks() error {
	err := s.getBlockchainNetworks()
	if err != nil {
		return err
	}
	return nil
}

// Get is a generic function for sending GET request to SDNHttp
func (s *realSDNHTTP) Get(endpoint string, requestBody []byte) ([]byte, error) {
	url := s.sdnURL + endpoint
	proxyReq, err := http.NewRequest(bxgateway.GetMethod, url, bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}
	c, err := s.httpClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(proxyReq)
	if err != nil {
		return nil, err
	}
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return respBytes, nil
}

// FetchBlockchainNetwork fetches a blockchain network given the blockchain number of the model registered with SDN
func (s *realSDNHTTP) FetchBlockchainNetwork() error {
	networkNum := s.NetworkNum()
	url := fmt.Sprintf("%v/blockchain-networks/%v", s.sdnURL, networkNum)
	resp, err := s.httpWithCache(url, bxgateway.GetMethod, blockchainNetworkCacheFileName, nil)
	if err != nil {
		return err
	}
	prev, ok := s.networks[networkNum]
	if !ok {
		s.networks[networkNum] = new(sdnmessage.BlockchainNetwork)
	}
	if err = json.Unmarshal(resp, s.networks[networkNum]); err != nil {
		return fmt.Errorf("could not deserialize blockchain network (previously cached as: %v) for networkNum %v, because : %v", prev, networkNum, err)
	}
	if prev != nil && s.networks[networkNum].MinTxAgeSeconds != prev.MinTxAgeSeconds {
		log.Debugf("The MinTxAgeSeconds changed from %v seconds to %v seconds after the update", prev.MinTxAgeSeconds, s.networks[networkNum].MinTxAgeSeconds)
	}
	if s.networks[networkNum].Protocol == bxgateway.Ethereum && s.networks[networkNum].DefaultAttributes.TerminalTotalDifficulty == 0 {
		s.networks[networkNum].DefaultAttributes.TerminalTotalDifficulty = big.NewInt(math.MaxInt)
	}

	return nil
}

// InitGateway fetches all necessary information over HTTP from the SDN
func (s *realSDNHTTP) InitGateway(protocol string, network string) error {
	var err error
	s.nodeModel.Network = network
	s.nodeModel.Protocol = protocol
	s.networks = make(sdnmessage.BlockchainNetworks)

	if err = s.Register(); err != nil {
		return err
	}
	if err = s.FetchBlockchainNetwork(); err != nil {
		return err
	}
	err = s.getAccountModel(s.nodeModel.AccountID)
	if err != nil {
		return err
	}
	return nil
}

func logLowestLatency(lowestLatencyRelay nodeLatencyInfo) {
	if lowestLatencyRelay.Latency > 40 {
		log.Warnf("ping latency of the fastest relay %v:%v is %v ms, which is more than 40 ms",
			lowestLatencyRelay.IP, lowestLatencyRelay.Port, lowestLatencyRelay.Latency)
	}
	log.Infof("fastest selected relay %v:%v has a latency of %v ms",
		lowestLatencyRelay.IP, lowestLatencyRelay.Port, lowestLatencyRelay.Latency)
}

// DirectRelayConnections directs the gateway on relays to connect/disconnect
func (s realSDNHTTP) DirectRelayConnections(ctx context.Context, relayHosts string, relayLimit uint64, relayInstructions chan<- RelayInstruction, autoRelayTimeout time.Duration) error {
	overrideRelays, autoCount, err := parsedCmdlineRelays(relayHosts, relayLimit)
	if err != nil {
		return err
	}

	// connect relays specified in `relays` argument
	for ip, port := range overrideRelays {
		relayInstructions <- RelayInstruction{IP: ip, Port: port, Type: Connect}
	}

	if autoCount == 0 {
		return nil
	}

	// TODO: fetching relay from SDN should be done in a loop inside manageAutoRelays
	// if auto relays specified, start and manage them
	relays, err := s.getRelays(s.nodeModel.NodeID, s.nodeModel.BlockchainNetworkNum)
	if err != nil {
		return fmt.Errorf("failed to extract relay list - %v", err)
	}
	if len(relays) == 0 {
		return errors.New("no relays were acquired from SDN")
	}
	go s.manageAutoRelays(ctx, autoCount, relayInstructions, autoRelayTimeout, relays, overrideRelays)
	return nil
}

// parsedCmdlineRelays parses the relayHosts argument and returns relays IPs up to the relay limit
func parsedCmdlineRelays(relayHosts string, relayLimit uint64) (relayMap, int, error) {
	overrideRelays := make(relayMap)
	autoCount := 0

	if len(relayHosts) == 0 {
		return nil, 0, fmt.Errorf("no --relays/relay-ip arguments were provided")
	}
	for _, relay := range strings.Split(relayHosts, ",") {
		// Clean and get the relay string
		if uint64(len(overrideRelays)+autoCount) == relayLimit { // Only counting unique relays + auto relays
			break
		}
		suggestedRelayString := strings.Trim(relay, " ")
		if suggestedRelayString == "auto" {
			autoCount++
			continue
		}
		if suggestedRelayString == "" {
			return nil, 0, fmt.Errorf("argument to --relays/relay-ip is empty or has an extra comma")
		}
		suggestedRelaySplit := strings.Split(suggestedRelayString, ":")
		if len(suggestedRelaySplit) > 2 {
			return nil, 0, fmt.Errorf("relay from --relays/relay-ip was given in the incorrect format '%s', should be IP:Port", relay)
		}

		host := suggestedRelaySplit[0]
		port := 1809
		var err error
		// Parse the relay string

		if len(suggestedRelaySplit) == 2 { // Make sure that port is an integer
			port, err = strconv.Atoi(suggestedRelaySplit[1])
			if err != nil {
				return nil, 0, fmt.Errorf("port provided %v is not valid - %v", suggestedRelaySplit[1], err)
			}
		}
		ip, err := utils.GetIP(host)
		if err != nil {
			log.Errorf("relay %s from --relays/relay-ip is not valid - %v", suggestedRelaySplit[0], err)
			return nil, 0, err
		}
		if _, ok := overrideRelays[ip]; !ok {
			overrideRelays[ip] = int64(port)
		}
	}
	return overrideRelays, autoCount, nil
}

func (s realSDNHTTP) manageAutoRelays(ctx context.Context, autoRelayCount int, relayInstructions chan<- RelayInstruction, autoRelayRefreshInterval time.Duration, relays sdnmessage.Peers, overrideRelays relayMap) {
	pingLatencies := s.getPingLatencies(relays) // list of SDN relays sorted by ascending order of latency
	if len(pingLatencies) == 0 {
		log.Errorf("ping latencies not found for relays from SDN")
		return
	}

	autoRelayCounter := 0
	//autoRelays := nodeLatencyInfos{}
	for idx, pingLatency := range pingLatencies {
		newRelayIP, err := utils.GetIP(pingLatency.IP)
		if err != nil {
			log.Errorf("relay %s from the SDN does not have a valid IP address - %v", pingLatency.IP, err)
			continue
		}
		// only connect to the relay if not already connected to
		if _, ok := overrideRelays[newRelayIP]; ok {
			continue
		}
		logLowestLatency(pingLatencies[idx])
		relayInstructions <- RelayInstruction{IP: newRelayIP, Port: pingLatency.Port, Type: Connect}
		//autoRelays = append(autoRelays, pingLatency)

		autoRelayCounter++
		if autoRelayCounter == autoRelayCount {
			// we found all autoRelays so we are done
			return
		}
	}
	// if we are here we failed to find all needed auto relays
	log.Errorf("available SDN relays %v; requested auto count %v", autoRelayCounter, autoRelayCount)
}

// NodeModel returns the node model returned by the SDN
func (s realSDNHTTP) NodeModel() *sdnmessage.NodeModel {
	return s.nodeModel
}

// AccountTier returns the account tier name
func (s realSDNHTTP) AccountTier() sdnmessage.AccountTier {
	return s.accountModel.TierName
}

// AccountModel returns the account model
func (s realSDNHTTP) AccountModel() sdnmessage.Account {
	return *s.accountModel
}

// NetworkNum returns the registered network number of the node model
func (s realSDNHTTP) NetworkNum() types.NetworkNum {
	return s.nodeModel.BlockchainNetworkNum
}

func (s realSDNHTTP) httpClient() (*http.Client, error) {
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
func (s *realSDNHTTP) Register() error {
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
		s.nodeID = nodeID
	}

	resp, err := s.httpWithCache(s.sdnURL+"/nodes", bxgateway.PostMethod, nodeModelCacheFileName, bytes.NewBuffer(s.nodeModel.Pack()))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(resp, &s.nodeModel); err != nil {
		return fmt.Errorf("could not deserialize node model: %v", err)
	}
	accountID, err := s.sslCerts.GetAccountID()
	if err != nil {
		return err
	}

	s.nodeID = s.nodeModel.NodeID
	s.accountID = accountID

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
func (s *realSDNHTTP) NeedsRegistration() bool {
	return s.nodeID == "" || s.sslCerts.NeedsPrivateCert()
}

func (s *realSDNHTTP) close(resp *http.Response) {
	err := resp.Body.Close()
	if err != nil {
		log.Error(fmt.Errorf("unable to close response body %v error %v", resp.Body, err))
	}
}

func (s *realSDNHTTP) GetQuotaUsage(accountID string) (*QuotaResponseBody, error) {
	reqBody := quotaRequestBody{
		AccountID: accountID,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		log.Errorf("unable to marshal SDN request: %v", err)
		return nil, err
	}

	resp, err := s.Get("/accounts/quota-status", body)
	if err != nil {
		return nil, err
	}

	quotaResp := QuotaResponseBody{}
	if err = json.Unmarshal(resp, &quotaResp); err != nil {
		return nil, err
	}

	return &quotaResp, nil
}

func (s *realSDNHTTP) getAccountModelWithEndpoint(accountID types.AccountID, endpoint string) (sdnmessage.Account, error) {
	url := fmt.Sprintf("%v/%v/%v", s.sdnURL, endpoint, accountID)
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

	return s.fillInAccountDefaults(&accountModel, time.Now().UTC())
}

func (s *realSDNHTTP) fillInAccountDefaults(accountModel *sdnmessage.Account, now time.Time) (sdnmessage.Account, error) {
	mappedAccountModel := sdnmessage.GetDefaultEliteAccount(now)
	err := copier.CopyWithOption(&mappedAccountModel, *accountModel, copier.Option{IgnoreEmpty: true, DeepCopy: true})

	if err != nil {
		return *accountModel, err
	}

	return mappedAccountModel, err
}

func (s *realSDNHTTP) getAccountModel(accountID types.AccountID) error {
	accountModel, err := s.getAccountModelWithEndpoint(accountID, "account")
	s.accountModel = &accountModel
	if s.accountModel.RelayLimit.MsgQuota.Limit == 0 {
		log.Warnf("relay limit was set to 0, setting to 1")
		s.accountModel.RelayLimit.MsgQuota.Limit = 1
	}

	if s.accountModel.MaxAllowedNodes.MsgQuota.Limit == 0 {
		log.Warnf("relay max allowed nodes limit was set to 0, setting to 6")
		s.accountModel.MaxAllowedNodes.MsgQuota.Limit = 6
	}

	return err
}

// FetchCustomerAccountModel get customer account model
func (s *realSDNHTTP) FetchCustomerAccountModel(accountID types.AccountID) (sdnmessage.Account, error) {
	return s.getAccountModelWithEndpoint(accountID, "accounts")
}

// getRelays gets the potential relays for a gateway
func (s *realSDNHTTP) getRelays(nodeID types.NodeID, networkNum types.NetworkNum) (sdnmessage.Peers, error) {
	url := fmt.Sprintf("%v/nodes/%v/%v/potential-relays", s.sdnURL, nodeID, networkNum)
	resp, err := s.httpWithCache(url, bxgateway.GetMethod, potentialRelaysFileName, nil)
	if err != nil {
		return nil, err
	}
	var relays sdnmessage.Peers
	if err = json.Unmarshal(resp, &relays); err != nil {
		return nil, fmt.Errorf("could not deserialize potential relays: %v", err)
	}
	return relays, nil
}

func (s *realSDNHTTP) httpWithCache(uri string, method string, fileName string, body io.Reader) ([]byte, error) {
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

func (s *realSDNHTTP) http(uri string, method string, body io.Reader) ([]byte, error) {
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

func (s *realSDNHTTP) getBlockchainNetworks() error {
	url := fmt.Sprintf("%v/blockchain-networks", s.sdnURL)
	resp, err := s.httpWithCache(url, bxgateway.GetMethod, blockchainNetworksCacheFileName, nil)
	if err != nil {
		return err
	}
	var networks []*sdnmessage.BlockchainNetwork
	if err = json.Unmarshal(resp, &networks); err != nil {
		return fmt.Errorf("could not deserialize blockchain networks: %v", err)
	}
	s.networks = sdnmessage.BlockchainNetworks{}
	for _, network := range networks {
		s.networks[network.NetworkNum] = network
	}
	return nil
}

// FindNetwork finds a BlockchainNetwork instance by its number and allow update
func (s *realSDNHTTP) FindNetwork(networkNum types.NetworkNum) (*sdnmessage.BlockchainNetwork, error) {
	return s.networks.FindNetwork(networkNum)
}

// MinTxAge returns MinTxAge for the current blockchain number the node model registered
func (s *realSDNHTTP) MinTxAge() time.Duration {
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

// SendNodeEvent sends node event to SDN through http
func (s *realSDNHTTP) SendNodeEvent(event sdnmessage.NodeEvent, id types.NodeID) {
	url := fmt.Sprintf("%v/nodes/%v/events", s.sdnURL, id)
	eventBytes, err := json.Marshal(event)
	if err != nil {
		log.Errorf("error in sending node event to SDN through http, can't serialize node event, %v", err)
	}
	resp, err := s.http(url, bxgateway.PostMethod, bytes.NewBuffer(eventBytes))
	if err != nil {
		log.Errorf("error in sending node event to SDN through http, %v", err)
	}
	log.Infof("node event %v sent to SDN, resp is %v", event.EventType, resp)
}

// SDNURL getter for the private sdnURL field
func (s *realSDNHTTP) SDNURL() string {
	return s.sdnURL
}

// NodeID getter for the private nodeID field
func (s *realSDNHTTP) NodeID() types.NodeID {
	return s.nodeID
}

// Networks getter for the private networks field
func (s *realSDNHTTP) Networks() *sdnmessage.BlockchainNetworks {
	return &s.networks
}

// SetNetworks setter for the private networks field
func (s *realSDNHTTP) SetNetworks(networks sdnmessage.BlockchainNetworks) {
	s.networks = networks
}
