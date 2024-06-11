package bxmessage

import (
	"fmt"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// UnpackIntent unpacks intent bxmessage
func UnpackIntent(b []byte, protocol Protocol) (*Intent, error) {
	intent := new(Intent)
	err := intent.Unpack(b, protocol)
	if err != nil {
		return nil, err
	}

	return intent, nil
}

// NewIntent constructor for Intent bxmessage
func NewIntent(id string, dAppAddr, senderAddress string, hash, signature []byte, timestamp time.Time, intent []byte) Message {
	return &Intent{
		Header:        Header{},
		ID:            id,
		DAppAddress:   dAppAddr,
		SenderAddress: senderAddress,
		Hash:          hash,
		Signature:     signature,
		Timestamp:     timestamp,
		Intent:        intent,
	}
}

// Intent bxmessage
type Intent struct {
	Header
	ID            string    // UUID
	DAppAddress   string    // ETH Address
	SenderAddress string    // ETH Address
	Hash          []byte    // Keccak256
	Signature     []byte    // ECDSA signature
	Timestamp     time.Time // Short timestamp
	Intent        []byte    // Variable length
}

// GetNetworkNum implements BroadcastMessage interface
func (i *Intent) GetNetworkNum() types.NetworkNum { return types.AllNetworkNum }

// Pack packs intent into bytes for broadcasting
func (i *Intent) Pack(protocol Protocol) ([]byte, error) {
	if protocol < IntentsProtocol {
		return nil, fmt.Errorf("invalid protocol version for Intent message: %v", protocol)
	}

	bufLen, err := calcPackSize(
		HeaderLen,
		UUIDv4Len,
		ETHAddressLen,
		ETHAddressLen,
		Keccak256HashLen,
		ECDSASignatureLen,
		ShortTimestampLen,
		i.Intent,
		ControlByteLen,
	)
	if err != nil {
		return nil, fmt.Errorf("calc pack size: %w", err)
	}

	var (
		offset = 0
		buf    = make([]byte, bufLen)
	)

	n, err := packHeader(buf, i.Header, IntentType)
	if err != nil {
		return nil, fmt.Errorf("pack Header: %w", err)
	}

	offset += n
	n, err = packUUIDv4String(buf[offset:], i.ID)
	if err != nil {
		return nil, fmt.Errorf("pack ID: %w", err)
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
	n, err = packECDSASignature(buf[offset:], i.Signature)
	if err != nil {
		return nil, fmt.Errorf("pack Signature: %w", err)
	}

	offset += n
	n, err = packTimestamp(buf[offset:], i.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("pack Timestamp: %w", err)
	}

	offset += n
	n, err = packRawBytes(buf[offset:], i.Intent)
	if err != nil {
		return nil, fmt.Errorf("pack Intent: %v", err)
	}

	if protocol >= IntentsWithAnySenderProtocol {
		offset += n
		n, err = packETHAddressHex(buf[offset:], i.SenderAddress)
		if err != nil {
			return nil, fmt.Errorf("pack SenderAddress: %v", err)
		}
	}

	offset += n
	if err := checkBuffEnd(&buf, offset); err != nil {
		return nil, err
	}

	return buf, nil
}

// Unpack unpacks raw bytes from the network into Intent bxmessage
func (i *Intent) Unpack(buf []byte, protocol Protocol) error {
	var (
		err    error
		offset int
		n      int
	)

	if protocol < IntentsProtocol {
		return fmt.Errorf("invalid protocol version for Intent message: %v", protocol)
	}

	i.Header, n, err = unpackHeader(buf, protocol)
	if err != nil {
		return fmt.Errorf("unpack Header: %v", err)
	}

	offset += n
	i.ID, n, err = unpackUUIDv4String(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack ID: %v", err)
	}

	offset += n
	i.DAppAddress, n, err = unpackETHAddressHex(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack DAppAddress: %v", err)
	}

	offset += n
	i.Hash, n, err = unpackKeccak256Hash(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack Hash: %v", err)
	}

	offset += n
	i.Signature, n, err = unpackECDSASignature(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack Signature: %v", err)
	}

	offset += n
	i.Timestamp, n, err = unpackTimestamp(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack Timestamp: %v", err)
	}

	offset += n
	i.Intent, n, err = unpackRawBytes(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack Intent: %v", err)
	}

	if protocol < IntentsWithAnySenderProtocol {
		// In older version of the protocol, the intent sender was always the DApp address
		i.SenderAddress = i.DAppAddress
	} else {
		offset += n
		i.SenderAddress, n, err = unpackETHAddressHex(buf[offset:])
		if err != nil {
			return fmt.Errorf("unpack SenderAddress: %v", err)
		}
	}

	return nil
}
