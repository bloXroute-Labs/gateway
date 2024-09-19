package bxmessage

import (
	"crypto/ecdsa"
	"testing"

	"github.com/bloXroute-Labs/gateway/v2/utils/intent"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuotes_Unpack(t *testing.T) {
	quote := createTestQuote(t)
	bytes, err := quote.Pack(CurrentProtocol)
	require.NoError(t, err)

	quoteRes := Quote{}
	err = quoteRes.Unpack(bytes, CurrentProtocol)
	require.NoError(t, err)
	assert.Equal(t, quoteRes, quote)
}

func createTestQuote(t *testing.T) Quote {
	solverPrivKey, err := crypto.GenerateKey()
	if err != nil {
		panic(err)
	}
	dappPrivKey, err := crypto.GenerateKey()
	if err != nil {
		panic(err)
	}

	dappAddress := createAddressFromPrivateKey(dappPrivKey)
	solverAddress := createAddressFromPrivateKey(solverPrivKey)
	quoteByte := []byte("this is test :(")

	id, err := intent.GenerateQuoteID(dappAddress.String(), quoteByte)
	require.NoError(t, err)
	quote := Quote{
		Header:        Header{msgType: QuotesType},
		ID:            id,
		DappAddress:   dappAddress.String(),
		SolverAddress: solverAddress.String(),
		Quote:         quoteByte,
	}

	combined := append([]byte(quote.SolverAddress), []byte(quote.DappAddress)...)
	quote.Hash = crypto.Keccak256Hash(combined).Bytes()

	sig, err := crypto.Sign(quote.Hash, solverPrivKey)
	if err != nil {
		panic(err)
	}
	quote.Signature = sig

	return quote
}

// createAddressFromPrivateKey creates an address from a given private key
func createAddressFromPrivateKey(privKey *ecdsa.PrivateKey) common.Address {
	pubKey := privKey.PublicKey
	signerAddress := crypto.PubkeyToAddress(pubKey)
	return signerAddress
}
