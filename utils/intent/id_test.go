package intent

import (
	"testing"

	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestGenerateIntentID(t *testing.T) {
	dAppAddress := "dAppAddress"
	intent := []byte("intent")

	got, err := GenerateIntentID(dAppAddress, intent)
	require.NoError(t, err)
	require.NotEmpty(t, got)
	require.Len(t, common.HexToHash(got).Bytes(), types.Keccak256HashLen)
}

func TestGenerateSolutionID(t *testing.T) {
	intentID := "intentID"
	solution := []byte("solution")

	got, err := GenerateSolutionID(intentID, solution)
	require.NoError(t, err)
	require.NotEmpty(t, got)
	_, err = uuid.Parse(got)
	require.NoError(t, err)
}
