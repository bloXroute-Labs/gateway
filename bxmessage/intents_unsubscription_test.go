package bxmessage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntentsUnsubscription_Unpack(t *testing.T) {
	r := genIntentsRequest()

	sub := IntentsUnsubscription{
		Header: Header{
			msgType: IntentsUnsubscriptionType,
		},
		SolverAddress: r.SolverAddress,
	}

	bytes, err := sub.Pack(CurrentProtocol)
	require.NoError(t, err)

	sub2 := IntentsUnsubscription{}
	err = sub2.Unpack(bytes, CurrentProtocol)
	require.NoError(t, err)

	require.Equal(t, sub.Header.msgType, sub2.Header.msgType)
	require.Equal(t, sub.SolverAddress, sub2.SolverAddress)
}
