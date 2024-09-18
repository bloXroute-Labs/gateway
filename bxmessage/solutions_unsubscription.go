package bxmessage

import (
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// UnpackSolutionsUnsubscription unpacks SolutionsUnsubscription bxmessage from bytes
func UnpackSolutionsUnsubscription(b []byte, protocol Protocol) (*SolutionsUnsubscription, error) {
	sub := new(SolutionsUnsubscription)
	err := sub.Unpack(b, protocol)
	if err != nil {
		return nil, err
	}

	return sub, nil
}

// NewSolutionsUnsubscription constructor for SolutionsUnsubscription bxmessage
func NewSolutionsUnsubscription(dAppAddr string) Message {
	return &SolutionsUnsubscription{
		Header:      Header{},
		DAppAddress: dAppAddr,
	}
}

// SolutionsUnsubscription bxmessage
type SolutionsUnsubscription struct {
	Header
	DAppAddress string
}

// Pack packs SolutionsUnsubscription into bytes
func (i *SolutionsUnsubscription) Pack(protocol Protocol) ([]byte, error) {
	if protocol < IntentsProtocol {
		return nil, fmt.Errorf("invalid protocol version for SolutionsUnsubscription message: %v", protocol)
	}

	bufLen, err := calcPackSize(
		HeaderLen,
		types.ETHAddressLen,
		ControlByteLen,
	)
	if err != nil {
		return nil, fmt.Errorf("calc pack size: %w", err)
	}

	buf := make([]byte, bufLen)
	var offset int

	n, err := packHeader(buf, i.Header, SolutionsUnsubscriptionType)
	if err != nil {
		return nil, fmt.Errorf("pack Header: %w", err)
	}

	offset += n
	n, err = packETHAddressHex(buf[offset:], i.DAppAddress)
	if err != nil {
		return nil, fmt.Errorf("pack DAppAddress: %w", err)
	}

	offset += n
	if err := checkBuffEnd(&buf, offset); err != nil {
		return nil, err
	}

	return buf, nil
}

// Unpack deserilizes SolutionsUnsubscription bxmessage
func (i *SolutionsUnsubscription) Unpack(buf []byte, protocol Protocol) error {
	var offset int
	if protocol < IntentsProtocol {
		return fmt.Errorf("invalid protocol version for SolutionsUnsubscription message: %v", protocol)
	}

	var err error
	var n int

	i.Header, n, err = unpackHeader(buf, protocol)
	if err != nil {
		return fmt.Errorf("unpack Header: %w", err)
	}

	offset += n
	i.DAppAddress, _, err = unpackETHAddressHex(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack DAppAddress: %w", err)
	}

	return nil
}
