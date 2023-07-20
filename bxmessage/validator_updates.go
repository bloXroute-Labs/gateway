package bxmessage

import (
	"encoding/binary"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/common"
)

// ValidatorUpdates is the bloxroute message struct that carries validator online and offline updates
type ValidatorUpdates struct {
	Header
	networkNum       uint32
	onlineListLength uint16
	onlineList       []string
}

// NewValidatorUpdates constructs a new transaction message, using the provided hash function on the transaction contents to determine the hash
func NewValidatorUpdates(networkNum types.NetworkNum, onlineLength int, onlineList []string) (*ValidatorUpdates, error) {
	if len(onlineList) != onlineLength {
		return nil, fmt.Errorf("input online validator length is %v, however the length of the list is %v", onlineLength, len(onlineList))
	}

	ret := ValidatorUpdates{networkNum: uint32(networkNum), onlineListLength: uint16(onlineLength), onlineList: onlineList}
	return &ret, nil
}

func (vu *ValidatorUpdates) size() uint32 {
	return vu.Header.Size() + uint32(types.UInt32Len+types.UInt16Len+common.AddressLength*vu.onlineListLength)
}

// GetOnlineLength is accessor for online length
func (vu *ValidatorUpdates) GetOnlineLength() int {
	return int(vu.onlineListLength)
}

// GetOnlineList is accessor for online list
func (vu *ValidatorUpdates) GetOnlineList() []string {
	return vu.onlineList
}

// Pack serializes a validator updates into a buffer for sending
func (vu *ValidatorUpdates) Pack(protocol Protocol) ([]byte, error) {
	if len(vu.onlineList) != int(vu.onlineListLength) {
		return nil, fmt.Errorf("input online validator length is %v, however the length of the list is %v", vu.onlineListLength, len(vu.onlineList))
	}

	listBytes := make([]byte, common.AddressLength*vu.onlineListLength)
	for index, validator := range vu.onlineList {
		addr := common.HexToAddress(validator)
		copy(listBytes[index*common.AddressLength:(index+1)*common.AddressLength], addr[:])
	}

	bufLen := vu.size()
	buf := make([]byte, bufLen)
	vu.Header.Pack(&buf, ValidatorUpdatesType)
	offset := HeaderLen
	binary.LittleEndian.PutUint32(buf[offset:], vu.networkNum)
	offset += types.UInt32Len
	binary.LittleEndian.PutUint16(buf[offset:], vu.onlineListLength)
	offset += types.UInt16Len
	copy(buf[offset:offset+len(vu.onlineList)*common.AddressLength], listBytes)

	return buf, nil
}

// Unpack deserializes a validator updates message and save it in
func (vu *ValidatorUpdates) Unpack(buf []byte, protocol Protocol) error {
	if err := checkBufSize(&buf, 0, HeaderLen); err != nil {
		return err
	}
	offset := HeaderLen

	if err := checkBufSize(&buf, offset, types.UInt32Len); err != nil {
		return err
	}
	vu.networkNum = binary.LittleEndian.Uint32(buf[offset:])
	offset += types.UInt32Len

	if err := checkBufSize(&buf, offset, types.UInt16Len); err != nil {
		return err
	}
	vu.onlineListLength = binary.LittleEndian.Uint16(buf[offset : offset+2])
	offset += types.UInt16Len

	if err := checkBufSize(&buf, offset, int(vu.onlineListLength*common.AddressLength)); err != nil {
		return err
	}
	validatorList := make([]string, vu.onlineListLength)
	for index := 0; index < int(vu.onlineListLength); index++ {
		addrBytes := buf[offset+index*common.AddressLength : offset+(index+1)*common.AddressLength]
		validatorList = append(validatorList, common.BytesToAddress(addrBytes).String())
	}
	vu.onlineList = validatorList

	return vu.Header.Unpack(buf, protocol)
}

// String returns the string representation of the validator update messages
func (vu *ValidatorUpdates) String() string {
	return fmt.Sprintf("validator udpates message for network number %v, %v online validators", vu.networkNum, vu.onlineListLength)
}

// GetNetworkNum return network number of the validator update message
func (vu *ValidatorUpdates) GetNetworkNum() types.NetworkNum {
	return types.NetworkNum(vu.networkNum)
}
