package bxmessage

import (
	"fmt"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// UnpackIntentSolution unpacks intent solution bxmessage from bytes
func UnpackIntentSolution(b []byte, protocol Protocol) (*IntentSolution, error) {
	solution := new(IntentSolution)
	err := solution.Unpack(b, protocol)
	if err != nil {
		return nil, err
	}

	return solution, nil
}

// NewIntentSolution constructor for IntentSolution bxmessage
func NewIntentSolution(id, solverAddr, intentID string, hash, signature []byte, timestamp time.Time, solution []byte) Message {
	return &IntentSolution{
		Header:        Header{msgType: IntentSolutionType},
		ID:            id,
		SolverAddress: solverAddr,
		IntentID:      intentID,
		Solution:      solution,
		Hash:          hash,
		Signature:     signature,
		Timestamp:     timestamp,
	}
}

// IntentSolution bxmessage
type IntentSolution struct {
	Header
	ID            string    // UUIDv4
	SolverAddress string    // ETH Address
	IntentID      string    // UUIDv4
	Solution      []byte    // Variable length
	Hash          []byte    // Keccak256
	Signature     []byte    // ECDSA Signature
	Timestamp     time.Time // Short timestamp
	DappAddress   string    // ETH Address -> set up by relay
	SenderAddress string    // ETH Address -> set up by relay
}

// GetNetworkNum implements BroadcastMessage interface
func (i *IntentSolution) GetNetworkNum() types.NetworkNum { return types.AllNetworkNum }

// Pack packs IntentSolution into raw bytes for broadcasting
func (i *IntentSolution) Pack(protocol Protocol) ([]byte, error) {
	if protocol < IntentsProtocol {
		return nil, fmt.Errorf("invalid protocol version for IntentSolution message: %v", protocol)
	}

	bufLen, err := i.size(protocol)
	if err != nil {
		return nil, fmt.Errorf("calc pack size: %w", err)
	}

	offset := 0
	buf := make([]byte, bufLen)

	n, err := packHeader(buf, i.Header, IntentSolutionType)
	if err != nil {
		return nil, fmt.Errorf("pack Header: %w", err)
	}

	offset += n
	n, err = packUUIDv4String(buf[offset:], i.ID)
	if err != nil {
		return nil, fmt.Errorf("pack ID: %w", err)
	}

	offset += n
	n, err = packETHAddressHex(buf[offset:], i.SolverAddress)
	if err != nil {
		return nil, fmt.Errorf("pack SolverAddress: %w", err)
	}

	offset += n
	n, err = packUUIDv4String(buf[offset:], i.IntentID)
	if err != nil {
		return nil, fmt.Errorf("pack IntentID: %w", err)
	}

	offset += n
	n, err = packRawBytes(buf[offset:], i.Solution)
	if err != nil {
		return nil, fmt.Errorf("pack Solution: %w", err)
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

	if protocol >= IntentSolutionProtocol { // DappAddress
		offset += n
		n, err = packETHAddressHex(buf[offset:], i.DappAddress)
		if err != nil {
			return nil, fmt.Errorf("pack DappAddress: %w", err)
		}
	}

	if protocol >= SolutionSenderAddressProtocol { // SenderAddress
		offset += n
		n, err = packETHAddressHex(buf[offset:], i.SenderAddress)
		if err != nil {
			return nil, fmt.Errorf("pack SenderAddress: %w", err)
		}
	}

	offset += n
	if err := checkBuffEnd(&buf, offset); err != nil {
		return nil, err
	}

	return buf, nil
}

// Unpack unpacks bxmessage from raw bytes
func (i *IntentSolution) Unpack(buf []byte, protocol Protocol) error {
	var (
		err    error
		n      int
		offset = 0
	)

	if protocol < IntentsProtocol {
		return fmt.Errorf("invalid protocol version for IntentSolution message: %v", protocol)
	}

	i.Header, n, err = unpackHeader(buf, protocol)
	if err != nil {
		return fmt.Errorf("unpack BroadcastHeader: %v", err)
	}

	offset += n
	i.ID, n, err = unpackUUIDv4String(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack ID: %v", err)
	}

	offset += n
	i.SolverAddress, n, err = unpackETHAddressHex(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack SolverAddress: %v", err)
	}

	offset += n
	i.IntentID, n, err = unpackUUIDv4String(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack IntentID: %v", err)
	}

	offset += n
	i.Solution, n, err = unpackRawBytes(buf[offset:])
	if err != nil {
		return fmt.Errorf("unpack IntentSolution: %v", err)
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

	if protocol >= IntentSolutionProtocol { // DappAddress
		offset += n
		i.DappAddress, n, err = unpackETHAddressHex(buf[offset:])
		if err != nil {
			return fmt.Errorf("unpack DappAddress: %v", err)
		}
	}

	if protocol >= SolutionSenderAddressProtocol { // SenderAddress
		offset += n
		i.SenderAddress, _, err = unpackETHAddressHex(buf[offset:])
		if err != nil {
			return fmt.Errorf("unpack SenderAddress: %v", err)
		}
	}

	return nil
}

func (i *IntentSolution) size(protocol Protocol) (uint32, error) {
	size, err := calcPackSize(
		HeaderLen,
		UUIDv4Len,
		ETHAddressLen,
		UUIDv4Len,
		i.Solution,
		Keccak256HashLen,
		ECDSASignatureLen,
		ShortTimestampLen,
		ControlByteLen,
	)
	if err != nil {
		return 0, err
	}

	if protocol >= IntentSolutionProtocol {
		size += ETHAddressLen // DappAddress
	}

	if protocol >= SolutionSenderAddressProtocol {
		size += ETHAddressLen // SenderAddress
	}

	return size, nil
}
