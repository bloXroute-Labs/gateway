package eth

import (
	"crypto/ecdsa"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"os"
)

type keyWriteError struct {
	error
}

// NewSHA256Hash is a utility function for converting between Ethereum common hashes and bloxroute hashes
func NewSHA256Hash(hash common.Hash) types.SHA256Hash {
	var sha256Hash types.SHA256Hash
	copy(sha256Hash[:], hash.Bytes())
	return sha256Hash
}

// LoadOrGeneratePrivateKey tries to load an ECDSA private key from the provided path. If this file does not exist, a new key is generated in its place.
func LoadOrGeneratePrivateKey(keyPath string) (privateKey *ecdsa.PrivateKey, generated bool, err error) {
	privateKey, err = crypto.LoadECDSA(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			if privateKey, err = crypto.GenerateKey(); err != nil {
				return
			}

			if err = crypto.SaveECDSA(keyPath, privateKey); err != nil {
				err = keyWriteError{err}
				return
			}

			generated = true
		} else {
			return
		}
	}
	return
}
