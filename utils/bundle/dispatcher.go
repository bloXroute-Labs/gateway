package bundle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/sourcegraph/jsonrpc2"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

const (
	flashbotAuthHeader = "X-Flashbots-Signature"
)

// Builder represents a MEV builder
type Builder struct {
	Name              string   `json:"name"`
	Endpoints         []string `json:"endpoints"`
	SignatureRequired bool     `json:"signature_required"`
}

// Dispatcher is responsible for dispatching MEV bundles to MEV builders
type Dispatcher struct {
	stats    statistics.Stats
	client   *http.Client
	builders map[string]*Builder
}

// NewDispatcher creates a new NewDispatcher
func NewDispatcher(stats statistics.Stats, builders map[string]*Builder) *Dispatcher {
	for name, builder := range builders {
		builder.Name = name
	}

	return &Dispatcher{
		stats: stats,
		client: &http.Client{
			Transport: &http.Transport{
				MaxConnsPerHost:     100,
				MaxIdleConnsPerHost: 100,
				MaxIdleConns:        100,
				IdleConnTimeout:     0 * time.Second,
			},
			Timeout: 60 * time.Second,
		},
		builders: builders,
	}
}

// Dispatch dispatches the MEV bundle to the MEV builders
func (d *Dispatcher) Dispatch(mevBundle *bxmessage.MEVBundle) error {
	// we create a bundle pointer, and if builders map is empty we create copy of the bundle,
	// we change it because we use mevBundle pointer in go routine in other place
	bundle := mevBundle
	if len(mevBundle.MEVBuilders) == 0 {
		bundle = mevBundle.Clone()
		bundle.MEVBuilders = map[string]string{
			bxgateway.BloxrouteBuilderName: "",
		}
	}
	if len(d.builders) == 0 {
		log.Warnf("received mevBundle message, but mev-builders-file-path is empty. Message %v from %v in network %v", bundle.BundleHash, bundle.SourceID(), bundle.GetNetworkNum())
		return nil
	}

	bundleForBuildersJSON, err := d.bundleForBuildersJSON(bundle)
	if err != nil {
		return fmt.Errorf("failed to create new mevBundle http request for bundleHash: %v, err: %v", bundle.BundleHash, err)
	}

	d.makeRequests(bundle, bundleForBuildersJSON)

	return nil
}

func (d *Dispatcher) bundleForBuildersJSON(bundle *bxmessage.MEVBundle) ([]byte, error) {
	// Important: no internal fields should be here
	params := []jsonrpc.RPCSendBundle{
		{
			Txs:               bundle.Transactions,
			UUID:              bundle.UUID,
			BlockNumber:       bundle.BlockNumber,
			MinTimestamp:      bundle.MinTimestamp,
			MaxTimestamp:      bundle.MaxTimestamp,
			RevertingTxHashes: bundle.RevertingHashes,
			Boost:             false, // set to `false` specifically for the 'beaverbuild' builder
			AccountID:         bundle.OriginalSenderAccountID,
			BlocksCount:       bundle.BlocksCount,
			DroppingTxHashes:  bundle.DroppingTxHashes,
		},
	}

	if bundle.GetNetworkNum() == bxgateway.BSCMainnetNum {
		params[0].AvoidMixedBundles = bundle.AvoidMixedBundles
		params[0].RefundRecipient = bundle.IncomingRefundRecipient
	}

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create new mevBundle http request for bundleHash: %v, err: %v", bundle.BundleHash, err)
	}

	mevBundle := jsonrpc2.Request{
		Params: (*json.RawMessage)(&paramsBytes),
		Method: string(jsonrpc.RPCEthSendBundle),
	}

	res, err := json.Marshal(mevBundle)
	if err != nil {
		return nil, fmt.Errorf("failed to create new mevBundle http request bundleHash: %v, err: %v", bundle.BundleHash, err)
	}

	return res, nil
}

// makeRequests concurrently sends provided bundle to MEVBuilders
func (d *Dispatcher) makeRequests(bundle *bxmessage.MEVBundle, json []byte) {
	mevBuilders := bundle.MEVBuilders
	_, ok := bundle.MEVBuilders[bxgateway.AllBuilderName]
	if ok {
		mevBuilders = d.getAllBuilders()
	}

	var wg = new(sync.WaitGroup)
	for builderName := range mevBuilders {
		builder := d.getBuilder(builderName)
		if builder == nil {
			continue
		}

		for _, endpoint := range builder.Endpoints {
			wg.Add(1)
			go func(endpoint string) {
				d.sendBundleToBuilder(endpoint, builder, bundle, json)
				wg.Done()
			}(endpoint)
		}
	}

	wg.Wait()
}

type bundleRequest struct {
	bundleHash  string
	blockNumber string
	method      string
	request     *http.Request
}

// String returns a string representation of the request
func (r *bundleRequest) String() string {
	return fmt.Sprintf("bundleHash: %v, blockNumber: %v, endpoint: %s, method: %v", r.bundleHash, r.blockNumber, r.request.URL, r.method)
}

