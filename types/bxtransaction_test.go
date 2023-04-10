package types

import (
	"encoding/hex"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

// example of real tx
// {"jsonrpc":"2.0","id":null,"method":"subscribe","params":{"subscription":"65770230-8550-46cd-8059-4f6e13484c83",
// "result":{"txContents":{"from":"0x832f166799a407275500430b61b622f0058f15d6","gas":"0x13880","gasPrice":"0x1bf08eb000",
// "hash":"0xed2b4580a766bc9d81c73c35a8496f0461e9c261621cb9f4565ae52ade56056d","input":"0x","nonce":"0x1b7f8",
// "value":"0x50b32f902486000","v":"0x1c","r":"0xaa803263146bda76a58ebf9f54be589280e920616bc57e7bd68248821f46fd0c",
// "s":"0x40266f84a2ecd4719057b0633cc80e3e0b3666f6f6ec1890a920239634ec6531","to":"0xb877c7e556d50b0027053336b90f36becf67b3dd"}}}}
var testNetworkNum = NetworkNum(5)

func TestValidContentParsing(t *testing.T) {
	var hash SHA256Hash
	hashRes, _ := hex.DecodeString("ed2b4580a766bc9d81c73c35a8496f0461e9c261621cb9f4565ae52ade56056d")
	copy(hash[:], hashRes)

	content, _ := hex.DecodeString("f8708301b7f8851bf08eb0008301388094b877c7e556d50b0027053336b90f36becf67b3dd88050b32f902486000801ca0aa803263146bda76a58ebf9f54be589280e920616bc57e7bd68248821f46fd0ca040266f84a2ecd4719057b0633cc80e3e0b3666f6f6ec1890a920239634ec6531")

	tx := NewBxTransaction(hash, testNetworkNum, TFPaidTx, time.Now())
	tx.SetContent(content)
	blockchainTx, err := tx.BlockchainTransaction(EmptySender)
	assert.Nil(t, err)

	ethTx, ok := blockchainTx.(*EthTransaction)
	ethTx.Filters([]string{})
	assert.True(t, ok)

	assert.Equal(t, "0x0", ethTx.fields["type"])
	assert.Nil(t, ethTx.fields["AccessList"])
	assert.Equal(t, "0xb877c7e556d50b0027053336b90f36becf67b3dd", ethTx.fields["to"])
	assert.Equal(t, "0x40266f84a2ecd4719057b0633cc80e3e0b3666f6f6ec1890a920239634ec6531", ethTx.fields["s"])
	assert.Equal(t, "0xaa803263146bda76a58ebf9f54be589280e920616bc57e7bd68248821f46fd0c", ethTx.fields["r"])
	assert.Equal(t, "0x1c", ethTx.fields["v"])
	assert.Equal(t, "0x50b32f902486000", ethTx.fields["value"])
	assert.Equal(t, uint64(0x1b7f8), ethTx.Nonce)
	assert.Equal(t, "0x", ethTx.fields["input"])
	assert.Equal(t, "0xed2b4580a766bc9d81c73c35a8496f0461e9c261621cb9f4565ae52ade56056d", ethTx.fields["hash"])
	assert.Equal(t, "0x832f166799a407275500430b61b622f0058f15d6", ethTx.fields["from"])
	assert.Equal(t, "0x1bf08eb000", ethTx.fields["gasPrice"])
	assert.Equal(t, "0x13880", ethTx.fields["gas"])

}

//func TestValidContentParsingType1Tx(t *testing.T) {
//	var hash SHA256Hash
//	hashRes, _ := hex.DecodeString("9310dc4f07748222d37f43c7296826cf4bf6693fa207968bd7500659ee2cc04d")
//	copy(hash[:], hashRes)
//
//	content, _ := hex.DecodeString("01f9022101829237853486ced000830285ee94653911da49db4cdb0b7c3e4d929cfb56024cd4e680b8a48201aa3f000000000000000000000000c02aaa39b223fe8d0a0e5c4f27ead9083c756cc200000000000000000000000000000000000000000000000083a297567e20f8000000000000000000000000000d8775f648430679a709e98d2b0cb6250d2887ef000000000000000000000000000000000000000000000358c5ee87d374000000fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff90111f859940d8775f648430679a709e98d2b0cb6250d2887eff842a01d76467e21923adb4ee07bcae017030c6208bbccde21ff0a61518956ad9b152aa0ec5bfdd140da829800c64d740e802727fca06fadec8b5d82a7b406c811851b55f85994653911da49db4cdb0b7c3e4d929cfb56024cd4e6f842a02a9a57a342e03a2b55a8bef24e9c777df22a7442475b1641875a66dba65855f0a0d0bcf4df132c65dad73803c5e5e1c826f151a3342680034a8a4c8e5f8eb0c13ff85994c02aaa39b223fe8d0a0e5c4f27ead9083c756cc2f842a01fc85b67921559ce4fef22a331ff00c886678cf8b163d395e45fe0543f8750bda047a365b3ae9dbfa1c60a2cd30347765e914a89b7dac828db3ac3bd3e775b1a9980a0a340fc367050387a1b295514210a48de3836ee7923d9739bf0104e6d79c37997a06e63c7801da3c72f1a53f9b809ae04a45637da673c2a9c47065a1b1cdeafef7d")
//	tx := NewBxTransaction(hash, 5)
//	tx.SetContent(content)
//	blockchainTx, err := tx.BlockchainTransaction()
//	assert.Nil(t, err)
//
//	ethTx, ok := blockchainTx.(*Transaction)
//	assert.True(t, ok)
//
//	assert.Equal(t, uint8(1), ethTx.TxType.UInt8)
//	assert.Equal(t, int64(1), ethTx.ChainID.Int64())
//	assert.Equal(t, 6, ethTx.AccessList.StorageKeys())
//	assert.Equal(t, "0x653911da49db4cdb0b7c3e4d929cfb56024cd4e6", strings.ToLower(ethTx.To.String()))
//	assert.Equal(t, uint64(37431), ethTx.Nonce.UInt64)
//	assert.Equal(t, "9310dc4f07748222d37f43c7296826cf4bf6693fa207968bd7500659ee2cc04d", ethTx.Hash.String())
//	assert.Equal(t, uint64(225600000000), ethTx.GasPrice.Uint64())
//	assert.Equal(t, uint64(165358), ethTx.GasLimit.UInt64)
//	assert.Equal(t, "0x0087c5900b9bbc051b5f6299f5bce92383273b28", strings.ToLower(ethTx.From.String()))
//}

func TestNotValidContentParsing(t *testing.T) {
	var hash SHA256Hash
	hashRes, _ := hex.DecodeString("aaaa2050da578b41d47fd664974bb1bda379be0a3949976a19d91a6cb7ca2079")
	copy(hash[:], hashRes)

	content, _ := hex.DecodeString("aaaa510e8516d1415400830283f9947a250d5630b4cf539739df2c5dacb4c659f2488d87b1a2bc2ec50000b8e47ff36ab5000000000000000000000000000000000000000000000010ee3c5d3728912a5d0000000000000000000000000000000000000000000000000000000000000080000000000000000000000000fd4d885c79fe72447239f50372940926b88017f5000000000000000000000000000000000000000000000000000000006024dfbb0000000000000000000000000000000000000000000000000000000000000002000000000000000000000000c02aaa39b223fe8d0a0e5c4f27ead9083c756cc20000000000000000000000009ed8e7c9604790f7ec589f99b94361d8aab64e5e26a076c06c21ac0d27d2866af4f344538f695b1729b54b764a32a758b5849df3b418a0229f5f935a60d4ef051c074ab49de8270f6ce949ba5e758d8de08923ff087cad")

	tx := &BxTransaction{
		hash:       hash,
		content:    content,
		shortIDs:   make(ShortIDList, 0),
		addTime:    time.Now(),
		networkNum: testNetworkNum,
	}

	blockchainTx, err := tx.BlockchainTransaction(EmptySender)
	assert.NotNil(t, err)
	assert.Nil(t, blockchainTx)
}

func TestHasContent(t *testing.T) {
	var hash SHA256Hash
	content, _ := hex.DecodeString("01")

	txNoContent := &BxTransaction{
		hash:       hash,
		shortIDs:   make(ShortIDList, 0),
		addTime:    time.Now(),
		networkNum: testNetworkNum,
	}
	assert.False(t, txNoContent.HasContent())
	txWithContent := &BxTransaction{
		hash:       hash,
		content:    content,
		shortIDs:   make(ShortIDList, 0),
		addTime:    time.Now(),
		networkNum: testNetworkNum,
	}
	assert.True(t, txWithContent.HasContent())

	txNoContent.SetContent(content)
	assert.True(t, txNoContent.HasContent())
}
