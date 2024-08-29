package bxmessage

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestIntentSolution_Unpack(t *testing.T) {
	tests := []struct {
		name               string
		protocol           Protocol
		emptyDAppAddress   bool
		emptySenderAddress bool
	}{
		{
			name:               "intents protocol",
			protocol:           IntentsProtocol,
			emptyDAppAddress:   true,
			emptySenderAddress: true,
		},
		{
			name:               "intent solution protocol",
			protocol:           IntentSolutionProtocol,
			emptyDAppAddress:   false,
			emptySenderAddress: true,
		},
		{
			name:               "solution sender address protocol",
			protocol:           SolutionSenderAddressProtocol,
			emptyDAppAddress:   false,
			emptySenderAddress: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := genIntentsRequest()
			is := NewIntentSolution(uuid.New().String(), r.SolverAddress, uuid.New().String(), r.Hash, r.Signature, time.Now(), []byte("this is test :("))
			solution, ok := is.(*IntentSolution)
			require.True(t, ok)

			privKey, err := crypto.GenerateKey()
			require.NoError(t, err)
			dAppAddress := crypto.PubkeyToAddress(privKey.PublicKey).String()
			solution.DappAddress = dAppAddress
			solution.SenderAddress = dAppAddress

			bytes, err := solution.Pack(tt.protocol)
			require.NoError(t, err)

			solution2 := IntentSolution{}
			err = solution2.Unpack(bytes, tt.protocol)
			require.NoError(t, err)

			require.Equal(t, solution.Header.msgType, solution2.Header.msgType)
			require.Equal(t, solution.ID, solution2.ID)
			require.Equal(t, solution.SolverAddress, solution2.SolverAddress)
			require.Equal(t, solution.IntentID, solution2.IntentID)
			require.Equal(t, solution.Solution, solution2.Solution)
			require.Equal(t, solution.Hash, solution2.Hash)
			require.Equal(t, solution.Signature, solution2.Signature)
			require.Equal(t, solution.Timestamp.UnixMilli(), solution2.Timestamp.UnixMilli())

			if tt.emptyDAppAddress {
				require.Empty(t, solution2.DappAddress)
			} else {
				require.Equal(t, solution.DappAddress, solution2.DappAddress)
			}

			if tt.emptySenderAddress {
				require.Empty(t, solution2.SenderAddress)
			} else {
				require.Equal(t, solution.SenderAddress, solution2.SenderAddress)
			}
		})
	}
}
