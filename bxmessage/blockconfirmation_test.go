package bxmessage

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEmptyPackSize(t *testing.T) {
	bc := BlockConfirmation{}
	buf, err := bc.Pack(22)
	assert.Equal(t, err, nil)
	assert.Equal(t, len(buf), 81)
}
