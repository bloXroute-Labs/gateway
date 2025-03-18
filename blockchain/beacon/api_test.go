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

	"github.com/jarcoal/httpmock"
	ssz "github.com/prysmaticlabs/fastssz"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/types"
	httputils "github.com/bloXroute-Labs/gateway/v2/utils/http"
)

var (
	ctx                      = context.Background()
	config                   = &network.EthConfig{}
	bridge                   = &blockchain.BxBridge{}
	url                      = "localhost:4000"
	blockID                  = "0x025ad52b0739ebbfcbe967b880d426b406764efff1515b555853c43bff81378a"
	denebBlockContentsData   []byte
	denebBlockData           []byte
	electraBlockContentsData []byte
	validBlock               interfaces.ReadOnlySignedBeaconBlock
)

func init() {
	var err error

	denebBlockContentsData, err = readTestBlock("test_data/contents_deneb_block.ssz")
	if err != nil {
		panic(err)
	}

	denebBlockData, err = readTestBlock("test_data/deneb_block.ssz")
	if err != nil {
		panic(err)
	}

	electraBlockContentsData, err = readTestBlock("test_data/contents_electra_block.ssz")
	if err != nil {
		panic(err)
	}

	validBlock = newBlock(201)
}

func TestNewAPIClient(t *testing.T) {
	// Initialize httpmock
	httpClient := httputils.Client(nil)
	httpmock.ActivateNonDefault(httpClient)
	defer httpmock.DeactivateAndReset()

	client := NewAPIClient(ctx, httpClient, config, bridge, url, types.NodeEndpoint{}, NewAPISharedSync())

	time.Sleep(time.Second)
	assert.Equal(t, ctx, client.ctx)
	assert.Equal(t, url, client.URL)
	assert.Equal(t, bridge, client.bridge)
	assert.Equal(t, config, client.config)
}

