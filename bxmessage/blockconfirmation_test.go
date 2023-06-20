package bxmessage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEmptyPackSize(t *testing.T) {
	bc := BlockConfirmation{}
	buf, err := bc.Pack(22)
	assert.Equal(t, err, nil)
	assert.Equal(t, len(buf), 81)
}
