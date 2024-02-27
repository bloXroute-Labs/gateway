package beacon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
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

// For the lighthouse client, we make different block encoding for now
// (JSON instead of SSZ), cause it doesn't support the SZZ.
const lighthouse = "lighthouse"

type nodeVersionResponse struct {
	Data struct {
		Version string `json:"version"`
	} `json:"data"`
}

// Used Beacon API routes
const (
	requestBlockRoute         = "http://%s/eth/v2/beacon/blocks/%s"
	requestClientVersionRoute = "http://%s/eth/v1/node/version"
	subscribeBlockEventRoute  = "http://%s/eth/v1/events?topics=head"
	broadcastBlockRoute       = "http://%s/eth/v1/beacon/blocks"
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
	nodeEndpoint *types.NodeEndpoint
	blockEncoder consensusBlockEncoder
	initialized  atomic.Bool
}

// NewAPIClient creates a new APIClient with the specified URL.
func NewAPIClient(ctx context.Context, httpClient *http.Client, config *network.EthConfig, bridge blockchain.Bridge, url, blockchainNetwork string) (*APIClient, error) {
	log := log.WithFields(log.Fields{
		"connType":   "beaconApi",
		"remoteAddr": url,
	})

	client := &APIClient{
		ctx:          ctx,
		URL:          url,
		log:          log,
		bridge:       bridge,
		config:       config,
		clock:        utils.RealClock{},
		httpClient:   httpClient,
		blockEncoder: nil,
	}

	var err error

	client.nodeEndpoint, err = CreateAPIEndpoint(url, blockchainNetwork)
	if err != nil {
		return nil, fmt.Errorf("error creating Beacon API endpoint: %v", err)
	}

	return client, nil
}

// CreateAPIEndpoint creates NodeEndpoint object from uri:port string of beacon API endpoint
func CreateAPIEndpoint(url, blockchainNetwork string) (*types.NodeEndpoint, error) {
	urlSplitted := strings.Split(url, ":")
	port, err := strconv.Atoi(urlSplitted[1])

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve endpoint port: %v", err)
	}

	return &types.NodeEndpoint{
		IP:                urlSplitted[0],
		Port:              port,
		PublicKey:         "BeaconAPI",
		IsBeacon:          true,
		BlockchainNetwork: blockchainNetwork,
		Name:              "BeaconAPI",
		ConnectedAt:       time.Now().Format(time.RFC3339),
	}, nil
}

func (c *APIClient) requestClientVersion() (string, error) {
	uri := fmt.Sprintf(requestClientVersionRoute, c.URL)

	req, err := http.NewRequestWithContext(c.ctx, http.MethodGet, uri, nil)
	if err != nil {
		return "", fmt.Errorf("error in creating request")
	}
	req.Header.Set("accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending the request: %v", err)
	}
	defer resp.Body.Close()

	var nodeVersionBody nodeVersionResponse
	err = json.NewDecoder(resp.Body).Decode(&nodeVersionBody)
	if err != nil {
		return "", fmt.Errorf("error in decoding the request body: %v", err)
	}

	return strings.ToLower(nodeVersionBody.Data.Version), nil
}

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
	case version.String(version.Capella):
		rawBlock = &ethpb.SignedBeaconBlockCapella{}
	default:
		// Not all the clients support the block's version in the HTTP header of the response.
		// If version didn't mention - use the last one.
		rawBlock = &ethpb.SignedBeaconBlockDeneb{}
	}
	if err := rawBlock.UnmarshalSSZ(respBodyRaw); err != nil {
		return nil, fmt.Errorf("[hash=%s,version=%s], failed to unmarshal response body: %s, err: %v", hash, v, string(respBodyRaw), err)
	}

	return blocks.NewSignedBeaconBlock(rawBlock)
}

type headEventData struct {
	Slot  uint64 `json:"slot,string"`
	Block string `json:"block"`
	State string `json:"state"`
}

