package bxmessage

import (
	"testing"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/stretchr/testify/assert"
)

func TestValidatorUpdatesPackUnpack(t *testing.T) {
	onlineList := []string{
		"0x0bac492386862ad3df4b666bc096b0505bb694da",
		"0x2465176c461afb316ebc773c61faee85a6515daa",
		"0x295e26495cef6f69dfa69911d9d8e4f3bbadb89b",
	}
	vu, err := NewValidatorUpdates(bxgateway.BSCMainnetNum, 3, onlineList)

	assert.NoError(t, err)

	b, err := vu.Pack(0)
	assert.NoError(t, err)

	var update ValidatorUpdates
	err = update.Unpack(b, 0)
	assert.NoError(t, err)

	assert.Equal(t, bxgateway.BSCMainnetNum, types.NetworkNum(update.networkNum))
	assert.Equal(t, 3, int(update.onlineListLength))
}
