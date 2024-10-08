package bxmessage

import (
	"encoding/binary"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage/utils"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// GetTxs - represent the "gettxs" message
type GetTxs struct {
	Header
	ShortIDs types.ShortIDList
	Hash     types.SHA256Hash // Hash is not a part of the buffer
}

// Pack serializes a GetTxs into a buffer for sending
func (getTxs *GetTxs) Pack(protocol Protocol) ([]byte, error) {
	bufLen := getTxs.size()
	buf := make([]byte, bufLen)
	offset := uint32(HeaderLen)
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(getTxs.ShortIDs)))
	offset += types.UInt32Len
	for _, shortID := range getTxs.ShortIDs {
		binary.LittleEndian.PutUint32(buf[offset:], uint32(shortID))
		offset += types.UInt32Len
	}

	if err := checkBuffEnd(&buf, int(offset)); err != nil {
		return nil, err
	}

	getTxs.Header.Pack(&buf, GetTransactionsType)
	return buf, nil
}

// Unpack deserializes a GetTxs from a buffer
func (getTxs *GetTxs) Unpack(buf []byte, protocol Protocol) error {
	getTxs.Hash = utils.DoubleSHA256(buf[:])
	offset := HeaderLen

	if err := checkBufSize(&buf, offset, types.UInt32Len); err != nil {
		return err
	}
	shortIDs := binary.LittleEndian.Uint32(buf[offset:])
	offset += types.UInt32Len
	for i := 0; i < int(shortIDs); i++ {
		if err := checkBufSize(&buf, offset, types.UInt32Len); err != nil {
			return err
		}
		shortID := binary.LittleEndian.Uint32(buf[offset:])
		getTxs.ShortIDs = append(getTxs.ShortIDs, types.ShortID(shortID))
		offset += types.UInt32Len
	}
	return getTxs.Header.Unpack(buf, protocol)
}

func (getTxs *GetTxs) size() uint32 {
	return getTxs.Header.Size() + uint32(types.UInt32Len+len(getTxs.ShortIDs)*types.UInt32Len)
}
