package bxmessage

import (
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// UnpackGetIntentSolutions unpacks a GetIntentSolutions message from a buffer
func UnpackGetIntentSolutions(b []byte, protocol Protocol) (*GetIntentSolutions, error) {
	s := new(GetIntentSolutions)
	err := s.Unpack(b, protocol)
	if err != nil {
		return nil, err
	}

	return s, nil
}

// GetIntentSolutions - represent the "getintsol" message
type GetIntentSolutions struct {
	Header
	IntentID            string // UUIDv4
	DAppOrSenderAddress string // ETH Address
	Hash                []byte // hash of the intentID+DAppAddress
	Signature           []byte // ECDSA Signature
}

// NewGetIntentSolutions - create a new GetIntentSolutions message
func NewGetIntentSolutions(intentID, dAppOrSenderAddress string, hash, signature []byte) *GetIntentSolutions {
	return &GetIntentSolutions{
		Header: Header{
			msgType: GetIntentSolutionsType,
		},
		IntentID:            intentID,
		DAppOrSenderAddress: dAppOrSenderAddress,
		Hash:                hash,
		Signature:           signature,
	}
}

// Pack serializes a GetIntentSolutions into a buffer for sending
func (s *GetIntentSolutions) Pack(_ Protocol) ([]byte, error) {
	bufLen, err := calcPackSize(
		HeaderLen,
		UUIDv4Len,
		ETHAddressLen,
		Keccak256HashLen,
		ECDSASignatureLen,
		ControlByteLen,
	)
	if err != nil {
		return nil, fmt.Errorf("calc pack size: %w", err)
	}

	offset := 0
	buf := make([]byte, bufLen)

	n, err := packHeader(buf, s.Header, GetIntentSolutionsType)
	if err != nil {
		return nil, fmt.Errorf("pack Header: %w", err)
	}

	offset += n
	n, err = packUUIDv4String(buf[offset:], s.IntentID)
	if err != nil {
		return nil, fmt.Errorf("pack ID: %w", err)
	}

	offset += n
	n, err = packETHAddressHex(buf[offset:], s.DAppOrSenderAddress)
	if err != nil {
		return nil, fmt.Errorf("pack DAppOrSenderAddress: %w", err)
	}

	offset += n
	n, err = packKeccak256Hash(buf[offset:], s.Hash)
	if err != nil {
		return nil, fmt.Errorf("pack Hash: %w", err)
	}

	offset += n
	n, err = packECDSASignature(buf[offset:], s.Signature)
	if err != nil {
		return nil, fmt.Errorf("pack Signature: %w", err)
	}

	offset += n
	if err = checkBuffEnd(&buf, offset); err != nil {
		return nil, err
	}

	return buf, nil
}

// Unpack deserializes a GetIntentSolutions from a buffer
func (s *GetIntentSolutions) Unpack(buf []byte, protocol Protocol) error {
	var (
		err    error
		n      int
		offset = 0
	)

	s.Header, n, err = unpackHeader(buf, protocol)
	if err != nil {
		return fmt.Errorf("unpack BroadcastHeader: %v", err)
	}

	offset += n
	s.IntentID, n, err = unpackUUIDv4String(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack intent ID: %v", err)
	}

	offset += n
	s.DAppOrSenderAddress, n, err = unpackETHAddressHex(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack DAppOrSenderAddress: %v", err)
	}

	offset += n
	s.Hash, n, err = unpackKeccak256Hash(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack Hash: %v", err)
	}

	offset += n
	s.Signature, _, err = unpackECDSASignature(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack Signature: %v", err)
	}

	return nil
}

// GetNetworkNum returns the network number
func (s *GetIntentSolutions) GetNetworkNum() types.NetworkNum { return types.AllNetworkNum }
