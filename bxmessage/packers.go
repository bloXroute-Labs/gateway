package bxmessage

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
)

// validateBufSize checks if buf is capable to fit the size
func validateBufSize(buf []byte, size int) error {
	if ln := len(buf); ln < size {
		return fmt.Errorf("invalid buffer size, expected: %d bytes, actual: %d bytes", size, ln)
	}

	return nil
}

func calcPackSize(fields ...any) (uint32, error) {
	var n uint32
	for _, f := range fields {
		switch x := f.(type) {
		case int:
			n += uint32(x)
		case uint32:
			n += x
		case []byte:
			n += uint32(len(x))
			n += types.UInt32Len
		default:
			return 0, fmt.Errorf("unsupported type %T", f)
		}
	}

	return n, nil
}

func packHeader(dst []byte, header Header, msgType string) (n int, err error) {
	n = HeaderLen
	if err = validateBufSize(dst, n); err != nil {
		return 0, err
	}

	header.Pack(&dst, msgType)
	return
}

func unpackHeader(src []byte, protocol Protocol) (h Header, n int, err error) {
	n = HeaderLen
	if err = validateBufSize(src, n); err != nil {
		return h, n, err
	}

	h = Header{}
	return h, n, h.Unpack(src, protocol)
}

func packKeccak256Hash(dst, hash []byte) (n int, err error) {
	n = Keccak256HashLen
	if x := len(hash); x != n {
		return n, fmt.Errorf("invalid input data size, expected: %d bytes, actual: %d bytes", n, x)
	}

	if err = validateBufSize(dst, n); err != nil {
		return 0, err
	}

	copy(dst, hash)
	return
}

func unpackKeccak256Hash(buf []byte) (hash []byte, n int, err error) {
	n = Keccak256HashLen
	if err = validateBufSize(buf, n); err != nil {
		return nil, n, err
	}

	hash = buf[:n]
	return
}

func packRawBytes(dst []byte, src []byte) (n int, err error) {
	srcln := len(src)
	n = types.UInt32Len + srcln
	if err := validateBufSize(dst, n); err != nil {
		return 0, err
	}

	binary.LittleEndian.PutUint32(dst, uint32(srcln))
	copy(dst[types.UInt32Len:], src)
	return
}

func unpackRawBytes(src []byte) (b []byte, n int, err error) {
	if err := validateBufSize(src, types.UInt32Len); err != nil {
		return b, n, err
	}

	n = types.UInt32Len + int(binary.LittleEndian.Uint32(src[:types.UInt32Len]))
	if err := validateBufSize(src, n); err != nil {
		return nil, 0, err
	}

	return src[types.UInt32Len:n], n, nil
}

func packETHAddressHex(dst []byte, addrHex string) (n int, err error) {
	n = ETHAddressLen
	if err := validateBufSize(dst, n); err != nil {
		return 0, err
	}

	addr := common.HexToAddress(addrHex)
	copy(dst, addr[:])
	return
}

func unpackETHAddressHex(src []byte) (s string, n int, err error) {
	n = ETHAddressLen
	if err := validateBufSize(src, ETHAddressLen); err != nil {
		return s, n, err
	}

	return common.BytesToAddress(src[:ETHAddressLen]).Hex(), n, nil
}

func packECDSASignature(dst []byte, sig []byte) (n int, err error) {
	n = ECDSASignatureLen
	if x := len(sig); x != n {
		return n, fmt.Errorf("invalid input data size, expected: %d bytes, actual: %d bytes", n, x)
	}

	if err := validateBufSize(dst, n); err != nil {
		return 0, err
	}

	copy(dst, sig)
	return
}

func unpackECDSASignature(src []byte) (sig []byte, n int, err error) {
	n = ECDSASignatureLen
	if err := validateBufSize(src, n); err != nil {
		return nil, n, err
	}

	sig = src[:n]
	return
}

func packTimestamp(dst []byte, timestamp time.Time) (n int, err error) {
	n = ShortTimestampLen
	if err = validateBufSize(dst, n); err != nil {
		return 0, err
	}

	binary.LittleEndian.PutUint32(dst, uint32(timestamp.UnixNano()>>10))
	return
}

func unpackTimestamp(src []byte) (t time.Time, n int, err error) {
	n = ShortTimestampLen
	if err := validateBufSize(src, n); err != nil {
		return t, n, err
	}

	t = time.Unix(0, decodeTimestamp(binary.LittleEndian.Uint32(src[:n])))
	return
}

func packUUIDv4String(dst []byte, uid string) (n int, err error) {
	n = UUIDv4Len
	if err = validateBufSize(dst, n); err != nil {
		return n, err
	}

	parsed, err := uuid.Parse(uid)
	if err != nil {
		return n, fmt.Errorf("failed to parse UUID %s: %w", uid, err)
	}

	copy(dst, parsed[:])
	return
}

func unpackUUIDv4String(src []byte) (uid string, n int, err error) {
	n = UUIDv4Len
	if err = validateBufSize(src, n); err != nil {
		return uid, n, err
	}

	u, err := uuid.FromBytes(src[:n])
	if err != nil {
		return uid, n, fmt.Errorf("failed to parse UUID: %w", err)
	}

	uid = u.String()
	return
}
