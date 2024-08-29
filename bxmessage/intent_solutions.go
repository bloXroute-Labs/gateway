package bxmessage

import (
	"encoding/binary"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// UnpackIntentSolutions unpacks a GetIntentSolutions message from a buffer
func UnpackIntentSolutions(b []byte, protocol Protocol) (*IntentSolutions, error) {
	s := new(IntentSolutions)
	err := s.Unpack(b, protocol)
	if err != nil {
		return nil, err
	}

	return s, nil
}

// IntentSolutions represents the response to GetIntentSolutions from relay proxies
type IntentSolutions struct {
	Header
	solutions []IntentSolution
}

// NewIntentSolutions returns a new IntentSolutions message for packing
func NewIntentSolutions(solutions []IntentSolution) *IntentSolutions {
	return &IntentSolutions{
		Header:    Header{msgType: IntentSolutionsType},
		solutions: solutions,
	}
}

// Solutions returns all the requested intent solutions
func (s *IntentSolutions) Solutions() []IntentSolution {
	return s.solutions
}

// Pack packs IntentSolutions into raw bytes for broadcasting
func (s *IntentSolutions) Pack(protocol Protocol) ([]byte, error) {
	if protocol < IntentsProtocol {
		return nil, fmt.Errorf("invalid protocol version for IntentSolutions message: %v", protocol)
	}

	bufLen, err := s.size(protocol)
	if err != nil {
		return nil, fmt.Errorf("calc pack size: %w", err)
	}

	offset := 0
	buf := make([]byte, bufLen)

	n, err := packHeader(buf, s.Header, IntentSolutionsType)
	if err != nil {
		return nil, fmt.Errorf("pack Header: %w", err)
	}

	offset += n

	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(s.solutions)))
	offset += types.UInt32Len

	for _, solution := range s.solutions {
		packedSolution, err := solution.Pack(protocol)
		if err != nil {
			return nil, fmt.Errorf("pack solution: %w", err)
		}

		copy(buf[offset:], packedSolution)
		offset += len(packedSolution)
	}

	if err = checkBuffEnd(&buf, offset); err != nil {
		return nil, err
	}

	return buf, nil
}

// Unpack deserializes a IntentSolutions from a buffer
func (s *IntentSolutions) Unpack(data []byte, protocol Protocol) error {
	if protocol < IntentsProtocol {
		return fmt.Errorf("invalid protocol version for IntentSolutions message: %v", protocol)
	}

	var (
		err    error
		n      int
		offset = 0
	)

	s.Header, n, err = unpackHeader(data, protocol)
	if err != nil {
		return fmt.Errorf("unpack BroadcastHeader: %v", err)
	}

	offset += n

	solutionsLen := binary.LittleEndian.Uint32(data[offset:])
	offset += types.UInt32Len

	s.solutions = make([]IntentSolution, solutionsLen)

	for i := 0; i < int(solutionsLen); i++ {
		solution := new(IntentSolution)
		err = solution.Unpack(data[offset:], protocol)
		if err != nil {
			return fmt.Errorf("unpack solution: %w", err)
		}

		solSize, err := solution.size(protocol)
		if err != nil {
			return fmt.Errorf("calc solution size: %w", err)
		}

		offset += int(solSize)
		s.solutions[i] = *solution
	}

	return nil
}

// GetNetworkNum implements BroadcastMessage interface
func (s *IntentSolutions) GetNetworkNum() types.NetworkNum { return types.AllNetworkNum }

func (s *IntentSolutions) size(protocol Protocol) (uint32, error) {
	size := uint32(0)

	for _, solution := range s.solutions {
		solutionSize, err := solution.size(protocol)
		if err != nil {
			return 0, fmt.Errorf("calc solution size: %w", err)
		}

		size += solutionSize
	}

	return s.Header.Size() + types.UInt32Len + size, nil
}
