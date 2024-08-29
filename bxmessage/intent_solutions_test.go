package bxmessage

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntentSolutions_Unpack(t *testing.T) {
	solutions := make([]IntentSolution, 5)

	for i := 0; i < 5; i++ {
		solutions[i] = createTestIntentSolution()
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

func createTestIntentSolution() IntentSolution {
	r := genIntentsRequest()

	return IntentSolution{
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
}
