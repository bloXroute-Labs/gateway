package bxmock

import (
	"crypto/ecdsa"
	"math/big"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

// ChainID ethereum chain ID
var ChainID = big.NewInt(10)
var pKey, _ = crypto.HexToECDSA("dae2cb3b03f8a1bbaedae4d43e159360c8d07ffab119d5d7311a81a9d4f53bd1")

// NewSignedEthTx generates a valid signed Ethereum transaction from a provided private key. nil can be specified to use a hardcoded key.
func NewSignedEthTx(txType uint8, nonce uint64, privateKey *ecdsa.PrivateKey) *ethtypes.Transaction {
	if privateKey == nil {
		privateKey = pKey
	}

	var unsignedTx *ethtypes.Transaction

	switch txType {
	case ethtypes.LegacyTxType:
		unsignedTx = newEthLegacyTx(nonce, privateKey)
	case ethtypes.AccessListTxType:
		unsignedTx = newEthAccessListTx(nonce, privateKey)
	case ethtypes.DynamicFeeTxType:
		unsignedTx = newEthDynamicFeeTx(nonce, privateKey)
	default:
		panic("provided tx type does not exist")
	}

	signer := ethtypes.NewLondonSigner(ChainID)
	hash := signer.Hash(unsignedTx)
	signature, _ := crypto.Sign(hash.Bytes(), privateKey)

	signedTx, _ := unsignedTx.WithSignature(signer, signature)
	return signedTx
}

// NewSignedEthTxBytes generates a valid Ethereum transaction, and packs it into RLP encoded bytes
func NewSignedEthTxBytes(txType uint8, nonce uint64, privateKey *ecdsa.PrivateKey) (*ethtypes.Transaction, []byte) {
	tx := NewSignedEthTx(txType, nonce, privateKey)
	var b []byte
	var err error
	switch txType {
	case ethtypes.LegacyTxType:
		b, err = rlp.EncodeToBytes(tx)
		if err != nil {
			panic(err)
		}
	case ethtypes.DynamicFeeTxType, ethtypes.AccessListTxType:
		b, err = tx.MarshalBinary()
		if err != nil {
			panic(err)
		}
	default:
		panic("provided tx type does not exist")
	}

	return tx, b
}

// NewSignedEthTxMessage generates a valid Ethereum transaction, and packs it into a bloxroute tx message
func NewSignedEthTxMessage(txType uint8, nonce uint64, privateKey *ecdsa.PrivateKey, networkNum types.NetworkNum, flags types.TxFlags) (*ethtypes.Transaction, *bxmessage.Tx) {
	ethTx, ethTxBytes := NewSignedEthTxBytes(txType, nonce, privateKey)
	var hash types.SHA256Hash
	copy(hash[:], ethTx.Hash().Bytes())
	return ethTx, bxmessage.NewTx(hash, ethTxBytes, networkNum, flags, "")
}

// newEthLegacyTx generates a valid signed Ethereum transaction from a provided private key. nil can be specified to use a hardcoded private key.
func newEthLegacyTx(nonce uint64, privateKey *ecdsa.PrivateKey) *ethtypes.Transaction {
	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	unsignedTx := ethtypes.NewTx(&ethtypes.LegacyTx{
		Nonce:    nonce,
		GasPrice: big.NewInt(100),
		Gas:      0,
		To:       &address,
		Value:    big.NewInt(1),
		Data:     []byte{},
		V:        nil,
		R:        nil,
		S:        nil,
	})
	return unsignedTx
}

func newEthAccessListTx(nonce uint64, privateKey *ecdsa.PrivateKey) *ethtypes.Transaction {
	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	unsignedTx := ethtypes.NewTx(&ethtypes.AccessListTx{
		ChainID:    ChainID,
		Nonce:      nonce,
		GasPrice:   big.NewInt(100),
		Gas:        0,
		To:         &address,
		Value:      big.NewInt(1),
		Data:       []byte{},
		AccessList: nil,
		V:          nil,
		R:          nil,
		S:          nil,
	})
	return unsignedTx
}

// newEthDynamicFeeTx generates a valid signed Ethereum transaction from a provided private key. nil can be specified to use a hardcoded private key.
func newEthDynamicFeeTx(nonce uint64, privateKey *ecdsa.PrivateKey) *ethtypes.Transaction {
	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	unsignedTx := ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		ChainID:    ChainID,
		Nonce:      nonce,
		GasTipCap:  big.NewInt(100),
		GasFeeCap:  big.NewInt(100),
		Gas:        0,
		To:         &address,
		Value:      big.NewInt(1),
		Data:       []byte{},
		AccessList: nil,
		V:          nil,
		R:          nil,
		S:          nil,
	})
	return unsignedTx
}
