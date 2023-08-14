package bundle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/sourcegraph/jsonrpc2"
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

type request struct {
	bundleHash  string
	blockNumber string
	method      string
	request     *http.Request
}

// String returns a string representation of the request
func (r *request) String() string {
	return fmt.Sprintf("bundleHash: %v, blockNumber: %v, endpoint: %s, method: %v", r.bundleHash, r.blockNumber, r.request.URL, r.method)
}

// Dispatcher is responsible for dispatching MEV bundles to MEV builders
type Dispatcher struct {
	client              *http.Client
	builders            map[string]*Builder
	mevMaxProfitBuilder bool
	processMegaBundle   bool
}

// NewDispatcher creates a new NewDispatcher
func NewDispatcher(builders map[string]*Builder, mevMaxProfitBuilder bool, processMegaBundle bool) *Dispatcher {
	for name, builder := range builders {
		builder.Name = name
	}

	return &Dispatcher{
		client: &http.Client{
			Transport: &http.Transport{
				MaxConnsPerHost:     100,
				MaxIdleConnsPerHost: 100,
				MaxIdleConns:        100,
				IdleConnTimeout:     0 * time.Second,
			},
			Timeout: 60 * time.Second,
		},
		builders:            builders,
		mevMaxProfitBuilder: mevMaxProfitBuilder,
		processMegaBundle:   processMegaBundle,
	}
}

// Dispatch dispatches the MEV bundle to the MEV builders
func (d *Dispatcher) Dispatch(bundle *bxmessage.MEVBundle) error {
	if len(d.builders) == 0 {
		log.Warnf("received mevBundle message, but mev-builders-file-path is empty. Message %v from %v in network %v", bundle.BundleHash, bundle.SourceID(), bundle.GetNetworkNum())
		return nil
	}

	if !d.mevMaxProfitBuilder && bundle.Frontrunning {
		log.Warnf("MEV bundle %v is frontrunning, but max profit builder is not enabled. Skipping.", bundle.BundleHash)
		return nil
	}

	if bundle.Method == string(jsonrpc.RPCEthSendMegaBundle) && !d.processMegaBundle {
		log.Warnf("received megaBundle message. Message %v from %v in network %v", bundle.BundleHash, bundle.SourceID(), bundle.GetNetworkNum())
		return nil
	}

	json, err := d.bundleJSON(bundle)
	if err != nil {
		return fmt.Errorf("failed to create new mevBundle http request for bundleHash: %v, err: %v", bundle.BundleHash, err)
	}

	for _, req := range d.makeRequests(bundle, json) {
		go func(req *request) {
			resp, err := d.client.Do(req.request)
			if err != nil {
				log.Errorf("failed to forward mevBundle (%s, json: '%s'), err: %v", req, json, err)
				return
			}
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Errorf("failed to read mevBundle (%s, json: '%s') response, err: %v", req, json, err)
				return
			}

			log.Tracef("sent mevBundle (%s, json: '%s') got response: %v, status code: %v", req, json, string(respBody), resp.StatusCode)
		}(req)
	}

	return nil
}

func (d *Dispatcher) bundleJSON(bundle *bxmessage.MEVBundle) ([]byte, error) {
	params := []jsonrpc.RPCSendBundle{
		{
			Txs:               bundle.Transactions,
			UUID:              bundle.UUID,
			BlockNumber:       bundle.BlockNumber,
			MinTimestamp:      bundle.MinTimestamp,
			MaxTimestamp:      bundle.MaxTimestamp,
			RevertingTxHashes: bundle.RevertingHashes,
			BundlePrice:       bundle.BundlePrice,
			EnforcePayout:     bundle.EnforcePayout,
		},
	}

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create new mevBundle http request for bundleHash: %v, err: %v", bundle.BundleHash, err)
	}

	mevBundle := jsonrpc2.Request{
		Params: (*json.RawMessage)(&paramsBytes),
		Method: string(jsonrpc.RPCEthSendBundle),
	}

	json, err := json.Marshal(mevBundle)
	if err != nil {
		return nil, fmt.Errorf("failed to create new mevBundle http request bundleHash: %v, err: %v", bundle.BundleHash, err)
	}

	return json, nil
}

func (d *Dispatcher) makeRequests(bundle *bxmessage.MEVBundle, json []byte) []*request {
	requests := make([]*request, 0, len(bundle.MEVBuilders))
	for builderName := range bundle.MEVBuilders {
		builder := d.getBuilder(builderName)
		if builder == nil {
			log.Errorf("failed to find mev builder %v in config, bundleHash: %v", builderName, bundle.BundleHash)
			continue
		}

		for _, endpoint := range builder.Endpoints {
			if builder.Name == bxgateway.FlashbotsBuilderName && len(bundle.Transactions) == 0 {
				req, err := d.cancelRequest(endpoint, bundle)
				if err != nil {
					log.Errorf("failed to create cancel request for mev builder %v, bundleHash: %v, err: %v", endpoint, bundle.BundleHash, err)
					continue
				}

				requests = append(requests, &request{
					request:     req,
					bundleHash:  bundle.BundleHash,
					blockNumber: bundle.BlockNumber,
					method:      string(jsonrpc.RPCEthCancelBundle),
				})
				continue
			}

			req, err := d.bundleRequest(endpoint, builder, bundle, json)
			if err != nil {
				log.Errorf("failed to create send http request for mev builder %v, bundleHash: %v, err: %v", endpoint, bundle.BundleHash, err)
				continue
			}

			requests = append(requests, &request{
				request:     req,
				bundleHash:  bundle.BundleHash,
				blockNumber: bundle.BlockNumber,
				method:      string(jsonrpc.RPCEthSendBundle),
			})
		}
	}

	return requests
}

func (d *Dispatcher) getBuilder(builder string) *Builder {
	if len(d.builders) > 0 {
		return d.builders[builder]

	}

	return nil
}

func (d *Dispatcher) bundleRequest(endpoint string, builder *Builder, bundle *bxmessage.MEVBundle, json []byte) (*http.Request, error) {
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

	return req, nil
}

func (d *Dispatcher) cancelRequest(endpoint string, bundle *bxmessage.MEVBundle) (*http.Request, error) {
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

	return req, nil
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
