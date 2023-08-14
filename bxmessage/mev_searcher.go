package bxmessage

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage/utils"
	"github.com/bloXroute-Labs/gateway/v2/types"
	gwUtils "github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	uuid "github.com/satori/go.uuid"
	"golang.org/x/crypto/sha3"
)

// MEVSearcherAuth alias for map[string]string
type MEVSearcherAuth map[string]string

// MEVSearcherParams alias for json.RawMessage
type MEVSearcherParams = json.RawMessage

// MEVSearcher represents data that we receive from searcher and send to BDN
// Deprecated: gateways should send MEVBundle instead. Will be removed in the future.
type MEVSearcher struct {
	BroadcastHeader

	ID      string `json:"id"`
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`

	auth                 MEVSearcherAuth
	UUID                 string `json:"uuid"`
	Frontrunning         bool   `json:"frontrunning"`
	effectiveGasPriceLen uint16
	EffectiveGasPrice    big.Int `json:"effective_gas_price"`
	coinbaseProfitLen    uint16
	CoinbaseProfit       big.Int `json:"coinbase_profit"`

	Params MEVSearcherParams `json:"params"`
}

// NewMEVSearcher create MEVSearcher
func NewMEVSearcher(mevMinerMethod string, auth MEVSearcherAuth, uuid string, frontrunning bool, effectiveGasPrice, coinbaseProfit big.Int, params MEVSearcherParams) (MEVSearcher, error) {
	if err := checkAuthSize(len(auth)); err != nil {
		return MEVSearcher{}, err
	}

	if len(uuid) != 0 && len(uuid) != 36 {
		return MEVSearcher{}, errors.New("invalid uuid len")
	}

	var effectiveGasPriceLen, coinbaseProfitLen uint16
	if len(effectiveGasPrice.Bytes()) != 0 {
		effectiveGasPriceLen = uint16(len(effectiveGasPrice.Bytes()))
	}

	if len(coinbaseProfit.Bytes()) != 0 {
		coinbaseProfitLen = uint16(len(coinbaseProfit.Bytes()))
	}

	return MEVSearcher{
		Method:               mevMinerMethod,
		auth:                 auth,
		UUID:                 uuid,
		Frontrunning:         frontrunning,
		effectiveGasPriceLen: effectiveGasPriceLen,
		EffectiveGasPrice:    effectiveGasPrice,
		coinbaseProfitLen:    coinbaseProfitLen,
		CoinbaseProfit:       coinbaseProfit,
		Params:               params,
	}, nil
}

// ToMEVBundle convert MEVSearcher to MEVBundle
func (m *MEVSearcher) ToMEVBundle() (*MEVBundle, error) {
	type mevSearcherParams struct {
		Txs               []hexutil.Bytes `json:"txs"`
		UUID              string          `json:"uuid,omitempty"`
		BlockNumber       string          `json:"blockNumber"`
		MinTimestamp      uint64          `json:"minTimestamp"`
		MaxTimestamp      uint64          `json:"maxTimestamp"`
		RevertingTxHashes []common.Hash   `json:"revertingTxHashes"`
	}

	var params []mevSearcherParams
	if err := json.Unmarshal(m.Params, &params); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mevSearcher params: %v", err)
	}

	if len(params) != 1 {
		return nil, fmt.Errorf("received invalid number of mevSearcher params, must be 1 element")
	}

	bundleHash := sha3.NewLegacyKeccak256()
	txs := make([]string, len(params[0].Txs))
	for i := range params[0].Txs {
		txs[i] = params[0].Txs[i].String()

		tx, err := gwUtils.ParseRawTransaction(params[0].Txs[i])
		if err != nil {
			return nil, fmt.Errorf("failed to parse raw transaction: %v", err)
		}

		bundleHash.Write(tx.Hash().Bytes())
	}

	revertingTxHashes := make([]string, len(params[0].RevertingTxHashes))
	for i := range params[0].RevertingTxHashes {
		revertingTxHashes[i] = params[0].RevertingTxHashes[i].String()
	}

	mevBundle, err := NewMEVBundle(
		txs,
		params[0].UUID,
		params[0].BlockNumber,
		int(params[0].MinTimestamp),
		int(params[0].MaxTimestamp),
		revertingTxHashes,
		m.Frontrunning,
		MEVBundleBuilders(m.Auth()),
		fmt.Sprintf("0x%x", bundleHash.Sum(nil)),
		// new fields are not supported for MEVSearcher
		0,
		false,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create mevBundle: %v", err)
	}

	return &mevBundle, nil
}

// SetHash set hash based on MEVSearcher params and auth
func (m *MEVSearcher) SetHash() {
	buf := []byte{}
	for name, auth := range m.auth {
		buf = append(buf, []byte(name+auth)...)
	}
	buf = append(buf, m.Params...)
	buf = append(buf, m.UUID...)
	buf = append(buf, byte(m.effectiveGasPriceLen))
	buf = append(buf, m.EffectiveGasPrice.Bytes()...)
	buf = append(buf, byte(m.coinbaseProfitLen))
	buf = append(buf, m.CoinbaseProfit.Bytes()...)

	if m.Frontrunning {
		buf = append(buf, []uint8{1}...)
	} else {
		buf = append(buf, []uint8{0}...)
	}

	m.hash = utils.DoubleSHA256(buf[:])
}

// Clone create new MEVSearcher entity based on auth
func (m MEVSearcher) Clone(auth MEVSearcherAuth) MEVSearcher {
	return MEVSearcher{
		BroadcastHeader:      m.BroadcastHeader,
		Method:               m.Method,
		auth:                 auth,
		UUID:                 m.UUID,
		Frontrunning:         m.Frontrunning,
		effectiveGasPriceLen: m.effectiveGasPriceLen,
		EffectiveGasPrice:    m.EffectiveGasPrice,
		coinbaseProfitLen:    m.coinbaseProfitLen,
		CoinbaseProfit:       m.CoinbaseProfit,
		Params:               m.Params,
	}
}

// Auth gets the message MEVSearcherAuth
func (m MEVSearcher) Auth() MEVSearcherAuth {
	return m.auth
}

func (m MEVSearcher) size(protocol Protocol) uint32 {
	var size uint32
	for name, authorization := range m.auth {
		size += uint32(types.UInt16Len + len(name) + types.UInt16Len + len(authorization))
	}

	size += types.UInt16Len + uint32(len(m.Method)) + types.UInt8Len + uint32(len(m.Params)) + m.BroadcastHeader.Size()

	switch {
	case protocol < MevSearcherWithUUID:
	case protocol == MevSearcherWithUUID:
		size += uuidSize
	default:
		size += uuidSize + types.UInt8Len + types.UInt16Len + uint32(m.effectiveGasPriceLen) + types.UInt16Len + uint32(m.coinbaseProfitLen)
	}

	return size

}

// Pack serializes a MEVBundle into a buffer for sending
func (m MEVSearcher) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.size(protocol)
	buf := make([]byte, bufLen)
	m.BroadcastHeader.Pack(&buf, MEVSearcherType, protocol)
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

	if protocol >= MevSearcherWithUUID {
		if m.UUID != "" {
			uuidBytes, err := uuid.FromString(m.UUID)
			if err != nil {
				return nil, fmt.Errorf("failed to set mev bundle uuid %v", err)
			}

			copy(buf[offset:], uuidBytes[:])
		}

		offset += uuidSize
	}

	if protocol >= MevMaxProfitBuilder {
		if m.Frontrunning {
			copy(buf[offset:], []uint8{1})
		} else {
			copy(buf[offset:], []uint8{0})
		}
		offset += types.UInt8Len

		binary.LittleEndian.PutUint16(buf[offset:], m.effectiveGasPriceLen)
		offset += types.UInt16Len

		copy(buf[offset:], m.EffectiveGasPrice.Bytes())
		offset += int(m.effectiveGasPriceLen)

		binary.LittleEndian.PutUint16(buf[offset:], m.coinbaseProfitLen)
		offset += types.UInt16Len

		copy(buf[offset:], m.CoinbaseProfit.Bytes())
		offset += int(m.coinbaseProfitLen)
	}

	copy(buf[offset:], m.Params)

	return buf, nil
}

// Unpack deserializes a MEVBundle from a buffer
func (m *MEVSearcher) Unpack(buf []byte, protocol Protocol) error {
	err := m.BroadcastHeader.Unpack(buf, protocol)
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

	if protocol >= MevSearcherWithUUID {
		if err := checkBufSize(&buf, offset, uuidSize); err != nil {
			return err
		}
		if !bytes.Equal(buf[offset:offset+uuidSize], emptyUUID) {
			uuidRaw, err := uuid.FromBytes(buf[offset : offset+uuidSize])
			if err != nil {
				return fmt.Errorf("failed to parse uuid from bytes, %v", err)
			}
			m.UUID = uuidRaw.String()
		}
		offset += uuidSize
	}

	if protocol >= MevMaxProfitBuilder {
		if err := checkBufSize(&buf, offset, types.UInt8Len); err != nil {
			return err
		}
		m.Frontrunning = int(buf[offset]) != 0
		offset += types.UInt8Len

		if err := checkBufSize(&buf, offset, types.UInt16Len); err != nil {
			return err
		}
		m.effectiveGasPriceLen = binary.LittleEndian.Uint16(buf[offset:])
		offset += types.UInt16Len
		if err := checkBufSize(&buf, offset, int(m.effectiveGasPriceLen)); err != nil {
			return err
		}
		m.EffectiveGasPrice.SetBytes(buf[offset : offset+int(m.effectiveGasPriceLen)])
		offset += int(m.effectiveGasPriceLen)

		if err := checkBufSize(&buf, offset, types.UInt16Len); err != nil {
			return err
		}
		m.coinbaseProfitLen = binary.LittleEndian.Uint16(buf[offset:])
		offset += types.UInt16Len
		if err := checkBufSize(&buf, offset, int(m.coinbaseProfitLen)); err != nil {
			return err
		}
		m.CoinbaseProfit.SetBytes(buf[offset : offset+int(m.coinbaseProfitLen)])
		offset += int(m.coinbaseProfitLen)
	}

	payloadOffsetEnd := len(buf) - ControlByteLen
	if err := checkBufSize(&buf, offset, payloadOffsetEnd-offset); err != nil {
		return err
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
