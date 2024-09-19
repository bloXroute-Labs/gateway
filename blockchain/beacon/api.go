package beacon

import (
	"bytes"
	"context"
	"encoding/hex"
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
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	prysmTypes "github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/r3labs/sse"
	"golang.org/x/sync/errgroup"
)

// For the lighthouse client, we make different block encoding for now
// (JSON instead of SSZ), cause it doesn't support the SZZ.
const (
	lighthouse       = "lighthouse"
	keepAliveMessage = ""
)

type nodeVersionResponse struct {
	Data struct {
		Version string `json:"version"`
	} `json:"data"`
}

const (
	// Beacon API routes
	requestBlockRoute         = "http://%s/eth/v2/beacon/blocks/%s"
	requestBlobSidecarRoute   = "http://%s/eth/v1/beacon/blob_sidecars/%s?indices=%s"
	requestClientVersionRoute = "http://%s/eth/v1/node/version"
	subscribeEventRoute       = "http://%s/eth/v1/events?topics=%s"
	broadcastBlockRoute       = "http://%s/eth/v1/beacon/blocks"

	// topics for the event stream
	topicsNewBlockHead    = "head"
	topicsNewBlobsSidecar = "blob_sidecar"
)

type blobDecoder interface {
	decodeBlobSidecar([]byte) (*ethpb.BlobSidecars, error)
}

func newDefaultBlobDecoder() blobDecoder {
	return &defaultBlobDecoder{}
}

type defaultBlobDecoder struct{}

func newLightHouseBlobDecoder() blobDecoder {
	return &lightHouseBlobDecoder{}
}

type lightHouseBlobDecoder struct {
	defaultBlobDecoder
}

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
	blobDecoder  blobDecoder
	initialized  atomic.Bool
	sharedSync   *APISharedSync
	isConnected  bool
}

// NewAPIClient creates a new APIClient with the specified URL.
func NewAPIClient(ctx context.Context, httpClient *http.Client, config *network.EthConfig, bridge blockchain.Bridge, url string, endpoint types.NodeEndpoint,
	sharedSync *APISharedSync,
) *APIClient {
	return &APIClient{
		ctx: ctx,
		URL: url,
		log: log.WithFields(log.Fields{
			"connType":   "beaconApi",
			"remoteAddr": url,
		}),
		bridge:       bridge,
		config:       config,
		clock:        utils.RealClock{},
		httpClient:   httpClient,
		blobDecoder:  newDefaultBlobDecoder(),
		nodeEndpoint: endpoint,
		initialized:  atomic.Bool{},
		sharedSync:   sharedSync,
	}
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBodyRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	version := resp.Header.Get("Eth-Consensus-Version")

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
		select {
		case <-c.ctx.Done():
			return
		default:
			version, err = c.requestClientVersion()
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				c.log.Errorf("error retrieving beacon client version: %v", err)
				break
			}

			if strings.Contains(version, lighthouse) {
				c.blobDecoder = newLightHouseBlobDecoder()
			}

			c.initialized.Store(true)

			return
		}

		time.Sleep(time.Second * 10) // wait before retrying
	}
}

// Start listens for events from the Beacon API event stream.
func (c *APIClient) Start() error {
	g := &errgroup.Group{}
	g.Go(func() error {
		c.requestClientVersionUntilSuccess()
		return nil
	})
	g.Go(func() error {
		c.subscribeToEvents(fmt.Sprintf(subscribeEventRoute, c.URL, topicsNewBlockHead), c.blockHeadEventHandler())
		return nil
	})
	g.Go(func() error {
		c.subscribeToEvents(fmt.Sprintf(subscribeEventRoute, c.URL, topicsNewBlobsSidecar), c.blobSidecarEventHandler())
		return nil
	})

	return g.Wait()
}

