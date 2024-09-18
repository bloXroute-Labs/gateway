package bxmessage

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func TestQuotesUnsubscription_Unpack(t *testing.T) {
	privKey, err := crypto.GenerateKey()
	if err != nil {
		panic(err)
	}

	pubKey := privKey.PublicKey
	signerAddress := crypto.PubkeyToAddress(pubKey)

	sub := QuotesUnsubscription{
		Header: Header{
			msgType: QuotesUnsubscriptionType,
		},
		DAppAddress: signerAddress.String(),
	}

	bytes, err := sub.Pack(CurrentProtocol)
	require.NoError(t, err)

	sub2 := QuotesUnsubscription{}
	err = sub2.Unpack(bytes, CurrentProtocol)
	require.NoError(t, err)

	require.Equal(t, sub.Header.msgType, sub2.Header.msgType)
	require.Equal(t, sub.DAppAddress, sub2.DAppAddress)
}
