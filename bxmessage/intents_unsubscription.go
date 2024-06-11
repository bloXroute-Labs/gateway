package bxmessage

import "fmt"

// UnpackIntentsUnsubscription unpacks IntentsUnsubscription bxmessage from bytes
func UnpackIntentsUnsubscription(b []byte, protocol Protocol) (*IntentsUnsubscription, error) {
	sub := new(IntentsUnsubscription)
	err := sub.Unpack(b, protocol)
	if err != nil {
		return nil, err
	}

	return sub, nil
}

// NewIntentsUnsubscription constructor for IntentsUnsubscription bxmessage
func NewIntentsUnsubscription(solverAddr string) Message {
	return &IntentsUnsubscription{
		Header:        Header{},
		SolverAddress: solverAddr,
	}
}

// IntentsUnsubscription bxmessage
type IntentsUnsubscription struct {
	Header
	SolverAddress string
}

// Pack packs IntentsUnsubscription into byte array
func (i *IntentsUnsubscription) Pack(protocol Protocol) ([]byte, error) {
	if protocol < IntentsProtocol {
		return nil, fmt.Errorf("invalid protocol version for IntentsUnsubscription message: %v", protocol)
	}

	bufLen, err := calcPackSize(
		HeaderLen,
		ETHAddressLen,
		ControlByteLen,
	)
	if err != nil {
		return nil, fmt.Errorf("calc pack size: %w", err)
	}

	buf := make([]byte, bufLen)
	var offset int

	n, err := packHeader(buf, i.Header, IntentsUnsubscriptionType)
	if err != nil {
		return nil, fmt.Errorf("pack Header: %w", err)
	}

	offset += n
	_, err = packETHAddressHex(buf[offset:], i.SolverAddress)
	if err != nil {
		return nil, fmt.Errorf("pack SolverAddress: %w", err)
	}

	return buf, nil
}

// Unpack deserializes IntentsUnsubscription from bytes
func (i *IntentsUnsubscription) Unpack(buf []byte, protocol Protocol) error {
	var offset int
	if protocol < IntentsProtocol {
		return fmt.Errorf("invalid protocol version for IntentsUnsubscription message: %v", protocol)
	}

	var err error
	var n int

	i.Header, n, err = unpackHeader(buf, protocol)
	if err != nil {
		return fmt.Errorf("unpack Header: %w", err)
	}

	offset += n
	i.SolverAddress, _, err = unpackETHAddressHex(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack SolverAddress: %w", err)
	}

	return nil
}
