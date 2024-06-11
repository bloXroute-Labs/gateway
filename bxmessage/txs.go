package bxmessage

import (
	"encoding/binary"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// TxsItem represents simplified data about a given requested transaction
type TxsItem struct {
	Hash    types.SHA256Hash
	Content types.TxContent
	ShortID types.ShortID
}

// Txs represents the response to GetTxs from relay proxies
type Txs struct {
	Header
	items []TxsItem
}

// NewTxs returns a new Txs message for packing
func NewTxs(items []TxsItem) *Txs {
	return &Txs{
		items: items,
	}
}

// Items returns all the requested transaction info
func (m *Txs) Items() []TxsItem {
	return m.items
}

func (m *Txs) size() uint32 {
	itemsSize := 0
	for _, item := range m.items {
		itemsSize += types.SHA256HashLen + types.ShortIDLen + types.UInt32Len + len(item.Content)
	}
	return m.Header.Size() + uint32(types.UInt32Len+itemsSize)
}

// Pack returns the txs encoded into the byte protocol
func (m Txs) Pack(protocol Protocol) ([]byte, error) {
	buf := make([]byte, m.size())

	offset := HeaderLen
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(m.items)))
	offset += types.UInt32Len

	for _, item := range m.items {
		binary.LittleEndian.PutUint32(buf[offset:], uint32(item.ShortID))
		offset += types.ShortIDLen

		copy(buf[offset:], item.Hash[:])
		offset += types.SHA256HashLen

		binary.LittleEndian.PutUint32(buf[offset:], uint32(len(item.Content)))
		offset += types.UInt32Len

		copy(buf[offset:], item.Content[:])
		offset += len(item.Content)
	}

	if err := checkBuffEnd(&buf, offset); err != nil {
		return nil, err
	}

	m.Header.Pack(&buf, TransactionsType)
	return buf, nil
}

// Unpack decodes a Txs message from the serialized byte protoool
func (m *Txs) Unpack(buf []byte, protocol Protocol) error {
	offset := HeaderLen

	itemCount := int(binary.LittleEndian.Uint32(buf[offset:]))
	offset += types.UInt32Len

	items := make([]TxsItem, 0)
	for i := 0; i < itemCount; i++ {
		if err := checkBufSize(&buf, offset, types.ShortIDLen+types.SHA256HashLen+types.UInt32Len); err != nil {
			return err
		}

		item := TxsItem{}

		item.ShortID = types.ShortID(binary.LittleEndian.Uint32(buf[offset:]))
		offset += types.ShortIDLen

		item.Hash, _ = types.NewSHA256Hash(buf[offset : offset+types.SHA256HashLen])
		offset += types.SHA256HashLen

		contentLength := int(binary.LittleEndian.Uint32(buf[offset:]))
		offset += types.UInt32Len

		if err := checkBufSize(&buf, offset, contentLength); err != nil {
			return err
		}
		item.Content = buf[offset : offset+contentLength]
		offset += contentLength

		items = append(items, item)
	}
	m.items = items

	return m.Header.Unpack(buf, protocol)
}