// subscribeToEvents sets up a subscription to server-sent events from the beacon chain API.
func (c *APIClient) subscribeToEvents(eventsURL string, handler func(msg *sse.Event)) {
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

			err := client.SubscribeRawWithContext(c.ctx, handler)

			// If the context was canceled, we're shutting down.
			if errors.Is(err, context.Canceled) {
				return
			}
			disconnectEventErr := c.bridge.SendBlockchainConnectionStatus(blockchain.ConnectionStatus{PeerEndpoint: c.nodeEndpoint, IsConnected: false})
			if disconnectEventErr != nil {
				log.Errorf("was not able to set node status to not connected, err: %v", err)
			} else {
				c.isConnected = false
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

func (defaultBlobDecoder) decodeBlobSidecar(responseBody []byte) (*ethpb.BlobSidecars, error) {
	blobSidecar := new(ethpb.BlobSidecars)

	if err := blobSidecar.UnmarshalSSZ(responseBody); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %v", err)
	}

	return blobSidecar, nil
}

func (d lightHouseBlobDecoder) decodeBlobSidecar(responseBody []byte) (*ethpb.BlobSidecars, error) {
	// Check if the response is missing the offset
	// Currently LightHouse doesn't include the offset in the response, but it's required for SSZ unmarshalling
	var offset uint64

	if offset = ssz.ReadOffset(responseBody[0:4]); offset > uint64(len(responseBody)) {
		return nil, ssz.ErrInvalidVariableOffset
	}

	responseBody = append([]byte{4, 0, 0, 0}, responseBody...)

	return d.defaultBlobDecoder.decodeBlobSidecar(responseBody)
}

func (c *APIClient) retry(fn func() error, maxTry int, sleep time.Duration) (attempt int, err error) {
	for ; attempt < maxTry; attempt++ {
		err = fn()
		if err == nil {
			break
		}
		time.Sleep(sleep)
	}
	return attempt + 1, err
}

func (c *APIClient) requestBlobSidecarWithRetries(beaconHash string, index string) (*ethpb.BlobSidecars, error) {
	uri := fmt.Sprintf(requestBlobSidecarRoute, c.URL, beaconHash, index)
	req, err := c.newRequest(uri)
	if err != nil {
		return nil, fmt.Errorf("failed to make new request to Beacon API route: %v", err)
	}

	var respBodyRaw []byte

	requestBlobSidecar := func() error {
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		respBodyRaw, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %v", err)
		}

		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("blob sidecars not found yet for hash: %s, index: %s", beaconHash, index)
		}
		return nil
	}

	attempt, err := c.retry(requestBlobSidecar, 10, time.Millisecond*40)
	if err != nil {
		return nil, err
	}

	c.log.Tracef("received blob sidecar from Beacon API for [hash: %s, index: %s] after %d attempts", beaconHash, index, attempt)

	return c.blobDecoder.decodeBlobSidecar(respBodyRaw)
}

func (c *APIClient) isNeedToRequestBlobSidecar(blobSidecarEvent *structs.BlobSidecarEvent) bool {
	eventSlot, err := strconv.ParseUint(blobSidecarEvent.Slot, 10, 64)
	if err != nil {
		c.log.Errorf("could not parse slot from blob sidecar event, slot data: %s, err: %v ", blobSidecarEvent.Slot, err)
		return false
	}

	index, err := strconv.ParseUint(blobSidecarEvent.Index, 10, 64)
	if err != nil {
		c.log.Errorf("could not parse index from blob sidecar event, index data: %s, err: %v ", blobSidecarEvent.Index, err)
		return false
	}

	needToRequestBlobSidecar, err := c.sharedSync.needToRequestBlobSidecar(eventSlot, index)
	if err != nil {
		c.log.Errorf("%v", err)
	}
	if !needToRequestBlobSidecar {
		c.log.Tracef("blob sidecar request already processed: slot: %s, index: %d, block hash: %s", blobSidecarEvent.Slot, index, blobSidecarEvent.BlockRoot)
	}
	return needToRequestBlobSidecar
}

