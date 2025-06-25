package ofac

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/bloXroute-Labs/bxcommon-go/syncmap"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/ethereum/go-ethereum/common"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
)

var (
	testPrivateKey, _     = crypto.GenerateKey()
	testBlockedAddress    = common.HexToAddress("0x1234567890123456789012345678901234567890")
	testNonBlockedAddress = common.HexToAddress("0x9876543210987654321098765432109876543210")
)

func createTestTransaction(privateKey *ecdsa.PrivateKey, to *common.Address) *ethtypes.Transaction {
	nonce := uint64(0)
	gasLimit := uint64(21000)
	gasPrice := big.NewInt(20000000000)
	value := big.NewInt(1000000000000000000)

	txData := &ethtypes.LegacyTx{
		Nonce:    nonce,
		GasPrice: gasPrice,
		Gas:      gasLimit,
		To:       to,
		Value:    value,
		Data:     nil,
	}

	tx := ethtypes.NewTx(txData)
	signer := ethtypes.LatestSignerForChainID(big.NewInt(1))
	signedTx, _ := ethtypes.SignTx(tx, signer, privateKey)
	return signedTx
}

func TestSanctionList_ShouldBlockTransaction_True_WithNilOFACList(t *testing.T) {
	blockedAddress := "0x8576acc5c05d6ce88f4e49bf65bdf0c62f91353c"

	ad := common.HexToAddress(blockedAddress)
	tx := createTestTransaction(testPrivateKey, &ad)

	addresses, shouldBlock := ShouldBlockTransaction(tx, nil)
	assert.Contains(t, addresses, blockedAddress)
	assert.True(t, shouldBlock)
}

func TestSanctionList_ShouldBlockTransaction_False_WithNilOFACList(t *testing.T) {
	tx := createTestTransaction(testPrivateKey, &testNonBlockedAddress)

	addresses, shouldBlock := ShouldBlockTransaction(tx, nil)
	assert.Empty(t, addresses)
	assert.False(t, shouldBlock)
}

func TestShouldBlockTransaction_NoBlockedAddresses(t *testing.T) {
	ofacList := syncmap.NewStringMapOf[bool]()

	tx := createTestTransaction(testPrivateKey, &testNonBlockedAddress)

	blockedAddresses, shouldBlock := ShouldBlockTransaction(tx, ofacList)

	assert.False(t, shouldBlock)
	assert.Nil(t, blockedAddresses)
}

func TestShouldBlockTransaction_FromAddressInOFAC(t *testing.T) {
	ofacList := syncmap.NewStringMapOf[bool]()

	// Add blocked address to OFAC list
	ofacList.Store(testBlockedAddress.String(), true)

	tx := createTestTransaction(testPrivateKey, &testBlockedAddress)
	blockedAddresses, shouldBlock := ShouldBlockTransaction(tx, ofacList)

	assert.True(t, shouldBlock)
	assert.Len(t, blockedAddresses, 1)
	assert.Contains(t, blockedAddresses, testBlockedAddress.String())
}
