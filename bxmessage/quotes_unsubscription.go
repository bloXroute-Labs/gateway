package bxmessage

import (
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// UnpackQuotesUnsubscription unpacks QuotesUnsubscription bxmessage from bytes
func UnpackQuotesUnsubscription(b []byte, protocol Protocol) (*QuotesUnsubscription, error) {
	sub := new(QuotesUnsubscription)
	err := sub.Unpack(b, protocol)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

// QuotesUnsubscription constructor for QuotesUnsubscription bxmessage
type QuotesUnsubscription struct {
	Header
	DAppAddress string
}

// NewQuotesUnsubscription constructor for QuotesUnsubscription bxmessage
func NewQuotesUnsubscription(dAppAddr string) Message {
	return &QuotesUnsubscription{
		Header:      Header{},
		DAppAddress: dAppAddr,
	}
}

// Pack packs QuotesUnsubscription into byte array
func (qu *QuotesUnsubscription) Pack(protocol Protocol) ([]byte, error) {
	if protocol < QuotesProtocol {
		return nil, fmt.Errorf("invalid protocol version for QuotesUnsubscription message: %v", protocol)
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

	n, err := packHeader(buf, qu.Header, QuotesUnsubscriptionType)
	if err != nil {
		return nil, fmt.Errorf("pack Header: %w", err)
	}

	offset += n
	n, err = packETHAddressHex(buf[offset:], qu.DAppAddress)
	if err != nil {
		return nil, fmt.Errorf("pack DappAddress: %w", err)
	}

	offset += n
	if err := checkBuffEnd(&buf, offset); err != nil {
		return nil, err
	}

	return buf, nil
}

// Unpack deserializes QuotesUnsubscription from bytes
func (qu *QuotesUnsubscription) Unpack(buf []byte, protocol Protocol) error {
	var offset int
	if protocol < QuotesProtocol {
		return fmt.Errorf("invalid protocol version for QuotesProtocol message: %v", protocol)
	}

	var err error
	var n int

	qu.Header, n, err = unpackHeader(buf, protocol)
	if err != nil {
		return fmt.Errorf("unpack Header: %w", err)
	}

	offset += n
	qu.DAppAddress, _, err = unpackETHAddressHex(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack DappAddress: %w", err)
	}

	return nil
}
