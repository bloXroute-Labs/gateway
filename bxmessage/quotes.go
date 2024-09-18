package bxmessage

import (
	"fmt"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// Quote bxmessage
type Quote struct {
	Header
	ID            string    // UUIDv4 - quote id
	DappAddress   string    // ETH Address
	SolverAddress string    // ETH Address
	Quote         []byte    // Variable length
	Hash          []byte    // Keccak256
	Signature     []byte    // ECDSA Signature
	Timestamp     time.Time // Short timestamp
}

// NewQuote constructor for Quote bxmessage
func NewQuote(id, dappAddress, solverAddr string, hash, signature, quote []byte, timestamp time.Time) *Quote {
	return &Quote{
		Header:        Header{msgType: QuotesType},
		ID:            id,
		DappAddress:   dappAddress,
		SolverAddress: solverAddr,
		Quote:         quote,
		Hash:          hash,
		Signature:     signature,
		Timestamp:     timestamp,
	}
}

// UnpackQuote unpacks quote bxmessage
func UnpackQuote(b []byte, protocol Protocol) (*Quote, error) {
	quote := new(Quote)
	err := quote.Unpack(b, protocol)
	if err != nil {
		return nil, err
	}

	return quote, nil
}

// Pack packs Quote into raw bytes for broadcasting
func (q *Quote) Pack(protocol Protocol) ([]byte, error) {
	if protocol < QuotesProtocol {
		return nil, fmt.Errorf("invalid protocol version for IntentSolution message: %v", protocol)
	}
	bufLen, err := q.size()
	if err != nil {
		return nil, fmt.Errorf("calc pack size: %w", err)
	}

	offset := 0
	buf := make([]byte, bufLen)

	n, err := packHeader(buf, q.Header, QuotesType)
	if err != nil {
		return nil, fmt.Errorf("pack Header: %w", err)
	}

	offset += n
	n, err = packUUIDv4String(buf[offset:], q.ID)
	if err != nil {
		return nil, fmt.Errorf("pack ID: %w", err)
	}

	offset += n
	n, err = packETHAddressHex(buf[offset:], q.DappAddress)
	if err != nil {
		return nil, fmt.Errorf("pack DappAddress: %w", err)
	}

	offset += n
	n, err = packETHAddressHex(buf[offset:], q.SolverAddress)
	if err != nil {
		return nil, fmt.Errorf("pack SolverAddress: %w", err)
	}

	offset += n
	n, err = packRawBytes(buf[offset:], q.Quote)
	if err != nil {
		return nil, fmt.Errorf("pack Quote: %w", err)
	}

	offset += n
	n, err = packKeccak256Hash(buf[offset:], q.Hash)
	if err != nil {
		return nil, fmt.Errorf("pack Hash: %w", err)
	}

	offset += n
	n, err = packECDSASignature(buf[offset:], q.Signature)
	if err != nil {
		return nil, fmt.Errorf("pack Signature: %w", err)
	}

	offset += n
	if err := checkBuffEnd(&buf, offset); err != nil {
		return nil, err
	}

	return buf, nil
}

// Unpack unpacks bxmessage from raw bytes
func (q *Quote) Unpack(buf []byte, protocol Protocol) error {
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
		return fmt.Errorf("unpack BroadcastHeader: %v", err)
	}

	offset += n
	q.ID, n, err = unpackUUIDv4String(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack ID: %v", err)
	}

	offset += n
	q.DappAddress, n, err = unpackETHAddressHex(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack DappAddress: %v", err)
	}

	offset += n
	q.SolverAddress, n, err = unpackETHAddressHex(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack SolverAddress: %v", err)
	}

	offset += n
	q.Quote, n, err = unpackRawBytes(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack Quote: %v", err)
	}

	offset += n
	q.Hash, n, err = unpackKeccak256Hash(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack Hash: %v", err)
	}

	offset += n
	q.Signature, _, err = unpackECDSASignature(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack Signature: %v", err)
	}

	return nil
}

// GetNetworkNum implements BroadcastMessage interface
func (q *Quote) GetNetworkNum() types.NetworkNum { return types.AllNetworkNum }

func (q *Quote) size() (uint32, error) {
	size, err := calcPackSize(HeaderLen, types.UUIDv4Len, types.ETHAddressLen, types.ETHAddressLen,
		q.Quote, types.Keccak256HashLen, types.ECDSASignatureLen, ControlByteLen)
	if err != nil {
		return 0, err
	}
	return size, nil
}