// Trying to request client version until success.
// Until then, don't process anything that related to this
// client connection.
func (c *APIClient) requestClientVersionUntilSuccess() {
	var version string
	var err error

	for {
		version, err = c.requestClientVersion()
		if err == nil {
			c.log.Infof("Received beacon client verion: %s", version)
			break
		}

		c.log.Errorf("Error retrieving beacon client version: %v", err)
		time.Sleep(time.Second * 10) // Wait before retrying
	}

	if strings.Contains(version, lighthouse) {
		c.blockEncoder = newJSONConsensusBlockEncoder()
	} else {
		c.blockEncoder = newSSZConsensusBlockEncoder()
	}

	c.initialized.Store(true)
}

// Start listens for events from the Beacon API event stream.
func (c *APIClient) Start() {
	go c.requestClientVersionUntilSuccess()
	go c.subscribeToEvents()
}

// subscribeToEvents sets up a subscription to server-sent events from the beacon chain API.
func (c *APIClient) subscribeToEvents() {
	eventsURL := fmt.Sprintf(subscribeBlockEventRoute, c.URL)
	client := sse.NewClient(eventsURL)
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			if !c.initialized.Load() {
				c.log.Debugf("Waiting for beacon API client to be initialized...")
				break
			}
			c.log.Info("subscribing to head events ", eventsURL)

			err := client.SubscribeRawWithContext(c.ctx, c.eventHandler())

			// If the context was canceled, we're shutting down.
			if errors.Is(err, context.Canceled) {
				return
			}

			if err != nil {
				c.log.Errorf("failed to subscribe to head events: %v", err)
			} else {
				c.log.Warnf("APIClient SubscribeRaw ended, reconnecting: %v", c.URL)
			}
		}

		time.Sleep(10 * time.Second)
	}
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

		if c.isOldBlock(block) {
			c.log.Errorf("block slot=%d is too old to process", block.Block().Slot())
			return
		}

		wrappedBlock := NewWrappedReadOnlySignedBeaconBlock(block)
		blockHash, err := c.hashOfBlock(wrappedBlock)
		if (err != nil) || (blockHash != data.Block) {
			c.log.Errorf("could not approve beacon block[slot=%d,hash=%s]: %v", block.Block().Slot(), data.Block, err)
			return
		}

		if err := SendBlockToBDN(c.clock, c.log, wrappedBlock, c.bridge, *c.nodeEndpoint); err != nil {
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
func (c *APIClient) hashOfBlock(block WrappedReadOnlySignedBeaconBlock) (string, error) {
	blockHash, err := block.HashTreeRoot()
	if err != nil {
		return "", err
	}
	return ethcommon.BytesToHash(blockHash[:]).String(), nil
}

// isOldBlock checks whether a beacon block is too old to be processed.
func (c *APIClient) isOldBlock(block interfaces.ReadOnlySignedBeaconBlock) bool {
	return block.Block().Slot() <= currentSlot(c.config.GenesisTime)-prysmTypes.Slot(c.config.IgnoreSlotCount)
}

// BroadcastBlock sends the block in octet-stream format to the beacon API endpoint
func (c *APIClient) BroadcastBlock(block interfaces.ReadOnlySignedBeaconBlock) error {
	if !c.initialized.Load() {
		return fmt.Errorf("unknown client version")
	}

	uri := fmt.Sprintf(broadcastBlockRoute, c.URL)

	rawBlock, err := c.blockEncoder.encodeBlock(block)
	if err != nil {
		return fmt.Errorf("failed to prepare block: %v", err)
	}

	req, err := http.NewRequestWithContext(c.ctx, http.MethodPost, uri, bytes.NewReader(rawBlock))
	if err != nil {
		return fmt.Errorf("failed to create new request: %v", err)
	}

	req.Header.Set("Eth-Consensus-Version", version.String(block.Version()))
	req.Header.Set("Content-Type", c.blockEncoder.contentType())
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// http.StatusAccepted == 202
	// {"code":202,"message":"The block failed validation, but was successfully broadcast anyway. It was not integrated into the beacon node's database."}"
	//
	// This message on the node side means "Ignoring already known beacon payload"
	// So it's don't need to drop any error in this case

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %v", err)
		}
		return fmt.Errorf("broadcasting block failed with status code %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
