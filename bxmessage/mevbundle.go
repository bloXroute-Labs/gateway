package bxmessage

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/bloXroute-Labs/gateway/bxmessage/utils"
	"github.com/bloXroute-Labs/gateway/types"
)

const mevBundleNameMaxSize = 255

// MEVMinerNames alias for []string
type MEVMinerNames = []string

// MEVBundleParams alias for json.RawMessage
type MEVBundleParams = json.RawMessage

// MEVBundle represents data that we receive from MEV-miners and send to BDN
type MEVBundle struct {
	BroadcastHeader

	ID      string `json:"id"`
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`

	names  MEVMinerNames
	Params MEVBundleParams `json:"params"`
}

// NewMEVBundle create MEVBundle
func NewMEVBundle(mevMinerMethod string, mevMinerNames MEVMinerNames, mevBundleParams MEVBundleParams) (MEVBundle, error) {
	if err := checkMEVBundleNameSize(len(mevMinerNames)); err != nil {
		return MEVBundle{}, err
	}
	return MEVBundle{
		Method: mevMinerMethod,
		names:  mevMinerNames,
		Params: mevBundleParams,
	}, nil
}

// Names gets the message MEVMinerNames
func (m MEVBundle) Names() MEVMinerNames {
	return m.names
}

// SetHash set hash based in params
func (m *MEVBundle) SetHash() {
	buf := []byte{}
	for _, name := range m.names {
		buf = append(buf, []byte(name)...)
	}
	buf = append(buf, m.Params...)
	m.hash = utils.DoubleSHA256(buf[:])
}

// SetNames set names
func (m *MEVBundle) SetNames(names MEVMinerNames) {
	m.names = names
}

// Clone create new MEVMinerNames entity based on names
func (m MEVBundle) Clone(names MEVMinerNames) MEVBundle {
	return MEVBundle{
		BroadcastHeader: m.BroadcastHeader,
		Method:          m.Method,
		names:           names,
		Params:          m.Params,
	}
}

func (m MEVBundle) size() uint32 {
	var size uint32
	for _, name := range m.names {
		size += uint32(types.UInt16Len + len(name))
	}
	return size + types.UInt16Len + uint32(len(m.Method)) + types.UInt8Len + uint32(len(m.Params)) + m.BroadcastHeader.Size()
}

// Pack serializes a MEVBundle into a buffer for sending
func (m MEVBundle) Pack(_ Protocol) ([]byte, error) {
	bufLen := m.size()
	buf := make([]byte, bufLen)
	m.BroadcastHeader.Pack(&buf, MEVBundleType)
	offset := BroadcastHeaderLen

	binary.LittleEndian.PutUint16(buf[offset:], uint16(len(m.Method)))
	offset += types.UInt16Len

	copy(buf[offset:], m.Method)
	offset += len(m.Method)

	if err := checkMEVBundleNameSize(len(m.names)); err != nil {
		return nil, err
	}
	mevMiners := make([]byte, 1)
	mevMiners[0] = byte(len(m.names))
	copy(buf[offset:], mevMiners)
	offset++

	for _, name := range m.names {
		nameLength := len(name)
		binary.LittleEndian.PutUint16(buf[offset:], uint16(nameLength))
		offset += types.UInt16Len
		copy(buf[offset:], name)
		offset += nameLength
	}

	copy(buf[offset:], m.Params)

	return buf, nil
}

// Unpack deserializes a MEVBundle from a buffer
func (m *MEVBundle) Unpack(buf []byte, _ Protocol) error {
	err := m.BroadcastHeader.Unpack(buf, 0)
	if err != nil {
		return err
	}
	offset := BroadcastHeaderLen

	if err := checkBufSize(&buf, offset, types.UInt16Len); err != nil {
		return err
	}
	mevMinerMethodLen := binary.LittleEndian.Uint16(buf[offset:])
	offset += types.UInt16Len

	if err := checkBufSize(&buf, offset, int(mevMinerMethodLen)); err != nil {
		return err
	}
	m.Method = string(buf[offset : offset+int(mevMinerMethodLen)])
	offset += len(m.Method)

	if err := checkBufSize(&buf, offset, types.UInt8Len); err != nil {
		return err
	}
	mevMinersLength := buf[offset : offset+types.UInt8Len][0]
	offset++

	m.names = MEVMinerNames{}

	for i := 0; i < int(mevMinersLength); i++ {
		if err := checkBufSize(&buf, offset, types.UInt16Len); err != nil {
			return err
		}
		mevMinerNameLength := binary.LittleEndian.Uint16(buf[offset:])
		offset += types.UInt16Len

		if err := checkBufSize(&buf, offset, int(mevMinerNameLength)); err != nil {
			return err
		}
		mevMinerName := string(buf[offset : offset+int(mevMinerNameLength)])
		offset += int(mevMinerNameLength)
		m.names = append(m.names, mevMinerName)
	}

	payloadOffsetEnd := len(buf) - ControlByteLen
	if err := checkBufSize(&buf, offset, payloadOffsetEnd-offset); err != nil {
		return err
	}
	m.Params = buf[offset:payloadOffsetEnd]

	return nil
}

func checkMEVBundleNameSize(namesSize int) error {
	if namesSize > mevBundleNameMaxSize {
		return fmt.Errorf("number of mev builders names %v exceeded the limit (%v)", namesSize, mevBundleNameMaxSize)
	}

	return nil
}
