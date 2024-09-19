package intent

import (
	"bytes"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
)

// GenerateIntentID generates an intent ID from the dApp address, sender address, and intent
func GenerateIntentID(dAppAddress string, intent []byte) (string, error) {
	return generateID(dAppAddress, intent)
}

// GenerateSolutionID generates a solution ID from the solver address, intent ID, and solution
func GenerateSolutionID(intentID string, solution []byte) (string, error) {
	return generateID(intentID, solution)
}

// GenerateQuoteID generates a quote ID from the dapp address, quote ID and quote
func GenerateQuoteID(dAppAddress string, quote []byte) (string, error) {
	return generateID(dAppAddress, quote)
}

func generateID(s string, data []byte) (string, error) {
	hash := crypto.Keccak256Hash([]byte(s + string(data)))
	uuidFromHash, err := uuid.NewRandomFromReader(bytes.NewReader(hash[:]))
	if err != nil {
		return "", err
	}
	return uuidFromHash.String(), nil
}
