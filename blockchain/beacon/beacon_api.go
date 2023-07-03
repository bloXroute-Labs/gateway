package beacon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	ethcommon "github.com/ethereum/go-ethereum/common"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/prysm/v4/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v4/consensus-types/interfaces"
	prysmTypes "github.com/prysmaticlabs/prysm/v4/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v4/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v4/runtime/version"
	"github.com/r3labs/sse"
)

// APIClient represents the client for subscribing to the Beacon API event stream.
type APIClient struct {
	URL          string
	log          *log.Entry
	bridge       blockchain.Bridge
	config       *network.EthConfig
	clock        utils.Clock
	ctx          context.Context
	httpClient   *http.Client
	nodeEndpoint types.NodeEndpoint
}

// NewAPIClient creates a new APIClient with the specified URL.
func NewAPIClient(ctx context.Context, httpClient *http.Client, config *network.EthConfig, bridge blockchain.Bridge, url, blockchainNetwork string) *APIClient {
	log := log.WithFields(log.Fields{
		"connType":   "beaconApi",
		"remoteAddr": url,
	})

	return &APIClient{
		ctx:          ctx,
		URL:          url,
		log:          log,
		bridge:       bridge,
		config:       config,
		clock:        utils.RealClock{},
		httpClient:   httpClient,
		nodeEndpoint: createBeaconAPIEndpoint(url, blockchainNetwork),
	}
}

const requestBlockRoute = "http://%s/eth/v2/beacon/blocks/%s"

func (c *APIClient) requestBlock(hash string) (interfaces.ReadOnlySignedBeaconBlock, error) {
	uri := fmt.Sprintf(requestBlockRoute, c.URL, hash)
	req, err := c.newRequest(uri)
	if err != nil {
		return nil, fmt.Errorf("failed to make new request to Beacon API route: %v", err)
	}

	respBodyRaw, version, err := c.doRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request the Beacon API route: %v", err)
	}

	block, err := c.processResponse(respBodyRaw, version, hash)
	if err != nil {
		return nil, err
	}

	return block, nil
}

func (c *APIClient) newRequest(uri string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(c.ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	return req, nil
}

func (c *APIClient) doRequest(req *http.Request) ([]byte, string, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	respBodyRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response body: %v", err)
	}

	return respBodyRaw, resp.Header.Get("Eth-Consensus-Version"), nil
}

func (c *APIClient) processResponse(respBodyRaw []byte, v, hash string) (interfaces.ReadOnlySignedBeaconBlock, error) {
	var rawBlock ssz.Unmarshaler
	switch v {
	case version.String(version.Bellatrix):
		rawBlock = &ethpb.SignedBlindedBeaconBlockBellatrix{}
	case version.String(version.Altair):
		rawBlock = &ethpb.SignedBeaconBlockAltair{}
	case version.String(version.Phase0):
		rawBlock = &ethpb.SignedBeaconBlock{}
	default:
		// Not all the clients support the block's version in the HTTP header of the response.
		// If version didn't mention - use the last one.
		rawBlock = &ethpb.SignedBeaconBlockCapella{}
	}
	if err := rawBlock.UnmarshalSSZ(respBodyRaw); err != nil {
		return nil, fmt.Errorf("[hash=%s,version=%s], failed to unmarshal response body: %s, err: %v", hash, v, string(respBodyRaw), err)
	}

	return blocks.NewSignedBeaconBlock(rawBlock)
}

func createBeaconAPIEndpoint(url, blockchainNetwork string) types.NodeEndpoint {
	urlSplitted := strings.Split(url, ":")
	port, _ := strconv.Atoi(urlSplitted[1])

	return types.NodeEndpoint{
		IP:                urlSplitted[0],
		Port:              port,
		PublicKey:         "none",
		IsBeacon:          true,
		BlockchainNetwork: blockchainNetwork,
		Name:              "Beacon API",
		ConnectedAt:       time.Now().Format(time.RFC3339),
	}
}

type headEventData struct {
	Slot  uint64 `json:"slot,string"`
	Block string `json:"block"`
	State string `json:"state"`
}

// Start listens for events from the Beacon API event stream.
func (c *APIClient) Start() {
	go c.subscribeToEvents()
}

// subscribeToEvents sets up a subscription to server-sent events from the beacon chain API.
func (c *APIClient) subscribeToEvents() {
	eventsURL := c.eventsURL()
	client := sse.NewClient(eventsURL)
	for {
		c.log.Info("subscribing to head events ", eventsURL)

		err := client.SubscribeRawWithContext(c.ctx, c.eventHandler())

		if err != nil {
			c.log.Errorf("failed to subscribe to head events")
		} else {
			c.log.Warnf("APIClient SubscribeRaw ended, reconnecting: %v", c.URL)
		}
		time.Sleep(10 * time.Second)
	}
}

// eventsURL returns the URL for the beacon chain API's server-sent events.
const subscribeBlockEventRoute = "http://%s/eth/v1/events?topics=head"

func (c *APIClient) eventsURL() string {
	return fmt.Sprintf(subscribeBlockEventRoute, c.URL)
}

// eventHandler returns a function to handle server-sent events.
// The returned function processes head events, gets blocks and sends them to BDN.
func (c *APIClient) eventHandler() func(msg *sse.Event) {
	return func(msg *sse.Event) {
		data, err := c.unmarshalEvent(msg.Data)
		if err != nil {
			c.log.Errorf("could not unmarshal head event: %s, err: %v ", string(msg.Data), err)
			return
		}

		block, err := c.requestBlock(data.Block)
		if err != nil {
			c.log.Errorf("error in getting block: %v", err)
			return
		}

		blockHash, err := c.hashOfBlock(block)
		if (err != nil) || (blockHash != data.Block) {
			c.log.Errorf("could not approve beacon block[slot=%d,hash=%s]: %v", block.Block().Slot(), data.Block, err)
			return
		}

		if c.isOldBlock(block) {
			c.log.Errorf("block[slot=%d,hash=%s] is too old to process", block.Block().Slot(), blockHash)
			return
		}

		if err := sendBlockToBDN(c.clock, c.log, block, c.bridge, c.nodeEndpoint); err != nil {
			c.log.Errorf("could not proccess beacon block[slot=%d,hash=%s] to eth: %v", block.Block().Slot(), blockHash, err)
			return
		}

		c.log.Tracef("received beacon block[slot=%d,hash=%s]", block.Block().Slot(), blockHash)
	}
}

// unmarshalEvent unmarshals a server-sent event into a headEventData instance.
func (c *APIClient) unmarshalEvent(eventData []byte) (headEventData, error) {
	var data headEventData
	err := json.Unmarshal(eventData, &data)
	return data, err
}

// hashOfBlock returns the hash of a beacon block.
func (c *APIClient) hashOfBlock(block interfaces.ReadOnlySignedBeaconBlock) (string, error) {
	blockHash, err := block.Block().HashTreeRoot()
	if err != nil {
		return "", err
	}
	return ethcommon.BytesToHash(blockHash[:]).String(), nil
}

// isOldBlock checks whether a beacon block is too old to be processed.
func (c *APIClient) isOldBlock(block interfaces.ReadOnlySignedBeaconBlock) bool {
	return block.Block().Slot() <= currentSlot(c.config.GenesisTime)-prysmTypes.Slot(c.config.IgnoreSlotCount)
}