func TestAPIClient_requestBlock(t *testing.T) {
	// Initialize httpmock
	httpClient := httputils.Client(nil)
	httpmock.ActivateNonDefault(httpClient)
	defer httpmock.DeactivateAndReset()
	// Mock the clientVersion function
	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf("http://%s/eth/v1/node/version", url),
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewStringResponse(http.StatusOK, `{"data":{"version":"mocked-version"}}`), nil
		},
	)

	client := NewAPIClient(ctx, httpClient, config, bridge, url, types.NodeEndpoint{}, NewAPISharedSync())
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
					Body:       io.NopCloser(bytes.NewReader(denebBlockData)),
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
	httpClient := httputils.Client(nil)
	httpmock.ActivateNonDefault(httpClient)
	defer httpmock.DeactivateAndReset()
	// Mock the getBeaconNodeClientVersion function
	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf("http://%s/eth/v1/node/version", url),
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewStringResponse(http.StatusOK, `{"data":{"version":"mocked-version"}}`), nil
		},
	)

	client := NewAPIClient(ctx, httpClient, config, bridge, url, types.NodeEndpoint{}, NewAPISharedSync())
	respHeaders := http.Header{}
	respHeaders.Add("Eth-Consensus-Version", "deneb")
	httpmock.RegisterResponder("GET", "http://"+client.URL+"/eth/v2/beacon/blocks/"+blockID,
		httpmock.ResponderFromResponse(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(denebBlockData)),
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
	httpClient := httputils.Client(nil)
	httpmock.ActivateNonDefault(httpClient)
	defer httpmock.DeactivateAndReset()

	// Mock the getBeaconNodeClientVersion function
	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf("http://%s/eth/v1/node/version", url),
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewStringResponse(http.StatusOK, `{"data":{"version":"mocked-version"}}`), nil
		},
	)

	// Initialize Beacon API client
	client := NewAPIClient(ctx, httpClient, config, bridge, url, types.NodeEndpoint{}, NewAPISharedSync())

	version := "deneb"

	tests := []struct {
		name        string
		respBodyRaw []byte
		v           string
		wantErr     bool
	}{
		{
			name:        "Test case 1: valid body",
			respBodyRaw: denebBlockData,
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
	httpClient := httputils.Client(nil)
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
	client := NewAPIClient(ctx, httpClient, config, bridge, url, types.NodeEndpoint{}, NewAPISharedSync())
	go client.Start()

	test.WaitUntilTrueOrFail(t, client.initialized.Load)

	testCases := []struct {
		name        string
		block       func() (ssz.Marshaler, []byte)
		expectError bool
	}{
		{
			name:        "successful broadcast deneb",
			expectError: false,
			block: func() (ssz.Marshaler, []byte) {
				mockRawBlock := denebBlockContentsData
				denebBlock := &eth.SignedBeaconBlockContentsDeneb{}
				err := denebBlock.UnmarshalSSZ(denebBlockContentsData)
				require.NoError(t, err)

				return denebBlock, mockRawBlock
			},
		},
		{
			name:        "successful broadcast electra",
			expectError: false,
			block: func() (ssz.Marshaler, []byte) {
				mockRawBlock := electraBlockContentsData
				electraBlock := &eth.SignedBeaconBlockContentsElectra{}
				err := electraBlock.UnmarshalSSZ(electraBlockContentsData)
				require.NoError(t, err)

				return electraBlock, mockRawBlock
			},
		},
		{
			name:        "failed broadcast deneb",
			expectError: true,
			block: func() (ssz.Marshaler, []byte) {
				mockRawBlock := denebBlockContentsData
				denebBlock := &eth.SignedBeaconBlockContentsDeneb{}
				err := denebBlock.UnmarshalSSZ(denebBlockContentsData)
				require.NoError(t, err)

				denebBlock.Block.Block.Slot++

				return denebBlock, mockRawBlock
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			blockToBroadcast, mockRawBlock := tt.block()

			if !tt.expectError {
				httpmock.Reset()
				httpmock.RegisterResponder(http.MethodPost, fmt.Sprintf("http://%s/eth/v1/beacon/blocks", url),
					func(req *http.Request) (*http.Response, error) {
						if req.Header.Get("Content-Type") != "application/octet-stream" {
							t.Errorf("Expected content type to be 'application/octet-stream', but got '%s'", req.Header.Get("Content-Type"))
						}

						reqBody, _ := io.ReadAll(req.Body)

						if !bytes.Equal(reqBody, mockRawBlock) {
							t.Errorf("Expected request body:\n%s\nBut got:\n%s", mockRawBlock, reqBody)
						}

						return httpmock.NewStringResponse(http.StatusOK, ""), nil
					},
				)
			} else {
				httpmock.Reset()
				httpmock.RegisterResponder(http.MethodPost, fmt.Sprintf("http://%s/eth/v1/beacon/blocks", url),
					func(*http.Request) (*http.Response, error) {
						return httpmock.NewStringResponse(http.StatusServiceUnavailable, `{"code":503,"message":"Service Unavailable"}`), nil
					},
				)
			}

			err := client.BroadcastBlock(blockToBroadcast, version.String(version.Deneb))
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}

	/*// Test Case 1: Successful broadcast ssz
	mockRawBlock := denebBlockContentsData
	denebBlock := &eth.SignedBeaconBlockContentsDeneb{}
	err := denebBlock.UnmarshalSSZ(denebBlockContentsData)
	require.NoError(t, err)

	err = client.BroadcastBlock(denebBlock, version.String(version.Deneb))
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

	denebBlock.Block.Block.Slot++
	err = client.BroadcastBlock(denebBlock, version.String(version.Deneb))
	if err == nil {
		t.Error("Expected an error, but got none")
	}*/
}

func TestAPIClient_UnmarshallBlobFromRequest(t *testing.T) {
	blobFile := "test_data/blob_without_offset.ssz"
	blobData, err := os.ReadFile(blobFile)
	require.NoError(t, err)

	blob := &eth.BlobSidecars{}

	err = blob.UnmarshalSSZ(blobData)
	require.Error(t, err)

	_, err = lightHouseBlobDecoder{}.decodeBlobSidecar(blobData)
	require.NoError(t, err)
}

func newBlock(slot uint64) interfaces.ReadOnlySignedBeaconBlock {
	hashLen := 32

	b, err := blocks.NewSignedBeaconBlock(&eth.SignedBeaconBlock{
		Block: &eth.BeaconBlock{
			Slot:          primitives.Slot(slot),
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
	})
	if err != nil {
		panic(err)
	}

	return b
}

func readTestBlock(name string) ([]byte, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return io.ReadAll(file)
}
