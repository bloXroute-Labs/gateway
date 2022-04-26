package utils

import "crypto/sha256"

// DoubleSHA256 - returns the SHA256 checksum of the data after two checksums
func DoubleSHA256(buf []byte) [32]byte {
	hash := sha256.Sum256(buf[:])
	return sha256.Sum256(hash[:])
}
