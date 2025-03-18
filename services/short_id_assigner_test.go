package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShortIDAssigner(t *testing.T) {
	assigner := NewEmptyShortIDAssigner()
	sum := 0
	for i := 0; i < 1000; i++ {
		sum += int(assigner.Next())
	}
	assert.Equal(t, sum, 0)
}
