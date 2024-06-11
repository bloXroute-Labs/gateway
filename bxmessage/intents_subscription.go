package bxmessage

import (
	"fmt"
)

// UnpackIntentsSubscription unpacks IntentsSubscription bxmessage from bytes
func UnpackIntentsSubscription(b []byte, protocol Protocol) (*IntentsSubscription, error) {
	sub := new(IntentsSubscription)
	err := sub.Unpack(b, protocol)
	if err != nil {
		return nil, err
	}

	return sub, nil
}

// NewIntentsSubscription constructor for IntentsSubscription bxmessage
func NewIntentsSubscription(solverAddr string, hash, signature []byte) Message {
	return &IntentsSubscription{
		Header:        Header{},
		SolverAddress: solverAddr,
		Hash:          hash,
		Signature:     signature,
	}
}

// IntentsSubscription bxmessage
type IntentsSubscription struct {
	Header
	SolverAddress string
	Hash          []byte
	Signature     []byte
}

// Pack packs IntentsSubscription into bytes array for broadcasting
func (i *IntentsSubscription) Pack(protocol Protocol) ([]byte, error) {
	if protocol < IntentsProtocol {
		return nil, fmt.Errorf("invalid protocol version for IntentsSubscription message: %v", protocol)
	}

	bufLen, err := calcPackSize(
		HeaderLen,
		ETHAddressLen,
		Keccak256HashLen,
		ECDSASignatureLen,
		ControlByteLen,
	)
	if err != nil {
		return nil, fmt.Errorf("calc pack size: %w", err)
	}

	buf := make([]byte, bufLen)
	var offset int

	n, err := packHeader(buf, i.Header, IntentsSubscriptionType)
	if err != nil {
		return nil, fmt.Errorf("pack Header: %w", err)
	}

	offset += n
	n, err = packETHAddressHex(buf[offset:], i.SolverAddress)
	if err != nil {
		return nil, fmt.Errorf("pack SolverAddress: %w", err)
	}

	offset += n
	n, err = packKeccak256Hash(buf[offset:], i.Hash)
	if err != nil {
		return nil, fmt.Errorf("pack Hash: %w", err)
	}

	offset += n
	n, err = packECDSASignature(buf[offset:], i.Signature)
	if err != nil {
		return nil, fmt.Errorf("pack Signature: %w", err)
	}

	offset += n
	if err := checkBuffEnd(&buf, offset); err != nil {
		return nil, err
	}

	return buf, nil
}

// Unpack deserializes IntentsSubscription from bytes
func (i *IntentsSubscription) Unpack(buf []byte, protocol Protocol) error {
	var offset int
	var err error
	var n int

	if protocol < IntentsProtocol {
		return fmt.Errorf("invalid protocol version for IntentsSubscription message: %v", protocol)
	}

	i.Header, n, err = unpackHeader(buf, protocol)
	if err != nil {
		return fmt.Errorf("unpack Header: %w", err)
	}

	offset += n
	i.SolverAddress, n, err = unpackETHAddressHex(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack SolverAddress: %w", err)
	}

	offset += n
	i.Hash, n, err = unpackKeccak256Hash(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack Hash: %w", err)
	}

	offset += n
	i.Signature, _, err = unpackECDSASignature(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack Signature: %w", err)
	}

	return nil
}
