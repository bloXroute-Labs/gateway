//go:build integration
// +build integration

package connections

import (
	"fmt"
	"os"
	"testing"

	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Requires setting these environment variables:
// REGISTRATION_ONLY_CERT_PATH
// REGISTRATION_ONLY_KEY_PATH
// API_CERT_PATH (for testnet)
// API_KEY_PATH (for testnet)

func getSSLCerts(t *testing.T) utils.SSLCerts {
	apiCertPath := os.Getenv("API_CERT_PATH")
	apiKeyPath := os.Getenv("API_KEY_PATH")
	registrationOnlyCertPath := os.Getenv("REGISTRATION_ONLY_CERT_PATH")
	registrationOnlyKeyPath := os.Getenv("REGISTRATION_ONLY_KEY_PATH")

	if apiCertPath == "" || apiKeyPath == "" || registrationOnlyCertPath == "" || registrationOnlyKeyPath == "" {
		t.FailNow()
	}

	sslCerts := utils.NewSSLCertsFromFiles(apiCertPath, apiKeyPath, registrationOnlyCertPath, registrationOnlyKeyPath)

	return sslCerts
}

func tearDown() {
	os.Remove(blockchainNetworkCacheFileName)
	os.Remove(nodeModelCacheFileName)
}

func TestInitGateway_GetsCorrectBlockchainNetworkFromProtocolAndNetwork(t *testing.T) {
	testTable := []struct {
		protocol                string
		network                 string
		networkNumber           types.NetworkNum
		genesisHash             string
		blockInterval           int64
		maxBlockSizeBytes       int64
		maxTxSizeBytes          int64
		blockConfirmationsCount int64
		txSyncIntervalS         float64
	}{
		{"Ethereum", "Mainnet", 5, "d4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3", 15, 1048576, 1048576, 12, 1800},
		{"Ethereum", "Polygon-Mainnet", 36, "a9c28ce2141b56c474f1dc504bee9b01eb1bd7d1a507580d5519d4437a97de1b", 600, 2097152, 100000, 6, 1800},
		{"Ethereum", "BSC-Mainnet", 10, "0d21840abff46b96c84b2ac9e10e4f5cdaeb5693cb665db62a2f3b02d2d57b5b", 15, 1048576, 1048576, 12, 1800},
		{"BitcoinCash", "Mainnet", 3, "", 600, 33554432, 1048576, 6, 1800},
	}

	for _, testCase := range testTable {
		t.Run(fmt.Sprint(testCase), func(t *testing.T) {
			sslCerts := getSSLCerts(t)
			sdnURL := "https://bdn-api.testnet.blxrbdn.com"

			s, err := NewSDNHTTP(&sslCerts, sdnURL, sdnmessage.NodeModel{NodeType: "EXTERNAL_GATEWAY"}, "")
			require.NoError(t, err)

			err = s.InitGateway(testCase.protocol, testCase.network)
			assert.Nil(t, err)

			// check Network Number correct
			bcn, found := (*s.Networks())[testCase.networkNumber]
			if !found || bcn == nil {
				t.FailNow()
			}
			assert.Len(t, *s.Networks(), 1)

			// check Network correct
			assert.Equal(t, testCase.protocol, bcn.Protocol)
			assert.Equal(t, testCase.network, bcn.Network)
			assert.Equal(t, testCase.networkNumber, bcn.NetworkNum)
			if testCase.protocol == "Ethereum" {
				assert.Equal(t, testCase.genesisHash, bcn.DefaultAttributes.GenesisHash)
			}
			assert.Equal(t, testCase.blockInterval, bcn.BlockInterval)
			assert.Equal(t, testCase.maxBlockSizeBytes, bcn.MaxBlockSizeBytes)
			assert.Equal(t, testCase.maxTxSizeBytes, bcn.MaxTxSizeBytes)
			assert.Equal(t, testCase.blockConfirmationsCount, bcn.BlockConfirmationsCount)
			assert.Equal(t, testCase.txSyncIntervalS, bcn.TxSyncIntervalS)

			tearDown()
		})
	}
}

func TestInitGateway_ReturnsErrorIfIncorrectNetworkOrProtocol(t *testing.T) {
	testTable := []struct {
		protocol string
		network  string
		errMsg   string
	}{
		{"Ethereumm", "Mainnet", "got error from http request: doing POST on https://bdn-api.testnet.blxrbdn.com/nodes recv and error 400 Bad Request, payload {INTERNAL_GATEWAY 0 0  false false Mainnet Ethereumm"},
		{"Ethereum", "Minnet", "got error from http request: doing POST on https://bdn-api.testnet.blxrbdn.com/nodes recv and error 400 Bad Request, payload {INTERNAL_GATEWAY 0 0  false false Minnet Ethereum"},
		{"Ethereumm", "Minnet", "got error from http request: doing POST on https://bdn-api.testnet.blxrbdn.com/nodes recv and error 400 Bad Request, payload {INTERNAL_GATEWAY 0 0  false false Minnet Ethereumm"},
		{"Ethereumm", "BSCMainnet", "got error from http request: doing POST on https://bdn-api.testnet.blxrbdn.com/nodes recv and error 400 Bad Request, payload {INTERNAL_GATEWAY 0 0  false false BSCMainnet Ethereumm"},
	}

	for _, testCase := range testTable {
		t.Run(fmt.Sprint(testCase), func(t *testing.T) {
			sslCerts := getSSLCerts(t)
			sdnURL := "https://bdn-api.testnet.blxrbdn.com"

			s, err := NewSDNHTTP(&sslCerts, sdnURL, sdnmessage.NodeModel{NodeType: "EXTERNAL_GATEWAY"}, "")
			require.NoError(t, err)

			err = s.InitGateway(testCase.protocol, testCase.network)

			if err == nil {
				t.FailNow()
			}
			assert.Contains(t, err.Error(), testCase.errMsg)
			assert.Contains(t, err.Error(), "and can't load cache file nodemodel.json: open nodemodel.json: no such file or directory")
		})
	}
}
