package bxmessage

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/bloXroute-Labs/gateway/bxmessage/utils"
	"github.com/bloXroute-Labs/gateway/types"
)

const maxAuthNames = 255

// MEVSearcherAuth alias for map[string]string
type MEVSearcherAuth map[string]string

// MEVSearcherParams alias for json.RawMessage
// TODO: think about implement SendBundleArgs flashbot struct instead of json.RawMessage
type MEVSearcherParams = json.RawMessage

// MEVSearcher represents data that we receive from searcher and send to BDN
type MEVSearcher struct {
	BroadcastHeader

	ID      string `json:"id"`
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`

	auth   MEVSearcherAuth
	Params MEVSearcherParams `json:"params"`
}

// NewMEVSearcher create MEVSearcher
func NewMEVSearcher(mevMinerMethod string, auth MEVSearcherAuth, params MEVSearcherParams) (MEVSearcher, error) {
	if err := checkAuthSize(len(auth)); err != nil {
		return MEVSearcher{}, err
	}
	return MEVSearcher{
		Method: mevMinerMethod,
		auth:   auth,
		Params: params,
	}, nil
}

// SetHash set hash based on MEVSearcher params and auth
func (m *MEVSearcher) SetHash() {
	buf := []byte{}
	for name, auth := range m.auth {
		buf = append(buf, []byte(name+auth)...)
	}
	buf = append(buf, m.Params...)
	m.hash = utils.DoubleSHA256(buf[:])
}

// Clone create new MEVSearcher entity based on auth
func (m MEVSearcher) Clone(auth MEVSearcherAuth) MEVSearcher {
	return MEVSearcher{
		BroadcastHeader: m.BroadcastHeader,
		Method:          m.Method,
		auth:            auth,
		Params:          m.Params,
	}
}

// Auth gets the message MEVSearcherAuth
func (m MEVSearcher) Auth() MEVSearcherAuth {
	return m.auth
}

func (m MEVSearcher) size() uint32 {
	var size uint32
	for name, authorization := range m.auth {
		size += uint32(types.UInt16Len + len(name) + types.UInt16Len + len(authorization))
	}

	return size + types.UInt16Len + uint32(len(m.Method)) + types.UInt8Len + uint32(len(m.Params)) + m.BroadcastHeader.Size()
}

// Pack serializes a MEVBundle into a buffer for sending
func (m MEVSearcher) Pack(_ Protocol) ([]byte, error) {
	bufLen := m.size()
	buf := make([]byte, bufLen)
	m.BroadcastHeader.Pack(&buf, MEVSearcherType)
	offset := BroadcastHeaderLen

	binary.LittleEndian.PutUint16(buf[offset:], uint16(len(m.Method)))
	offset += types.UInt16Len

	copy(buf[offset:], m.Method)
	offset += len(m.Method)

	if err := checkAuthSize(len(m.auth)); err != nil {
		return nil, err
	}
	mevMiners := make([]uint8, 1)
	mevMiners[0] = byte(len(m.auth))
	copy(buf[offset:], mevMiners)
	offset++

	for name, auth := range m.auth {
		nameLength := len(name)
		authorizationLength := len(auth)
		binary.LittleEndian.PutUint16(buf[offset:], uint16(nameLength))
		offset += types.UInt16Len
		copy(buf[offset:], name)
		offset += nameLength

		binary.LittleEndian.PutUint16(buf[offset:], uint16(authorizationLength))
		offset += types.UInt16Len
		copy(buf[offset:], auth)
		offset += authorizationLength
	}

	copy(buf[offset:], m.Params)

	return buf, nil
}

// Unpack deserializes a MEVBundle from a buffer
func (m *MEVSearcher) Unpack(buf []byte, _ Protocol) error {
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
	mevSearchers := buf[offset]
	offset++

	m.auth = MEVSearcherAuth{}
	for i := 0; i < int(mevSearchers); i++ {
		if err := checkBufSize(&buf, offset, types.UInt16Len); err != nil {
			return err
		}
		nameLength := binary.LittleEndian.Uint16(buf[offset:])
		offset += types.UInt16Len

		if err := checkBufSize(&buf, offset, int(nameLength)); err != nil {
			return err
		}
		name := string(buf[offset : offset+int(nameLength)])
		offset += int(nameLength)

		if err := checkBufSize(&buf, offset, types.UInt16Len); err != nil {
			return err
		}
		authLength := binary.LittleEndian.Uint16(buf[offset:])
		offset += types.UInt16Len

		if err := checkBufSize(&buf, offset, int(authLength)); err != nil {
			return err
		}
		auth := string(buf[offset : offset+int(authLength)])
		offset += int(authLength)

		m.auth[name] = auth
	}

	m.Params = buf[offset : len(buf)-ControlByteLen]

	return nil
}

func checkAuthSize(authSize int) error {
	if authSize == 0 {
		return fmt.Errorf("at least 1 mev builder must be present")
	}

	if authSize > maxAuthNames {
		return fmt.Errorf("number of mev builders names %v exceeded the limit (%v)", authSize, maxAuthNames)
	}

	return nil
}
