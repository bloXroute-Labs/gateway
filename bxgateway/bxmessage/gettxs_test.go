package bxmessage

import (
	"encoding/hex"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/types"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetTxsPack(t *testing.T) {
	getTxs := GetTxs{}
	getTxs.ShortIDs = []types.ShortID{1, 2, 3, 4, 5}
	x, _ := getTxs.Pack(0)
	assert.Equal(t, "fffefdfc6765747478730000000000001900000005000000010000000200000003000000040000000500000001", hex.EncodeToString(x))
}
func TestGetTxsUnpack(t *testing.T) {
	getTxs := GetTxs{}
	x, _ := hex.DecodeString("fffefdfc6765747478730000000000001900000005000000010000000200000003000000040000000500000001")
	_ = getTxs.Unpack(x, 0)
	assert.Equal(t, types.ShortIDList{1, 2, 3, 4, 5}, getTxs.ShortIDs)
}
