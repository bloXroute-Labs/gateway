package bxmessage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntentsSubscription_Unpack(t *testing.T) {
	r := genIntentsRequest()

	sub := IntentsSubscription{
		Header: Header{
			msgType: IntentsSubscriptionType,
		},
		SolverAddress: r.SolverAddress,
		Hash:          r.Hash,
		Signature:     r.Signature,
	}

	bytes, err := sub.Pack(CurrentProtocol)
	require.NoError(t, err)

	sub2 := IntentsSubscription{}
	err = sub2.Unpack(bytes, CurrentProtocol)
	require.NoError(t, err)

	require.Equal(t, sub.Header.msgType, sub2.Header.msgType)
	require.Equal(t, sub.SolverAddress, sub2.SolverAddress)
	require.Equal(t, sub.Hash, sub2.Hash)
	require.Equal(t, sub.Signature, sub2.Signature)
}
