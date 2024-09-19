package bxmessage

import (
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// UnpackQuotesSubscription unpacks QuotesSubscription bxmessage from bytes
func UnpackQuotesSubscription(b []byte, protocol Protocol) (*QuotesSubscription, error) {
	sub := new(QuotesSubscription)
	err := sub.Unpack(b, protocol)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

// NewQuotesSubscription constructor for QuotesSubscription bxmessage
func NewQuotesSubscription(dappAddress string) Message {
	return &QuotesSubscription{
		Header:      Header{},
		DappAddress: dappAddress,
	}
}

// QuotesSubscription bxmessage
type QuotesSubscription struct {
	Header
	DappAddress string
}

// Pack packs IntentsQuotesSubscriptionType into bytes array for broadcasting
func (q *QuotesSubscription) Pack(protocol Protocol) ([]byte, error) {
	if protocol < QuotesProtocol {
		return nil, fmt.Errorf("invalid protocol version for QuotesProtocol message: %v", protocol)
	}

	bufLen, err := calcPackSize(HeaderLen, types.ETHAddressLen, ControlByteLen)
	if err != nil {
		return nil, fmt.Errorf("calc pack size: %w", err)
	}

	buf := make([]byte, bufLen)
	var offset int

	n, err := packHeader(buf, q.Header, QuotesSubscriptionType)
	if err != nil {
		return nil, fmt.Errorf("pack Header: %w", err)
	}

	offset += n
	n, err = packETHAddressHex(buf[offset:], q.DappAddress)
	if err != nil {
		return nil, fmt.Errorf("pack SolverAddress: %w", err)
	}

	offset += n
	if err := checkBuffEnd(&buf, offset); err != nil {
		return nil, err
	}

	return buf, nil
}

// Unpack deserializes IntentsQuotesSubscriptionType from bytes
func (q *QuotesSubscription) Unpack(buf []byte, protocol Protocol) error {
	var (
		err    error
		n      int
		offset = 0
	)

	if protocol < QuotesProtocol {
		return fmt.Errorf("invalid protocol version for QuotesProtocol message: %v", protocol)
	}

	q.Header, n, err = unpackHeader(buf, protocol)
	if err != nil {
		return fmt.Errorf("unpack Header: %w", err)
	}

	offset += n
	q.DappAddress, _, err = unpackETHAddressHex(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack DappAddress: %w", err)
	}

	return nil
}
