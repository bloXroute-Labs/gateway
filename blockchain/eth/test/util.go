package test

import (
	"github.com/ethereum/go-ethereum/p2p/enode"
	"math/rand"
)

// GenerateEnodeID randomly creates an enode for testing purposes
func GenerateEnodeID() enode.ID {
	var enodeID enode.ID
	id := make([]byte, 32)
	_, _ = rand.Read(id)

	copy(enodeID[:], id)
	return enodeID
}