// sendBundleToBuilder sends MEVBundle or CancelBundle request to MEVBuilder
func (d *Dispatcher) sendBundleToBuilder(endpoint string, builder *Builder, bundle *bxmessage.MEVBundle, json []byte) {
	lg := log.WithFields(log.Fields{
		"builder":         builder.Name,
		"builderEndpoint": endpoint,
		"bundleHash":      bundle.BundleHash,
	})

	var bundleRq *bundleRequest
	var err error
	var cancelBundle bool

	if builder.Name == bxgateway.FlashbotsBuilderName && len(bundle.Transactions) == 0 {
		cancelBundle = true
		bundleRq, err = d.cancelBundleRequest(endpoint, bundle)
		if err != nil {
			lg.Errorf("failed to create cancelBundleRequest for MEVBuilder: %s", err)
			return
		}
	} else {
		bundleRq, err = d.bundleRequest(endpoint, builder, bundle, json)
		if err != nil {
			lg.Errorf("failed to create bundleRequest for MEVBuilder: %s", err)
			return
		}
	}

	rsp, err := d.do(bundleRq, json)
	if err != nil {
		lg.Errorf("do HTTP request to MEVBuilder: %s", err)
		return
	}

	if cancelBundle {
		lg.Tracef("sent CancelMEVBundle to MEVBuilder (request: %s, json: '%s') got response: %v, status code: %v", bundleRq, json, string(rsp.body), rsp.code)
	} else {
		lg.Tracef("sent MEVBundle to MEVBuilder (request: %s, json: '%s') got response: %v, status code: %v", bundleRq, json, string(rsp.body), rsp.code)
		if rsp.code != http.StatusOK {
			d.stats.AddBuilderGatewaySentBundleToMEVBuilderEvent(
				rsp.estimatedBundleReceivedTime,
				builder.Name,
				bundle.BundleHash,
				bundle.BlockNumber,
				bundle.UUID,
				endpoint,
				bundle.GetNetworkNum(),
				types.AccountID(bundle.OriginalSenderAccountID),
				bundle.OriginalSenderAccountTier,
				rsp.code,
			)
		}
	}
}

type doResult struct {
	code                        int
	body                        []byte
	estimatedBundleReceivedTime time.Time
}

// do performs HTTP request to provided MEVBuilder and records BuilderGatewaySentBundleToBuilder event
func (d *Dispatcher) do(bundleRq *bundleRequest, json []byte) (rsp *doResult, err error) {
	timeBeforeSendingBundle := time.Now()
	resp, err := d.client.Do(bundleRq.request)
	if err != nil {
		return nil, fmt.Errorf("forward MEVBundle (request: %s, json: '%s'): %v", bundleRq, json, err)
	}

	defer func() { _ = resp.Body.Close() }()

	sendingDuration := time.Since(timeBeforeSendingBundle)
	estimatedBundleReceivedTime := timeBeforeSendingBundle.Add(sendingDuration / 2)
	rspBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read MEVBuilder response (request: %s, json: '%s'): %v", bundleRq, json, err)
	}

	return &doResult{
		code:                        resp.StatusCode,
		body:                        rspBody,
		estimatedBundleReceivedTime: estimatedBundleReceivedTime,
	}, nil
}

func (d *Dispatcher) getBuilder(builder string) *Builder {
	if len(d.builders) > 0 {
		return d.builders[builder]

	}

	return nil
}

func (d *Dispatcher) getAllBuilders() map[string]string {
	builders := make(map[string]string)
	for name, builder := range d.builders {
		builders[name] = builder.Name
	}

	return builders
}

func (d *Dispatcher) bundleRequest(endpoint string, builder *Builder, bundle *bxmessage.MEVBundle, json []byte) (*bundleRequest, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(json))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if builder.SignatureRequired {
		if bundle.MEVBuilders[builder.Name] == "" {
			flashbotsSignature, err := generateRandomFlashbotsSignature(json)
			if err != nil {
				return nil, fmt.Errorf("failed to create random flashbots signature: %v", err)
			}

			req.Header.Set(flashbotAuthHeader, flashbotsSignature)
		} else {
			req.Header.Set(flashbotAuthHeader, bundle.MEVBuilders[bxgateway.FlashbotsBuilderName])
		}
	}

	return &bundleRequest{
		bundleHash:  bundle.BundleHash,
		blockNumber: bundle.BlockNumber,
		method:      string(jsonrpc.RPCEthSendBundle),
		request:     req,
	}, nil
}

func (d *Dispatcher) cancelBundleRequest(endpoint string, bundle *bxmessage.MEVBundle) (*bundleRequest, error) {
	params := []jsonrpc.RPCCancelBundlePayload{
		{
			ReplacementUUID: bundle.UUID,
		},
	}
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: %v", err)
	}
	cancelFlashbotsBundlePayload := jsonrpc2.Request{
		ID:     jsonrpc2.ID{Str: "1", Num: 1, IsString: false},
		Params: (*json.RawMessage)(&paramsBytes),
		Method: string(jsonrpc.RPCEthCancelBundle),
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(cancelFlashbotsBundlePayload); err != nil {
		return nil, fmt.Errorf("failed to marshal err: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: err: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	flashbotsSignature, err := generateRandomFlashbotsSignature(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to create random flashbots signature: %v", err)
	}
	req.Header.Set(flashbotAuthHeader, flashbotsSignature)

	return &bundleRequest{
		bundleHash:  bundle.BundleHash,
		blockNumber: bundle.BlockNumber,
		method:      string(jsonrpc.RPCEthCancelBundle),
		request:     req,
	}, nil
}

// generateRandomFlashbotsSignature Generates a signature while submitting bundles to flashbots
func generateRandomFlashbotsSignature(payload []byte) (string, error) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return "", err
	}

	hashedBody := crypto.Keccak256Hash(payload).Hex()

	sig, err := crypto.Sign(accounts.TextHash([]byte(hashedBody)), privateKey)
	if err != nil {
		return "", err
	}
	return crypto.PubkeyToAddress(privateKey.PublicKey).Hex() + ":" + hexutil.Encode(sig), nil
}
