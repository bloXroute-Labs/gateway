package bxmessage

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage/utils"
	"github.com/bloXroute-Labs/gateway/v2/types"
	uuid "github.com/satori/go.uuid"
)

const (
	maxAuthNames = 255
	uuidSize     = 16
)

var emptyUUID = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

// MEVBundleBuilders alias for map[string]string
type MEVBundleBuilders map[string]string

// MEVBundle represents data that we receive from searcher and propagate via BDN
type MEVBundle struct {
	BroadcastHeader

	ID      string `json:"id"`
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`

	UUID            string   `json:"uuid,omitempty"`
	Transactions    []string `json:"transaction,omitempty"`
	BlockNumber     string   `json:"blockNumber,omitempty"`
	MinTimestamp    int      `json:"minTimestamp,omitempty"`
	MaxTimestamp    int      `json:"maxTimestamp,omitempty"`
	RevertingHashes []string `json:"revertingTxHashes,omitempty"`
	Frontrunning    bool     `json:"frontrunning,omitempty"`

	// used for tracking performance (gateway to receiving relay, followed by
	// overriding in relay handler (after logging),
	// and relay receiving req from other relay -> to another gateway
	PerformanceTimestamp time.Time `json:"performanceTimestamp,omitempty"`
	BundleHash           string    `json:"bundleHash"`

	MEVBuilders MEVBundleBuilders `json:"mev_builders,omitempty"`
}

// NewMEVBundle creates a new MEVBundle
func NewMEVBundle(transaction []string, uuid string, blockNumber string, minTimestamp int, maxTimestamp int, revertingHashes []string, frontrunning bool, mevBuilders MEVBundleBuilders, bundleHash string) (MEVBundle, error) {
	if err := checkBuilderSize(len(mevBuilders)); err != nil {
		return MEVBundle{}, err
	}

	if len(uuid) != 0 && len(uuid) != 36 {
		return MEVBundle{}, errors.New("invalid uuid len")
	}
	return MEVBundle{
		Transactions:    transaction,
		UUID:            uuid,
		BlockNumber:     blockNumber,
		MinTimestamp:    minTimestamp,
		MaxTimestamp:    maxTimestamp,
		RevertingHashes: revertingHashes,
		Frontrunning:    frontrunning,
		MEVBuilders:     mevBuilders,
		BundleHash:      bundleHash,
	}, nil
}

// SetHash sets the hash based on the fields in BundleSubmission
func (m *MEVBundle) SetHash() {
	buf := []byte{}

	buf = append(buf, m.UUID...)
	for _, tx := range m.Transactions {
		buf = append(buf, []byte(tx)...)
	}
	buf = append(buf, []byte(m.BlockNumber)...)

	minTimestampBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(minTimestampBytes, uint64(m.MinTimestamp))
	buf = append(buf, minTimestampBytes...)

	maxTimestampBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(maxTimestampBytes, uint64(m.MaxTimestamp))
	buf = append(buf, maxTimestampBytes...)

	for _, hash := range m.RevertingHashes {
		buf = append(buf, []byte(hash)...)
	}
	if m.Frontrunning {
		buf = append(buf, []uint8{1}...)
	} else {
		buf = append(buf, []uint8{0}...)
	}
	for name, auth := range m.MEVBuilders {
		buf = append(buf, []byte(name+auth)...)
	}

	m.hash = utils.DoubleSHA256(buf[:])
}

func (m MEVBundle) size() uint32 {
	var size uint32

	size += m.BroadcastHeader.Size() +
		types.UInt16Len + uint32(len(m.Method)) + // Method length (2 bytes) + method
		uuidSize + // UUID
		types.UInt64Len + // BlockNumber length (8 bytes) + blockNumber
		types.UInt64Len + // MinTimestamp (4 bytes) + MaxTimestamp (4 bytes)
		types.UInt16Len + // RevertingHashes count (2 bytes)
		types.UInt8Len + // Frontrunning (1 byte)
		TimestampLen +
		types.SHA256HashLen

	// Transactions
	for _, tx := range m.Transactions {
		size += types.UInt16Len + uint32(len(tx)) // Length (2 bytes) + transaction
	}

	// RevertingHashes
	for _, hash := range m.RevertingHashes {
		size += types.UInt16Len + uint32(len(hash)) // Length (2 bytes) + hash
	}

	// MEVBuilders
	size += types.UInt32Len // MEVBuilders count (4 bytes)

	for name, authorization := range m.MEVBuilders {
		size += uint32(types.UInt16Len + len(name) + types.UInt16Len + len(authorization))
	}

	return size
}

// Clone create new MEVSearcher entity based on auth
func (m MEVBundle) Clone(auth MEVBundleBuilders) MEVBundle {
	clone := MEVBundle{}

	clone.JSONRPC = m.JSONRPC
	clone.Method = m.Method
	clone.UUID = m.UUID
	clone.BlockNumber = m.BlockNumber
	clone.MinTimestamp = m.MinTimestamp
	clone.MaxTimestamp = m.MaxTimestamp
	clone.Frontrunning = m.Frontrunning
	clone.MEVBuilders = auth
	clone.BundleHash = m.BundleHash
	clone.BroadcastHeader = m.BroadcastHeader

	clone.Transactions = make([]string, len(m.Transactions))
	copy(clone.Transactions, m.Transactions)

	clone.RevertingHashes = make([]string, len(m.RevertingHashes))
	copy(clone.RevertingHashes, m.RevertingHashes)

	return clone
}

// Names Returns a slice of builder names
func (m MEVBundle) Names() []string {
	builderNames := make([]string, 0, len(m.MEVBuilders))

	for builder := range m.MEVBuilders {
		builderNames = append(builderNames, builder)
	}
	return builderNames
}

// Pack Serializes an MEV bundle to buffer for sending over wire
func (m MEVBundle) Pack(protocol Protocol) ([]byte, error) {
	if protocol < BundlesOverBDNProtocol {
		return nil, fmt.Errorf("MEVBundle should not pack from lower protocol %v", protocol)
	}

	// Calculate the total size of the buffer
	//bufLen := BroadcastHeaderLen +
	//	4 + len(m.Method) + // Method length (4 bytes) + method
	//	16 + // UUID
	//	4 + // Transactions count (4 bytes)
	//	len(m.Transactions)*4 + // Total length of all transactions (4 bytes per transaction)
	//	4 + len(m.BlockNumber) + // BlockNumber length (4 bytes)
	//	blockNumber
	//	8 + // MinTimestamp (4 bytes) + MaxTimestamp (4 bytes)
	//	4 + // RevertingHashes count (4 bytes)
	//	len(m.RevertingHashes)*4 + // Total length of all reverting hashes (4 bytes per hash)
	//	1 + // Frontrunning (1 byte)
	//	2 // MEVBuilders count (2 bytes)

	bufLen := m.size()
	buf := make([]byte, bufLen)
	m.BroadcastHeader.Pack(&buf, MEVBundleType, protocol)
	offset := BroadcastHeaderLen

	// Method
	binary.LittleEndian.PutUint16(buf[offset:], uint16(len(m.Method)))
	offset += types.UInt16Len // method is 2 bytes

	copy(buf[offset:], m.Method)
	offset += len(m.Method)

	// UUID
	if m.UUID != "" {
		uuidBytes, err := uuid.FromString(m.UUID)
		if err != nil {
			return nil, fmt.Errorf("failed to set mev bundle uuid %v", err)
		}

		copy(buf[offset:], uuidBytes[:])
	}
	offset += uuidSize

	// Transactions
	binary.LittleEndian.PutUint16(buf[offset:], uint16(len(m.Transactions)))
	offset += types.UInt16Len
	for _, tx := range m.Transactions {
		txBytes, err := hex.DecodeString(tx[2:]) // Assuming m.Transaction elements are hex strings
		if err != nil {
			return nil, fmt.Errorf("Error decoding tx: %v, err: %v", tx, err)
		}
		binary.LittleEndian.PutUint16(buf[offset:], uint16(len(txBytes)))
		offset += types.UInt16Len
		copy(buf[offset:], txBytes)
		offset += len(txBytes)
	}

	blockNumber, err := strconv.ParseInt(strings.TrimPrefix(m.BlockNumber, "0x"), 16, 64)
	if err != nil {
		return nil, err
	}
	binary.LittleEndian.PutUint64(buf[offset:], uint64(blockNumber))
	offset += types.UInt64Len

	// MinTimestamp and MaxTimestamp
	binary.LittleEndian.PutUint32(buf[offset:], uint32(m.MinTimestamp))
	offset += types.UInt32Len
	binary.LittleEndian.PutUint32(buf[offset:], uint32(m.MaxTimestamp))
	offset += types.UInt32Len

	// RevertingHashes
	binary.LittleEndian.PutUint16(buf[offset:], uint16(len(m.RevertingHashes)))
	offset += types.UInt16Len
	for _, hash := range m.RevertingHashes {
		// Remove the "0x" prefix and decode the hex string
		decodedHash, _ := hex.DecodeString(hash[2:])
		// Write the decoded hash
		copy(buf[offset:], decodedHash)
		offset += len(decodedHash)
	}

	// Bundle hash
	trimmedBundleHash := strings.TrimPrefix(m.BundleHash, "0x")
	decodedBundleHash, err := hex.DecodeString(trimmedBundleHash)
	if err != nil {
		return nil, fmt.Errorf("failed to decode bundle hash: %v", err)
	}
	copy(buf[offset:], decodedBundleHash)
	offset += types.SHA256HashLen

	// Frontrunning
	if m.Frontrunning {
		buf[offset] = 1
	} else {
		buf[offset] = 0
	}
	offset += types.UInt8Len

	// MEVBuilders
	if err := checkBuilderSize(len(m.MEVBuilders)); err != nil {
		return nil, err
	}
	mevBuilders := make([]uint8, 1)
	mevBuilders[0] = byte(len(m.MEVBuilders))
	copy(buf[offset:], mevBuilders)
	offset++

	for name, auth := range m.MEVBuilders {
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

	// Timestamp
	timestamp := float64(m.PerformanceTimestamp.UnixNano()) / nanosInSecond
	binary.LittleEndian.PutUint64(buf[offset:], math.Float64bits(timestamp))
	offset += TimestampLen

	return buf, nil
}

// Unpack Deserializes a MEVBundle into its struct
func (m *MEVBundle) Unpack(data []byte, protocol Protocol) error {
	if protocol < BundlesOverBDNProtocol {
		return fmt.Errorf("MEVBundle should not unpack from lower protocol %v", protocol)
	}

	err := m.BroadcastHeader.Unpack(data, protocol)
	if err != nil {
		return err
	}
	offset := BroadcastHeaderLen

	if err := checkBufSize(&data, offset, types.UInt16Len); err != nil {
		return err
	}
	methodLen := binary.LittleEndian.Uint16(data[offset:])
	offset += types.UInt16Len

	if err := checkBufSize(&data, offset, types.UInt16Len); err != nil {
		return err
	}
	m.Method = string(data[offset : offset+int(methodLen)])
	offset += int(methodLen)

	// UUID
	if err := checkBufSize(&data, offset, types.UInt16Len); err != nil {
		return err
	}
	if bytes.Compare(data[offset:offset+uuidSize], emptyUUID) != 0 {
		uuidRaw, err := uuid.FromBytes(data[offset : offset+uuidSize])
		if err != nil {
			return fmt.Errorf("failed to parse uuid from bytes, %v", err)
		}
		m.UUID = uuidRaw.String()
	}
	offset += uuidSize

	// Transactions
	if err := checkBufSize(&data, offset, types.UInt16Len); err != nil {
		return err
	}
	transactionCount := binary.LittleEndian.Uint16(data[offset:])
	offset += types.UInt16Len

	m.Transactions = make([]string, transactionCount)
	for i := 0; i < int(transactionCount); i++ {
		if err := checkBufSize(&data, offset, types.UInt16Len); err != nil {
			return err
		}
		txLen := binary.LittleEndian.Uint16(data[offset:])
		offset += types.UInt16Len

		if err := checkBufSize(&data, offset, int(txLen)); err != nil {
			return err
		}
		txBytes := data[offset : offset+int(txLen)]
		offset += int(txLen)
		m.Transactions[i] = "0x" + hex.EncodeToString(txBytes) // Assuming m.Transaction elements should be hex strings
	}

	if err := checkBufSize(&data, offset, types.UInt16Len); err != nil {
		return err
	}

	// BlockNumber
	blockNumberInt := int64(binary.LittleEndian.Uint64(data[offset:]))
	offset += types.UInt64Len
	m.BlockNumber = "0x" + strconv.FormatInt(blockNumberInt, 16)

	if err := checkBufSize(&data, offset, types.UInt16Len); err != nil {
		return err
	}

	// MinTimestamp and MaxTimestamp
	m.MinTimestamp = int(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += types.UInt32Len
	if err := checkBufSize(&data, offset, types.UInt16Len); err != nil {
		return err
	}

	m.MaxTimestamp = int(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += types.UInt32Len

	if err := checkBufSize(&data, offset, types.UInt16Len); err != nil {
		return err
	}

	// RevertingHashes
	numHashes := int(binary.LittleEndian.Uint16(data[offset:]))
	offset += types.UInt16Len
	m.RevertingHashes = make([]string, numHashes)
	for i := 0; i < numHashes; i++ {
		// Read the decoded hash
		if err := checkBufSize(&data, offset, types.SHA256HashLen); err != nil {
			return err
		}
		decodedHash := data[offset : offset+types.SHA256HashLen]
		offset += types.SHA256HashLen

		// Encode the hash as a hex string and add the "0x" prefix
		m.RevertingHashes[i] = "0x" + hex.EncodeToString(decodedHash)
	}

	if err := checkBufSize(&data, offset, types.SHA256HashLen); err != nil {
		return err
	}

	// Bundle hash
	decodedBundleHash := make([]byte, types.SHA256HashLen)
	copy(decodedBundleHash, data[offset:])
	offset += types.SHA256HashLen
	m.BundleHash = "0x" + hex.EncodeToString(decodedBundleHash)

	if err := checkBufSize(&data, offset, types.UInt8Len); err != nil {
		return err
	}

	// Frontrunning
	m.Frontrunning = data[offset] == 1
	offset += types.UInt8Len

	if err := checkBufSize(&data, offset, types.UInt16Len); err != nil {
		return err
	}

	// MEVBuilders
	mevBuildersCount := int(data[offset])
	offset++

	m.MEVBuilders = make(map[string]string, mevBuildersCount)
	for i := 0; i < mevBuildersCount; i++ {
		if err := checkBufSize(&data, offset, types.UInt16Len); err != nil {
			return err
		}
		nameLength := binary.LittleEndian.Uint16(data[offset:])
		offset += types.UInt16Len

		if err := checkBufSize(&data, offset, int(nameLength)); err != nil {
			return err
		}
		name := string(data[offset : offset+int(nameLength)])
		offset += int(nameLength)

		if err := checkBufSize(&data, offset, types.UInt16Len); err != nil {
			return err
		}
		authLength := binary.LittleEndian.Uint16(data[offset:])
		offset += types.UInt16Len

		if err := checkBufSize(&data, offset, int(authLength)); err != nil {
			return err
		}
		auth := string(data[offset : offset+int(authLength)])
		offset += int(authLength)

		m.MEVBuilders[name] = auth
	}

	// PerformanceTimestamp
	if err := checkBufSize(&data, offset, TimestampLen); err != nil {
		return err
	}
	tmp := binary.LittleEndian.Uint64(data[offset:])
	timestamp := math.Float64frombits(tmp)
	offset += TimestampLen
	nanoseconds := int64(timestamp) * int64(nanosInSecond)
	m.PerformanceTimestamp = time.Unix(0, nanoseconds)

	return nil
}

func checkBuilderSize(builderSize int) error {
	if builderSize == 0 {
		return fmt.Errorf("at least 1 mev builder must be present")
	}

	if builderSize > maxAuthNames {
		return fmt.Errorf("number of mev builders names %v exceeded the limit (%v)", builderSize, maxAuthNames)
	}

	return nil
}
