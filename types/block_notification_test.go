package types

import (
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
)

func TestEthBlock(t *testing.T) {
	block := EthBlockNotification{}
	_ = block.WithFields([]string{"hash", "header", "transactions", "uncles"})
	// TODO add test checking the header values
}

func TestCheckNonce(t *testing.T) {
	nonce := fmt.Sprintf("0x%016s", hexutil.EncodeUint64(57635743)[2:])
	assert.Equal(t, "0x00000000036f739f", nonce)

	nonce = fmt.Sprintf("0x%016s", hexutil.EncodeUint64(0)[2:])
	assert.Equal(t, "0x0000000000000000", nonce)
}