// blobSidecarEventHandler returns a function to handle server-sent events.
func (c *APIClient) blobSidecarEventHandler() func(msg *sse.Event) {
	handleBlobSidecarEvent := func(eventData []byte) {
		if string(eventData) == keepAliveMessage {
			return
		}

		c.log.Tracef("received blob sidecar event: %s", string(eventData))

		var blobSidecarEvent structs.BlobSidecarEvent

		err := json.Unmarshal(eventData, &blobSidecarEvent)
		if err != nil {
			c.log.Errorf("could not unmarshal blob sidecar event: %s, err: %v ", string(eventData), err)
			return
		}

		if !c.isNeedToRequestBlobSidecar(&blobSidecarEvent) {
			return
		}

		// retry to request blob sidecars in case of failure
		blobSidecars, err := c.requestBlobSidecarWithRetries(blobSidecarEvent.BlockRoot, blobSidecarEvent.Index)
		if err != nil {
			c.log.Tracef("failed to request blob sidecar: %v", err)
			return
		}

		for _, sidecar := range blobSidecars.Sidecars {
			// validate the blob sidecar
			bdnSidecar, err := c.bridge.BeaconMessageToBDN(sidecar)
			if err != nil {
				c.log.Errorf("could not convert blob sidecar to BDN: %v", err)
				continue
			}

			if err := c.bridge.SendBeaconMessageToBDN(bdnSidecar, c.nodeEndpoint); err != nil {
				c.log.Errorf("could not send blob sidecar to BDN: %v", err)
				continue
			}

			headerRoot, err := sidecar.SignedBlockHeader.Header.HashTreeRoot()
			if err != nil {
				c.log.Errorf("could not get header root: %v", err)
				continue
			}

			c.log.Tracef("propagated blob sidecar from Beacon API to BDN: index %v, slot %v, kzgCommitment %v, block hash %s", sidecar.Index, sidecar.SignedBlockHeader.Header.Slot, hex.EncodeToString(sidecar.KzgCommitment), hex.EncodeToString(headerRoot[:]))
		}
	}

	return func(msg *sse.Event) {
		go handleBlobSidecarEvent(msg.Data)
	}
}

// blockHeadEventHandler returns a function to handle server-sent events.
// The returned function processes head events, gets blocks and sends them to BDN.
func (c *APIClient) blockHeadEventHandler() func(msg *sse.Event) {
	return func(msg *sse.Event) {
		// if data is empty, its keep alive msg and we ignore it
		if string(msg.Data) == keepAliveMessage {
			return
		}
		data, err := c.unmarshalHeadEvent(msg.Data)
		if err != nil {
			c.log.Errorf("could not unmarshal head event: %s, err: %v ", string(msg.Data), err)
			return
		}

		if !c.isConnected {
			// if we were not able to set the node to connected, we will try again next block
			if err := c.bridge.SendBlockchainConnectionStatus(blockchain.ConnectionStatus{PeerEndpoint: c.nodeEndpoint, IsConnected: true}); err != nil {
				log.Errorf("was not able to set node status to connected, err: %v", err)
			} else {
				c.isConnected = true
			}
		}

		if c.sharedSync.isKnownSlot(data.Slot, true) {
			c.log.Tracef("skip processing already processed block[slot=%d]", data.Slot)
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

		if err := SendBlockToBDN(c.clock, c.log, wrappedBlock, c.bridge, c.nodeEndpoint); err != nil {
			c.log.Errorf("could not proccess beacon block[slot=%d,hash=%s] to eth: %v", block.Block().Slot(), blockHash, err)
			return
		}

		c.log.Tracef("received beacon block[slot=%d,hash=%s]", block.Block().Slot(), blockHash)
	}
}

// unmarshalHeadEvent unmarshals a server-sent event into a headEventData instance.
func (c *APIClient) unmarshalHeadEvent(eventData []byte) (headEventData, error) {
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
func (c *APIClient) BroadcastBlock(block *ethpb.SignedBeaconBlockContentsDeneb) error {
	if !c.initialized.Load() {
		return fmt.Errorf("unknown client version")
	}

	blockSlot := uint64(block.GetBlock().GetBlock().GetSlot())
	if c.sharedSync.isKnownSlot(blockSlot, false) {
		c.log.Tracef("skip broadcast already processed block[slot=%d]", blockSlot)
		return nil
	}

	uri := fmt.Sprintf(broadcastBlockRoute, c.URL)

	rawBlock, err := block.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("failed to prepare block: %v", err)
	}

	req, err := http.NewRequestWithContext(c.ctx, http.MethodPost, uri, bytes.NewReader(rawBlock))
	if err != nil {
		return fmt.Errorf("failed to create new request: %v", err)
	}

	req.Header.Set("Eth-Consensus-Version", version.String(version.Deneb))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Accept", "application/json")

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
