package bxmessage

import "fmt"

// UnpackSolutionsSubscription unpacks SolutionsSubscription bxmessage from bytes
func UnpackSolutionsSubscription(b []byte, protocol Protocol) (*SolutionsSubscription, error) {
	var sub = new(SolutionsSubscription)
	err := sub.Unpack(b, protocol)
	if err != nil {
		return nil, err
	}

	return sub, nil
}

// NewSolutionsSubscription constructor for SolutionsSubscription bxmessage
func NewSolutionsSubscription(dAppAddr string, hash, signature []byte) Message {
	return &SolutionsSubscription{
		Header:      Header{},
		DAppAddress: dAppAddr,
		Hash:        hash,
		Signature:   signature,
	}
}

// SolutionsSubscription describes solutions subscription bxmessage
type SolutionsSubscription struct {
	Header
	DAppAddress string
	Hash        []byte
	Signature   []byte
}

// Pack packs SolutionsSubscription into bytes array
func (i *SolutionsSubscription) Pack(protocol Protocol) ([]byte, error) {
	if protocol < IntentsProtocol {
		return nil, fmt.Errorf("invalid protocol version for SolutionsSubscription message: %v", protocol)
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

	var buf = make([]byte, bufLen)
	var offset int

	n, err := packHeader(buf, i.Header, SolutionsSubscriptionType)
	if err != nil {
		return nil, fmt.Errorf("pack Header: %w", err)
	}

	offset += n
	n, err = packETHAddressHex(buf[offset:], i.DAppAddress)
	if err != nil {
		return nil, fmt.Errorf("pack DAppAddress: %w", err)
	}

	offset += n
	n, err = packKeccak256Hash(buf[offset:], i.Hash)
	if err != nil {
		return nil, fmt.Errorf("pack Hash: %w", err)
	}

	offset += n
	_, err = packECDSASignature(buf[offset:], i.Signature)
	if err != nil {
		return nil, fmt.Errorf("pack Signature: %w", err)
	}

	return buf, nil
}

// Unpack deserializes SolutionSubscription from bytes
func (i *SolutionsSubscription) Unpack(buf []byte, protocol Protocol) error {
	var offset int

	if protocol < IntentsProtocol {
		return fmt.Errorf("invalid protocol version for SolutionsSubscription message: %v", protocol)
	}

	var err error
	var n int

	i.Header, n, err = unpackHeader(buf, protocol)
	if err != nil {
		return fmt.Errorf("unpack Header: %w", err)
	}

	offset += n
	i.DAppAddress, n, err = unpackETHAddressHex(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack DAppAddress: %w", err)
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
