package beacon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/test"
	httpclient "github.com/bloXroute-Labs/gateway/v2/utils/httpclient"
	httpmock "github.com/jarcoal/httpmock"
	fieldparams "github.com/prysmaticlabs/prysm/v4/config/fieldparams"
	blocks "github.com/prysmaticlabs/prysm/v4/consensus-types/blocks"
	interfaces "github.com/prysmaticlabs/prysm/v4/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v4/encoding/bytesutil"
	eth "github.com/prysmaticlabs/prysm/v4/proto/prysm/v1alpha1"
	ethpb "github.com/prysmaticlabs/prysm/v4/proto/prysm/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	ctx               = context.Background()
	config            = &network.EthConfig{}
	bridge            = &blockchain.BxBridge{}
	url               = "localhost:4000"
	blockchainNetwork = "Test"
	blockID           = "0x025ad52b0739ebbfcbe967b880d426b406764efff1515b555853c43bff81378a"
	file              *os.File
	blockData         []byte
	err               error
	validBlock        interfaces.ReadOnlySignedBeaconBlock
)

func init() {
	file, err = os.Open("test_data/deneb_block.ssz")
	if err == nil {
		defer file.Close()
		blockData, err = io.ReadAll(file)
	}

	hashLen := 32
	blk := &eth.SignedBeaconBlock{Block: &eth.BeaconBlock{
		Slot:          201,
		ProposerIndex: 2,
		ParentRoot:    bytesutil.PadTo([]byte("parent root"), hashLen),
		StateRoot:     bytesutil.PadTo([]byte("state root"), hashLen),
		Body: &eth.BeaconBlockBody{
			Eth1Data: &eth.Eth1Data{
				BlockHash:    bytesutil.PadTo([]byte("block hash"), hashLen),
				DepositRoot:  bytesutil.PadTo([]byte("deposit root"), hashLen),
				DepositCount: 1,
			},
			RandaoReveal:      bytesutil.PadTo([]byte("randao"), fieldparams.BLSSignatureLength),
			Graffiti:          bytesutil.PadTo([]byte("teehee"), hashLen),
			ProposerSlashings: []*eth.ProposerSlashing{},
			AttesterSlashings: []*eth.AttesterSlashing{},
			Attestations:      []*eth.Attestation{},
			Deposits:          []*eth.Deposit{},
			VoluntaryExits:    []*eth.SignedVoluntaryExit{},
		},
	},
		Signature: bytesutil.PadTo([]byte("signature"), fieldparams.BLSSignatureLength),
	}

	validBlock, _ = blocks.NewSignedBeaconBlock(blk)

}

func TestNewAPIClient(t *testing.T) {
	// Initialize httpmock
	httpClient := httpclient.Client(nil)
	httpmock.ActivateNonDefault(httpClient)
	defer httpmock.DeactivateAndReset()

	client, err := NewAPIClient(ctx, httpClient, config, bridge, url, blockchainNetwork)

	time.Sleep(time.Second)
	assert.NoError(t, err)
	assert.Equal(t, ctx, client.ctx)
	assert.Equal(t, url, client.URL)
	assert.Equal(t, bridge, client.bridge)
	assert.Equal(t, config, client.config)
}
func TestAPIClient_requestBlock(t *testing.T) {
	// Initialize httpmock
	httpClient := httpclient.Client(nil)
	httpmock.ActivateNonDefault(httpClient)
	defer httpmock.DeactivateAndReset()
	// Mock the clientVersion function
	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf("http://%s/eth/v1/node/version", url),
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewStringResponse(http.StatusOK, `{"data":{"version":"mocked-version"}}`), nil
		},
	)

	client, err := NewAPIClient(ctx, httpClient, config, bridge, url, blockchainNetwork)
	assert.NoError(t, err)
	tests := []struct {
		name        string
		version     string
		expectBlock bool
		expectError bool
	}{
		{"Success", "deneb", true, false},
		{"FailedToUnmarshal", "phase0", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respHeaders := http.Header{}
			respHeaders.Add("Eth-Consensus-Version", tt.version)

			httpmock.RegisterResponder("GET", "http://"+client.URL+"/eth/v2/beacon/blocks/"+blockID,
				httpmock.ResponderFromResponse(&http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(blockData)),
					Header:     respHeaders,
				}),
			)

			block, err := client.requestBlock(blockID)

			if tt.expectBlock {
				assert.NotNil(t, block)
			} else {
				assert.Nil(t, block)
			}

			if tt.expectError {
				assert.NotNil(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAPIClient_hashOfBlock(t *testing.T) {
	// Initialize httpmock
	httpClient := httpclient.Client(nil)
	httpmock.ActivateNonDefault(httpClient)
	defer httpmock.DeactivateAndReset()
	// Mock the getBeaconNodeClientVersion function
	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf("http://%s/eth/v1/node/version", url),
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewStringResponse(http.StatusOK, `{"data":{"version":"mocked-version"}}`), nil
		},
	)

	client, err := NewAPIClient(ctx, httpClient, config, bridge, url, blockchainNetwork)
	assert.NoError(t, err)
	respHeaders := http.Header{}
	respHeaders.Add("Eth-Consensus-Version", "deneb")
	httpmock.RegisterResponder("GET", "http://"+client.URL+"/eth/v2/beacon/blocks/"+blockID,
		httpmock.ResponderFromResponse(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(blockData)),
			Header:     respHeaders,
		}),
	)

	blockFromFile, err := client.requestBlock(blockID)
	if err != nil {
		t.Fatalf("Failed to get block: %v", err)
	}

	tests := []struct {
		name         string
		block        interfaces.ReadOnlySignedBeaconBlock
		wantFuncErr  bool
		expectedHash string
		wantHashErr  bool
	}{
		{
			name:         "Test case 1: Valid block",
			block:        blockFromFile,
			expectedHash: blockID,
			wantFuncErr:  false,
			wantHashErr:  false,
		},
		{
			name:         "Test case 2: Invalid block Hash",
			block:        validBlock,
			expectedHash: "0x66a1a8fca5c5bda93e99f832fc691b4b24ca672666c00f19e3fbd51a00aa1f9f",
			wantFuncErr:  false,
			wantHashErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.hashOfBlock(NewWrappedReadOnlySignedBeaconBlock(tt.block))
			if (err != nil) != tt.wantFuncErr {
				t.Errorf("APIClient.hashOfBlock() error = %v, wantErr %v", err, tt.wantFuncErr)
				return
			}

			if (got != tt.expectedHash) != tt.wantHashErr {
				t.Errorf("APIClient.hashOfBlock() = %v, want %v", got, tt.expectedHash)
			}
		})
	}
}

