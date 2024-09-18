package bxmessage

import (
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/utils/intent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntentSolutions_Unpack(t *testing.T) {
	solutions := make([]IntentSolution, 5)

	for i := 0; i < 5; i++ {
		solutions[i] = createTestIntentSolution(t)
	}

	intentSolutions := NewIntentSolutions(solutions)

	bytes, err := intentSolutions.Pack(CurrentProtocol)
	require.NoError(t, err)

	intentSolutions2 := IntentSolutions{}
	err = intentSolutions2.Unpack(bytes, CurrentProtocol)
	require.NoError(t, err)

	require.Equal(t, intentSolutions.Header.msgType, intentSolutions2.Header.msgType)
	require.Equal(t, len(intentSolutions.Solutions()), len(intentSolutions2.Solutions()))

	for i := 0; i < len(intentSolutions.Solutions()); i++ {
		assert.Equal(t, intentSolutions.Solutions()[i].Solution, intentSolutions2.Solutions()[i].Solution)
	}
}

func createTestIntentSolution(t *testing.T) IntentSolution {
	r := genIntentsRequest()
	solution := []byte("this is test :(")
	intentID, err := intent.GenerateSolutionID("test", []byte("test"))
	require.NoError(t, err)
	id, err := intent.GenerateSolutionID(intentID, solution)
	require.NoError(t, err)

	return IntentSolution{
		Header: Header{
			msgType: IntentSolutionType,
		},
		ID:            id,
		SolverAddress: r.SolverAddress,
		IntentID:      intentID,
		Solution:      solution,
		Hash:          r.Hash,
		Signature:     r.Signature,
		Timestamp:     time.Now(),
	}
}
