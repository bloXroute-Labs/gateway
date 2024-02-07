package bxmessage

import (
	"testing"
	"time"

	gateway "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestIntent_Unpack(t *testing.T) {
	r := genIntentsRequest()

	intent := Intent{
		Header: Header{
			msgType: "intent",
		},
		ID:            uuid.New().String(),
		DAppAddress:   r.SolverAddress,
		SenderAddress: "0xb794F5eA0ba39494cE839613fffBA74279579268",
		Hash:          r.Hash,
		Signature:     r.Signature,
		Timestamp:     time.Now(),
		Intent:        []byte("testing intent"),
	}

	bytes, err := intent.Pack(CurrentProtocol)
	require.NoError(t, err)

	intent2 := Intent{}
	err = intent2.Unpack(bytes, CurrentProtocol)
	require.NoError(t, err)

	require.Equal(t, intent.Header.msgType, intent2.Header.msgType)
	require.Equal(t, intent.ID, intent2.ID)
	require.Equal(t, intent.DAppAddress, intent2.DAppAddress)
	require.Equal(t, intent.SenderAddress, intent2.SenderAddress)
	require.Equal(t, intent.Hash, intent2.Hash)
	require.Equal(t, intent.Signature, intent2.Signature)
	require.Equal(t, intent.Timestamp.UnixMilli(), intent2.Timestamp.UnixMilli())
	require.Equal(t, intent.Intent, intent2.Intent)

	// Test unpacking with old protocol, should substitute DAppAddress with SenderAddress
	intent3 := Intent{}
	err = intent3.Unpack(bytes, IntentsProtocol)
	require.NoError(t, err)
	require.Equal(t, intent.DAppAddress, intent3.SenderAddress)
}

func genIntentsRequest() *gateway.IntentsRequest {
	// Generate an ECDSA key pair using secp256k1 curve
	privKey, err := crypto.GenerateKey()
	if err != nil {
		panic(err)
	}

	// Extract the public key
	pubKey := privKey.PublicKey
	signerAddress := crypto.PubkeyToAddress(pubKey)
	hash := crypto.Keccak256Hash([]byte(signerAddress.String())).Bytes()
	sig, err := crypto.Sign(hash, privKey)
	if err != nil {
		panic(err)
	}

	return &gateway.IntentsRequest{
		SolverAddress: signerAddress.String(),
		Hash:          hash,
		Signature:     sig,
	}
}
