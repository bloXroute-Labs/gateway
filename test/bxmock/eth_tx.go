package bxmock

import (
	"crypto/ecdsa"
	"math/big"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
)

// ChainID ethereum chain ID
var (
	ChainID = big.NewInt(10)
	pKey, _ = crypto.HexToECDSA("dae2cb3b03f8a1bbaedae4d43e159360c8d07ffab119d5d7311a81a9d4f53bd1") //nolint:errcheck
)

// NewSignedEthTx generates a valid signed Ethereum transaction from a provided private key. nil can be specified to use a hardcoded key.
func NewSignedEthTx(txType uint8, nonce uint64, privateKey *ecdsa.PrivateKey, chainID *big.Int) *ethtypes.Transaction {
	if privateKey == nil {
		privateKey = pKey
	}
	if chainID == nil {
		chainID = ChainID
	}

	var unsignedTx *ethtypes.Transaction

	switch txType {
	case ethtypes.LegacyTxType:
		unsignedTx = newEthLegacyTx(nonce, privateKey)
	case ethtypes.AccessListTxType:
		unsignedTx = newEthAccessListTx(nonce, privateKey, chainID)
	case ethtypes.DynamicFeeTxType:
		unsignedTx = newEthDynamicFeeTx(nonce, privateKey, chainID)
	case ethtypes.BlobTxType:
		unsignedTx = newEthBlobTx(nonce, privateKey, uint256.MustFromBig(chainID))
	default:
		panic("provided tx type does not exist")
	}

	signer := ethtypes.NewCancunSigner(chainID)
	hash := signer.Hash(unsignedTx)
	signature, _ := crypto.Sign(hash.Bytes(), privateKey)

	signedTx, _ := unsignedTx.WithSignature(signer, signature)
	return signedTx
}

// NewSignedEthBlobTxWithSidecar generates a valid signed Ethereum transaction with a blob sidecar from a provided private key. nil can be specified to use a hardcoded key.
func NewSignedEthBlobTxWithSidecar(nonce uint64, privateKey *ecdsa.PrivateKey, chainID *big.Int, blobSidecar *ethtypes.BlobTxSidecar) *ethtypes.Transaction {
	if privateKey == nil {
		privateKey = pKey
	}
	if chainID == nil {
		chainID = ChainID
	}

	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	unsignedTx := ethtypes.NewTx(&ethtypes.BlobTx{
		ChainID:    uint256.MustFromBig(chainID),
		Nonce:      nonce,
		GasTipCap:  uint256.NewInt(100),
		GasFeeCap:  uint256.NewInt(100),
		Gas:        0,
		To:         address,
		Value:      uint256.NewInt(1),
		Data:       []byte{},
		BlobFeeCap: uint256.NewInt(100),
		BlobHashes: blobSidecar.BlobHashes(),
		Sidecar:    blobSidecar,
		AccessList: ethtypes.AccessList{},
		V:          &uint256.Int{},
		R:          &uint256.Int{},
		S:          &uint256.Int{},
	})

	signer := ethtypes.NewCancunSigner(chainID)
	hash := signer.Hash(unsignedTx)
	signature, _ := crypto.Sign(hash.Bytes(), privateKey)

	signedTx, _ := unsignedTx.WithSignature(signer, signature)
	return signedTx
}

// NewSignedEthTxBytes generates a valid Ethereum transaction, and packs it into RLP encoded bytes
func NewSignedEthTxBytes(txType uint8, nonce uint64, privateKey *ecdsa.PrivateKey, chainID *big.Int) (*ethtypes.Transaction, []byte) {
	tx := NewSignedEthTx(txType, nonce, privateKey, chainID)

	var b []byte
	var err error

	if txType == ethtypes.BlobTxType {
		b, err = rlp.EncodeToBytes(tx)
	} else {
		b, err = tx.MarshalBinary()
	}

	if err != nil {
		panic(err)
	}

	return tx, b
}

// NewSignedEthTxMessage generates a valid Ethereum transaction, and packs it into a bloxroute tx message
func NewSignedEthTxMessage(txType uint8, nonce uint64, privateKey *ecdsa.PrivateKey, networkNum types.NetworkNum, flags types.TxFlags, chainID *big.Int) (*ethtypes.Transaction, *bxmessage.Tx) {
	ethTx, ethTxBytes := NewSignedEthTxBytes(txType, nonce, privateKey, chainID)
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

func newEthAccessListTx(nonce uint64, privateKey *ecdsa.PrivateKey, chainID *big.Int) *ethtypes.Transaction {
	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	unsignedTx := ethtypes.NewTx(&ethtypes.AccessListTx{
		ChainID:    chainID,
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

// newEthBlobTx generates a valid signed Ethereum transaction from a provided private key. nil can be specified to use a hardcoded private key.
func newEthBlobTx(nonce uint64, privateKey *ecdsa.PrivateKey, chainID *uint256.Int) *ethtypes.Transaction {
	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	unsignedTx := ethtypes.NewTx(&ethtypes.BlobTx{
		ChainID:    chainID,
		Nonce:      nonce,
		GasTipCap:  uint256.NewInt(100),
		GasFeeCap:  uint256.NewInt(100),
		Gas:        0,
		To:         address,
		Value:      uint256.NewInt(1),
		Data:       []byte{},
		BlobFeeCap: uint256.NewInt(100),
		BlobHashes: []common.Hash{},
		Sidecar: &ethtypes.BlobTxSidecar{
			Blobs:       []kzg4844.Blob{},
			Commitments: []kzg4844.Commitment{},
			Proofs:      []kzg4844.Proof{},
		},
		AccessList: ethtypes.AccessList{},
		V:          &uint256.Int{},
		R:          &uint256.Int{},
		S:          &uint256.Int{},
	})
	return unsignedTx
}

// newEthDynamicFeeTx generates a valid signed Ethereum transaction from a provided private key. nil can be specified to use a hardcoded private key.
func newEthDynamicFeeTx(nonce uint64, privateKey *ecdsa.PrivateKey, chainID *big.Int) *ethtypes.Transaction {
	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	unsignedTx := ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		ChainID:    chainID,
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
