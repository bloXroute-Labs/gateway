package bxmessage

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestIntentSolution_Unpack(t *testing.T) {
	r := genIntentsRequest()

	solution := IntentSolution{
		Header: Header{
			msgType: IntentSolutionType,
		},
		ID:            uuid.New().String(),
		SolverAddress: r.SolverAddress,
		IntentID:      uuid.New().String(),
		Solution:      []byte("this is test :("),
		Hash:          r.Hash,
		Signature:     r.Signature,
		Timestamp:     time.Now(),
	}

	bytes, err := solution.Pack(CurrentProtocol)
	require.NoError(t, err)

	solution2 := IntentSolution{}
	err = solution2.Unpack(bytes, CurrentProtocol)
	require.NoError(t, err)

	require.Equal(t, solution.Header.msgType, solution2.Header.msgType)
	require.Equal(t, solution.ID, solution2.ID)
	require.Equal(t, solution.SolverAddress, solution2.SolverAddress)
	require.Equal(t, solution.IntentID, solution2.IntentID)
	require.Equal(t, solution.Solution, solution2.Solution)
	require.Equal(t, solution.Hash, solution2.Hash)
	require.Equal(t, solution.Signature, solution2.Signature)
	require.Equal(t, solution.Timestamp.UnixMilli(), solution2.Timestamp.UnixMilli())
}