func TestAPIClient_processResponse(t *testing.T) {
	// Initialize httpmock
	httpClient := httpclient.Client(nil)
	httpmock.ActivateNonDefault(httpClient)
	defer httpmock.DeactivateAndReset()

	// Mock the getBeaconNodeClientVersion function
	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf("http://%s/eth/v1/node/version", url),
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewStringResponse(http.StatusOK, `{"data":{"version":"mocked-version"}}`), nil
		},
	)

	// Initialize Beacon API client
	client, err := NewAPIClient(ctx, httpClient, config, bridge, url, blockchainNetwork)

	assert.NoError(t, err)

	version := "deneb"

	tests := []struct {
		name        string
		respBodyRaw []byte
		v           string
		wantErr     bool
	}{
		{
			name:        "Test case 1: valid body",
			respBodyRaw: blockData,
			v:           version,
			wantErr:     false,
		},
		{
			name:        "Test case 2: ivalid body",
			respBodyRaw: []byte("shit"),
			v:           version,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.processResponse(tt.respBodyRaw, tt.v, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("APIClient.processResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestAPIClient_broadcastBlock(t *testing.T) {
	// Initialize httpmock
	httpClient := httpclient.Client(nil)
	httpmock.ActivateNonDefault(httpClient)
	defer httpmock.DeactivateAndReset()

	// Mock the getBeaconNodeClientVersion function
	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf("http://%s/eth/v1/node/version", url),
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewStringResponse(http.StatusOK, `{"data":{"version":"mocked-version"}}`), nil
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Beacon API client
	client, err := NewAPIClient(ctx, httpClient, config, bridge, url, blockchainNetwork)
	assert.NoError(t, err)
	client.Start()

	test.WaitUntilTrueOrFail(t, client.initialized.Load)

	// Test Case 1: Successful broadcast ssz
	mockRawBlock := blockData
	var denebBlock = &ethpb.SignedBeaconBlockDeneb{}
	denebBlock.UnmarshalSSZ(blockData)
	block, err := blocks.NewSignedBeaconBlock(denebBlock)
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}

	httpmock.Reset()
	httpmock.RegisterResponder(http.MethodPost, fmt.Sprintf("http://%s/eth/v1/beacon/blocks", url),
		func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("Content-Type") != "application/octet-stream" {
				t.Errorf("Expected content type to be 'application/octet-stream', but got '%s'", req.Header.Get("Content-Type"))
			}

			reqBody, _ := io.ReadAll(req.Body)

			denebBlock := &eth.SignedBeaconBlockContentsDeneb{}
			err := denebBlock.UnmarshalSSZ(reqBody)
			require.NoError(t, err)

			// to check with the original mocked ssz block
			marshalledBlock, err := denebBlock.Block.MarshalSSZ()
			require.NoError(t, err)

			if !bytes.Equal(marshalledBlock, mockRawBlock) {
				t.Errorf("Expected request body:\n%s\nBut got:\n%s", mockRawBlock, reqBody)
			}

			return httpmock.NewStringResponse(http.StatusOK, ""), nil
		},
	)

	err = client.BroadcastBlock(block)
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
	}

	// Test Case 2: Failed broadcast
	httpmock.Reset()
	httpmock.RegisterResponder(http.MethodPost, fmt.Sprintf("http://%s/eth/v1/beacon/blocks", url),
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewStringResponse(http.StatusServiceUnavailable, `{"code":503,"message":"Service Unavailable"}`), nil
		},
	)

	err = client.BroadcastBlock(block)
	if err == nil {
		t.Error("Expected an error, but got none")
	}
}
