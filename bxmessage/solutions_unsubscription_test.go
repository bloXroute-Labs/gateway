package bxmessage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSolutionsUnsubscription_Unpack(t *testing.T) {
	r := genIntentsRequest()

	sub := SolutionsUnsubscription{
		Header: Header{
			msgType: SolutionsUnsubscriptionType,
		},
		DAppAddress: r.SolverAddress,
	}

	bytes, err := sub.Pack(CurrentProtocol)
	require.NoError(t, err)

	sub2 := SolutionsUnsubscription{}
	err = sub2.Unpack(bytes, CurrentProtocol)
	require.NoError(t, err)

	require.Equal(t, sub.Header.msgType, sub2.Header.msgType)
	require.Equal(t, sub.DAppAddress, sub2.DAppAddress)
}
