package types

import (
	"crypto/sha256"
	"encoding/hex"
	utils2 "github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/bxmessage/utils"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSHA256(t *testing.T) {
	buff := []byte{255, 254, 253, 252, 103, 101, 116, 116, 120, 115, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 1}
	hash := sha256.Sum256(buff)
	res := sha256.Sum256(hash[:])
	assert.Equal(t, "79ba4333350dfddd68ad06c44ff195ee991450d35cfa2b961f72d117ffd6fc95", hex.EncodeToString(res[:]))
	res2 := utils2.DoubleSHA256([]byte{255, 254, 253, 252, 103, 101, 116, 116, 120, 115, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 1})
	assert.Equal(t, "79ba4333350dfddd68ad06c44ff195ee991450d35cfa2b961f72d117ffd6fc95", hex.EncodeToString(res2[:]))
}
