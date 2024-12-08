package bxmessage

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	uuid "github.com/satori/go.uuid"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage/utils"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"

	"github.com/ethereum/go-ethereum/common"
)

const (
	maxAuthNames = 255
)

var emptyUUID = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
var emptyAddress = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

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

	// used for tracking performance (gateway to receiving relay, followed by
	// overriding in relay handler (after logging),
	// and relay receiving req from other relay -> to another gateway
	PerformanceTimestamp time.Time `json:"performanceTimestamp,omitempty"`
	BundleHash           string    `json:"bundleHash"`

	MEVBuilders MEVBundleBuilders `json:"mev_builders,omitempty"`

	// From protocol version 40
	OriginalSenderAccountID string `json:"originalSenderAccountId,omitempty"`

	// From protocol version 41
	OriginalSenderAccountTier sdnmessage.AccountTier
	SentFromCloudAPI          bool

	// From protocol version 43
	AvoidMixedBundles bool

	// From protocol version 45
	PriorityFeeRefund bool `json:"priority_fee_refund"`

	// From protocol version 48
	IncomingRefundRecipient string `json:"refund_recipient"`

	// From protocol version 52
	BlocksCount      int      `json:"blocks_count,omitempty"`
	DroppingTxHashes []string `json:"dropping_tx_hashes,omitempty"`
}

// NewMEVBundle creates a new MEVBundle
func NewMEVBundle(
	transaction []string,
	uuid string,
	blockNumber string,
	minTimestamp int,
	maxTimestamp int,
	revertingHashes []string,
	mevBuilders MEVBundleBuilders,
	bundleHash string,
	avoidMixedBundles bool,
	priorityFeeRefund bool,
	incomingRefundRecipient string,
	blocksCount int,
	droppingTxHashes []string,
) (MEVBundle, error) {
	if len(uuid) != 0 && len(uuid) != 36 {
		return MEVBundle{}, errors.New("invalid uuid len")
	}
	return MEVBundle{
		Transactions:            transaction,
		UUID:                    uuid,
		BlockNumber:             blockNumber,
		MinTimestamp:            minTimestamp,
		MaxTimestamp:            maxTimestamp,
		RevertingHashes:         revertingHashes,
		MEVBuilders:             mevBuilders,
		BundleHash:              bundleHash,
		AvoidMixedBundles:       avoidMixedBundles,
		PriorityFeeRefund:       priorityFeeRefund,
		IncomingRefundRecipient: incomingRefundRecipient,
		BlocksCount:             blocksCount,
		DroppingTxHashes:        droppingTxHashes,
	}, nil
}

// String returns a string representation of the MEVBundle
func (m MEVBundle) String() string {
	return fmt.Sprintf("mev bundle(sender account ID: %s, hash: %s, blockNumber: %s, builders: %v, txs: %d, sent from cloud api: %v, tier: %v, allowMixedBundles: %v, priorityFeeRefund: %v, incomingRefundRecipient: %v, UUID: %s, MinTimestamp %v , MaxTimestamp %v, RevertingHashes %v, blocksCount %v, dropingTxs %v)",
		m.OriginalSenderAccountID, m.BundleHash, m.BlockNumber, m.MEVBuilders, len(m.Transactions), m.SentFromCloudAPI, m.OriginalSenderAccountTier, m.AvoidMixedBundles, m.PriorityFeeRefund, m.IncomingRefundRecipient, m.UUID, m.MinTimestamp, m.MaxTimestamp, len(m.RevertingHashes), m.BlocksCount, len(m.DroppingTxHashes))
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

	// 1 bytes frontrunning legacy support
	buf = append(buf, []uint8{0}...)

	// Convert map to a sorted slice of key-value pairs
	builders := make([]string, 0, len(m.MEVBuilders))
	for k := range m.MEVBuilders {
		builders = append(builders, k)
	}

	sort.Strings(builders)

	for _, name := range builders {
		auth := m.MEVBuilders[name]
		buf = append(buf, []byte(name+auth)...)
	}

	blocksCount := make([]byte, 4)
	binary.BigEndian.PutUint32(blocksCount, uint32(m.BlocksCount))
	buf = append(buf, blocksCount...)

	for _, hash := range m.DroppingTxHashes {
		buf = append(buf, []byte(hash)...)
	}

	m.hash = utils.DoubleSHA256(buf[:])
}

func (m MEVBundle) size(protocol Protocol, txs [][]byte) uint32 {
	var size uint32

	size += BroadcastHeaderLen    // Broadcast header + Control Byte
	size += types.UInt16Len       // Method size-prefix
	size += uint32(len(m.Method)) // Method content
	size += types.UUIDv4Len       // UUID
	size += types.UInt64Len       // BlockNumber
	size += types.UInt32Len       // MinTimestamp
	size += types.UInt32Len       // MaxTimestamp
	size += TimestampLen          // PerformanceTimestamp
	size += types.SHA256HashLen   // BundleHash

	if protocol < BundlesUpdatedProtocol {
		size += types.UInt8Len // frontrunning legacy support
	}

	// Transactions
	size += types.UInt16Len // Transactions count
	for _, tx := range txs {
		size += types.UInt16Len // Transaction size-prefix
		size += uint32(len(tx)) // Transaction content
	}

	// RevertingHashes
	size += types.UInt16Len                                      // RevertingHashes count
	size += uint32(types.SHA256HashLen * len(m.RevertingHashes)) // Reverting hashes content

	size += types.UInt8Len // MEVBuilders count
	for name, authorization := range m.MEVBuilders {
		size += types.UInt16Len            // Name size-prefix
		size += uint32(len(name))          // Name content
		size += types.UInt16Len            // Authorization size-prefix
		size += uint32(len(authorization)) // Authorization content
	}

	// From protocol version 38: Bundle Price and Enforce Payout
	if protocol >= BundlesOverBDNPayoutProtocol && protocol < BundlesUpdatedProtocol {
		size += types.UInt64Len // BundlePrice (8 bytes)
		size += types.UInt8Len  // EnforcePayout (1 byte)
	}

	// From protocol version 40: OriginalSenderAccountID
	if protocol >= BundlesOverBDNOriginalSenderAccountProtocol {
		size += types.UInt16Len                        // OriginalSenderAccountID size-prefix
		size += uint32(len(m.OriginalSenderAccountID)) // OriginalSenderAccountID content
	}

	// From protocol version 41: OriginalSenderAccountTier and SentFromCloudAPI
	if protocol >= BundlesOverBDNOriginalSenderTierProtocol {
		size += types.UInt16Len                          // OriginalSenderAccountTier size-prefix
		size += uint32(len(m.OriginalSenderAccountTier)) // OriginalSenderAccountTier content
		size += types.UInt8Len                           // SentFromCloudAPI bool
	}

	// From protocol version 43: AvoidMixedBundles
	if protocol >= AvoidMixedBundleProtocol {
		size += types.UInt8Len // AvoidMixedBundles bool
	}

	// From protocol version 45: BundlePriorityFeeRefund
	if protocol >= BundlePriorityFeeRefundProtocol {
		size += types.UInt8Len // PriorityFeeRefund bool
	}

	// From protocol version 48: BundleRefundProtocol
	if protocol >= BundleRefundProtocol {
		size += common.AddressLength // IncomingRefundRecipient (address)
	}

	if protocol >= BundleBlocksCountAndDroppingTxs {
		size += types.UInt32Len                                       // BlocksCount
		size += types.UInt16Len                                       // DroppingTxHashes count
		size += uint32(types.SHA256HashLen * len(m.DroppingTxHashes)) // DroppingTxHashes
	}

	return size
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

	decodedTxs, err := m.decodeTransactions()
	if err != nil {
		return nil, err
	}

	decodedRevertingHashes, err := m.decodeHashes(m.RevertingHashes)
	if err != nil {
		return nil, err
	}

	bufLen := m.size(protocol, decodedTxs)
	buf := make([]byte, bufLen)
	m.BroadcastHeader.Pack(&buf, MEVBundleType, protocol)
	offset := BroadcastHeaderOffset

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
	offset += types.UUIDv4Len

	// Transactions
	binary.LittleEndian.PutUint16(buf[offset:], uint16(len(decodedTxs)))
	offset += types.UInt16Len
	for _, tx := range decodedTxs {
		binary.LittleEndian.PutUint16(buf[offset:], uint16(len(tx)))
		offset += types.UInt16Len
		copy(buf[offset:], tx)
		offset += len(tx)
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
	binary.LittleEndian.PutUint16(buf[offset:], uint16(len(decodedRevertingHashes)))
	offset += types.UInt16Len
	for _, hash := range decodedRevertingHashes {
		copy(buf[offset:], hash)
		offset += types.SHA256HashLen
	}

	// Bundle hash
	trimmedBundleHash := strings.TrimPrefix(m.BundleHash, "0x")
	decodedBundleHash, err := hex.DecodeString(trimmedBundleHash)
	if err != nil {
		return nil, fmt.Errorf("failed to decode bundle hash: %v", err)
	}
	copy(buf[offset:], decodedBundleHash)
	offset += types.SHA256HashLen

	if protocol < BundlesUpdatedProtocol {
		// frontrunning legacy support
		buf[offset] = 0
		offset += types.UInt8Len
	}

	// MEVBuilders
	if err := checkBuilderSize(len(m.MEVBuilders)); err != nil {
		return nil, err
	}

	mevBuilders := make([]uint8, 1)
	mevBuilders[0] = byte(len(m.MEVBuilders))
	copy(buf[offset:], mevBuilders)
	offset += types.UInt8Len

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
	if protocol >= BundlesOverBDNPayoutProtocol {
		binary.LittleEndian.PutUint64(buf[offset:], uint64(m.PerformanceTimestamp.UTC().UnixNano()))
	} else {
		// Losing precision here for no reason
		timestamp := float64(m.PerformanceTimestamp.UnixNano()) / nanosInSecond
		binary.LittleEndian.PutUint64(buf[offset:], math.Float64bits(timestamp))
	}
	offset += TimestampLen

	if protocol >= BundlesOverBDNPayoutProtocol && protocol < BundlesUpdatedProtocol {
		binary.LittleEndian.PutUint64(buf[offset:], uint64(0))
		offset += types.UInt64Len
		buf[offset] = 0
		offset += types.UInt8Len
	}

	if protocol >= BundlesOverBDNOriginalSenderAccountProtocol {
		// can be either bloXroute LABS or UUID so value here is dynamic
		originalSenderAccountIDLength := len(m.OriginalSenderAccountID)
		binary.LittleEndian.PutUint16(buf[offset:], uint16(originalSenderAccountIDLength))
		offset += types.UInt16Len

		copy(buf[offset:], m.OriginalSenderAccountID)
		offset += originalSenderAccountIDLength
	}

	if protocol >= BundlesOverBDNOriginalSenderTierProtocol {
		binary.LittleEndian.PutUint16(buf[offset:], uint16(len(m.OriginalSenderAccountTier)))
		offset += types.UInt16Len

		copy(buf[offset:], m.OriginalSenderAccountTier)
		offset += len(m.OriginalSenderAccountTier)

		if m.SentFromCloudAPI {
			buf[offset] = 1
		} else {
			buf[offset] = 0
		}
		offset += types.UInt8Len //nolint:ineffassign
	}

	if protocol >= AvoidMixedBundleProtocol {
		if m.AvoidMixedBundles {
			buf[offset] = 1
		} else {
			buf[offset] = 0
		}
		offset += types.UInt8Len //nolint:ineffassign
	}

	if protocol >= BundlePriorityFeeRefundProtocol {
		if m.PriorityFeeRefund {
			buf[offset] = 1
		} else {
			buf[offset] = 0
		}
		offset += types.UInt8Len //nolint:ineffassign
	}

	if protocol >= BundleRefundProtocol {
		if m.IncomingRefundRecipient != "" {
			var address common.Address
			err := address.UnmarshalText([]byte(m.IncomingRefundRecipient))
			if err != nil {
				return nil, fmt.Errorf("failed to set mev bundle refund_recipient %v", err)
			}
			copy(buf[offset:], address.Bytes())
		}
		offset += common.AddressLength //nolint:ineffassign
	}

	if protocol >= BundleBlocksCountAndDroppingTxs {
		binary.LittleEndian.PutUint32(buf[offset:], uint32(m.BlocksCount))
		offset += types.UInt32Len

		decodedDroppingTxs, err := m.decodeHashes(m.DroppingTxHashes)
		if err != nil {
			return nil, err
		}
		binary.LittleEndian.PutUint16(buf[offset:], uint16(len(decodedDroppingTxs)))
		offset += types.UInt16Len
		for _, tx := range decodedDroppingTxs {
			copy(buf[offset:], tx)
			offset += types.SHA256HashLen
		}
	}

	if err := checkBuffEnd(&buf, offset); err != nil {
		return nil, err
	}

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
	offset := BroadcastHeaderOffset

	if err = checkBufSize(&data, offset, types.UInt16Len); err != nil {
		return err
	}
	methodLen := binary.LittleEndian.Uint16(data[offset:])
	offset += types.UInt16Len

	if err = checkBufSize(&data, offset, types.UInt16Len); err != nil {
		return err
	}
	m.Method = string(data[offset : offset+int(methodLen)])
	offset += int(methodLen)

	// UUID
	if err = checkBufSize(&data, offset, types.UUIDv4Len); err != nil {
		return err
	}
	if uuidBytes := data[offset : offset+types.UUIDv4Len]; !bytes.Equal(uuidBytes, emptyUUID) {
		uuidRaw, err := uuid.FromBytes(uuidBytes)
		if err != nil {
			return fmt.Errorf("failed to parse uuid from bytes, %v", err)
		}
		m.UUID = uuidRaw.String()
	}
	offset += types.UUIDv4Len

	// Transactions
	if err = checkBufSize(&data, offset, types.UInt16Len); err != nil {
		return err
	}
	transactionCount := binary.LittleEndian.Uint16(data[offset:])
	offset += types.UInt16Len

	m.Transactions = make([]string, transactionCount)
	for i := 0; i < int(transactionCount); i++ {
		if err = checkBufSize(&data, offset, types.UInt16Len); err != nil {
			return err
		}
		txLen := binary.LittleEndian.Uint16(data[offset:])
		offset += types.UInt16Len

		if err = checkBufSize(&data, offset, int(txLen)); err != nil {
			return err
		}
		txBytes := data[offset : offset+int(txLen)]
		offset += int(txLen)
		m.Transactions[i] = "0x" + hex.EncodeToString(txBytes) // Assuming m.Transaction elements should be hex strings
	}

	if err = checkBufSize(&data, offset, types.UInt16Len); err != nil {
		return err
	}

	// BlockNumber
	blockNumberInt := int64(binary.LittleEndian.Uint64(data[offset:]))
	offset += types.UInt64Len
	m.BlockNumber = "0x" + strconv.FormatInt(blockNumberInt, 16)

	if err = checkBufSize(&data, offset, types.UInt16Len); err != nil {
		return err
	}

	// MinTimestamp and MaxTimestamp
	m.MinTimestamp = int(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += types.UInt32Len
	if err = checkBufSize(&data, offset, types.UInt16Len); err != nil {
		return err
	}

	m.MaxTimestamp = int(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += types.UInt32Len

	if err = checkBufSize(&data, offset, types.UInt16Len); err != nil {
		return err
	}

	// RevertingHashes
	numHashes := int(binary.LittleEndian.Uint16(data[offset:]))
	offset += types.UInt16Len
	m.RevertingHashes = make([]string, numHashes)
	for i := 0; i < numHashes; i++ {
		// Read the decoded hash
		if err = checkBufSize(&data, offset, types.SHA256HashLen); err != nil {
			return err
		}
		decodedHash := data[offset : offset+types.SHA256HashLen]
		offset += types.SHA256HashLen

		// Encode the hash as a hex string and add the "0x" prefix
		m.RevertingHashes[i] = "0x" + hex.EncodeToString(decodedHash)
	}

	if err = checkBufSize(&data, offset, types.SHA256HashLen); err != nil {
		return err
	}

	// Bundle hash
	decodedBundleHash := make([]byte, types.SHA256HashLen)
	copy(decodedBundleHash, data[offset:])
	offset += types.SHA256HashLen
	m.BundleHash = "0x" + hex.EncodeToString(decodedBundleHash)

	if err = checkBufSize(&data, offset, types.UInt8Len); err != nil {
		return err
	}

	if protocol < BundlesUpdatedProtocol {
		// frontrunning legacy support
		offset += types.UInt8Len
	}

	if err = checkBufSize(&data, offset, types.UInt16Len); err != nil {
		return err
	}

	// MEVBuilders
	mevBuildersCount := int(data[offset])
	offset++

	m.MEVBuilders = make(map[string]string, mevBuildersCount)
	for i := 0; i < mevBuildersCount; i++ {
		if err = checkBufSize(&data, offset, types.UInt16Len); err != nil {
			return err
		}
		nameLength := binary.LittleEndian.Uint16(data[offset:])
		offset += types.UInt16Len

		if err = checkBufSize(&data, offset, int(nameLength)); err != nil {
			return err
		}
		name := string(data[offset : offset+int(nameLength)])
		offset += int(nameLength)

		if err = checkBufSize(&data, offset, types.UInt16Len); err != nil {
			return err
		}
		authLength := binary.LittleEndian.Uint16(data[offset:])
		offset += types.UInt16Len

		if err = checkBufSize(&data, offset, int(authLength)); err != nil {
			return err
		}
		auth := string(data[offset : offset+int(authLength)])
		offset += int(authLength)

		m.MEVBuilders[name] = auth
	}

	// PerformanceTimestamp
	if err = checkBufSize(&data, offset, TimestampLen); err != nil {
		return err
	}

	if protocol >= BundlesOverBDNPayoutProtocol {
		timestamp := binary.LittleEndian.Uint64(data[offset:])
		m.PerformanceTimestamp = time.Unix(0, int64(timestamp)).UTC()
	} else {
		// Losing precision here for no reason
		tmp := binary.LittleEndian.Uint64(data[offset:])
		timestamp := math.Float64frombits(tmp)
		nanoseconds := int64(timestamp * nanosInSecond)
		m.PerformanceTimestamp = time.Unix(0, nanoseconds)
	}
	offset += TimestampLen

	if protocol >= BundlesOverBDNPayoutProtocol && protocol < BundlesUpdatedProtocol {
		if err = checkBufSize(&data, offset, types.UInt64Len); err != nil {
			return err
		}
		offset += types.UInt64Len

		if err = checkBufSize(&data, offset, types.UInt8Len); err != nil {
			return err
		}

		offset += types.UInt8Len
	}

	if protocol >= BundlesOverBDNOriginalSenderAccountProtocol {
		if err = checkBufSize(&data, offset, types.UInt16Len); err != nil {
			return err
		}
		originalSenderAccountIDLength := binary.LittleEndian.Uint16(data[offset:])
		offset += types.UInt16Len

		if err = checkBufSize(&data, offset, int(originalSenderAccountIDLength)); err != nil {
			return err
		}

		m.OriginalSenderAccountID = string(data[offset : offset+int(originalSenderAccountIDLength)])
		offset += int(originalSenderAccountIDLength)
	}

	if protocol >= BundlesOverBDNOriginalSenderTierProtocol {
		// OriginalSenderAccountTier
		if err = checkBufSize(&data, offset, types.UInt16Len); err != nil {
			return err
		}

		originalSenderAccountTierLen := binary.LittleEndian.Uint16(data[offset:])
		offset += types.UInt16Len

		if err = checkBufSize(&data, offset, int(originalSenderAccountTierLen)); err != nil {
			return err
		}

		m.OriginalSenderAccountTier = sdnmessage.AccountTier(data[offset : offset+int(originalSenderAccountTierLen)])
		offset += int(originalSenderAccountTierLen)

		// SentFromCloudAPI
		if err := checkBufSize(&data, offset, types.UInt8Len); err != nil {
			return err
		}

		m.SentFromCloudAPI = data[offset] == 1
		offset += types.UInt8Len //nolint:ineffassign
	}

	if protocol >= AvoidMixedBundleProtocol {
		if err = checkBufSize(&data, offset, types.UInt8Len); err != nil {
			return err
		}

		m.AvoidMixedBundles = data[offset] == 1
		offset += types.UInt8Len //nolint:ineffassign
	}

	if protocol >= BundlePriorityFeeRefundProtocol {
		if err = checkBufSize(&data, offset, types.UInt8Len); err != nil {
			return err
		}

		m.PriorityFeeRefund = data[offset] == 1
		offset += types.UInt8Len //nolint:ineffassign
	}

	if protocol >= BundleRefundProtocol {
		if err = checkBufSize(&data, offset, common.AddressLength); err != nil {
			return err
		}
		if addressBytes := data[offset : offset+common.AddressLength]; !bytes.Equal(addressBytes, emptyAddress) {
			address := common.BytesToAddress(addressBytes)
			m.IncomingRefundRecipient = address.Hex()
		}
		offset += common.AddressLength //nolint:ineffassign
	}

	if protocol >= BundleBlocksCountAndDroppingTxs {
		if err = checkBufSize(&data, offset, types.UInt32Len); err != nil {
			return err
		}
		m.BlocksCount = int(binary.LittleEndian.Uint32(data[offset : offset+4]))
		offset += types.UInt32Len

		if err = checkBufSize(&data, offset, types.UInt16Len); err != nil {
			return err
		}
		numOfDroppedTxs := int(binary.LittleEndian.Uint16(data[offset:]))
		offset += types.UInt16Len
		m.DroppingTxHashes = make([]string, numOfDroppedTxs)
		for i := 0; i < numOfDroppedTxs; i++ {
			if err = checkBufSize(&data, offset, types.SHA256HashLen); err != nil {
				return err
			}
			decodedHash := data[offset : offset+types.SHA256HashLen]
			offset += types.SHA256HashLen
			m.DroppingTxHashes[i] = "0x" + hex.EncodeToString(decodedHash)
		}
	} else {
		m.DroppingTxHashes = []string{}
	}

	return nil
}

func checkBuilderSize(builderSize int) error {
	if builderSize > maxAuthNames {
		return fmt.Errorf("number of mev builders names %v exceeded the limit (%v)", builderSize, maxAuthNames)
	}

	return nil
}

func (m *MEVBundle) decodeTransactions() ([][]byte, error) {
	decoded := make([][]byte, 0, len(m.Transactions))

	for _, tx := range m.Transactions {
		txBytes, err := hex.DecodeString(strings.TrimPrefix(tx, "0x"))
		if err != nil {
			return nil, fmt.Errorf("decode tx hex: %v for bundle %v: %v", tx, m.BundleHash, err)
		}

		decoded = append(decoded, txBytes)
	}

	return decoded, nil
}

func (m *MEVBundle) decodeHashes(txs []string) ([][]byte, error) {
	decoded := make([][]byte, 0, len(txs))

	for _, hash := range txs {
		// Remove the "0x" prefix and decode the hex string
		decodedHash, err := hex.DecodeString(strings.TrimPrefix(hash, "0x"))
		if err != nil {
			return nil, fmt.Errorf("decode hash hex %v for bundle %v: %s", hash, m.BundleHash, err)
		}

		if ln := len(decodedHash); ln != types.SHA256HashLen {
			return nil, fmt.Errorf("illegal hash size %v for bundle %v: expected size: %dbytes, got %dbytes", hash, m.BundleHash, types.SHA256HashLen, ln)
		}

		decoded = append(decoded, decodedHash)
	}

	return decoded, nil
}

// Clone bundle and deep copy builders map
func (m *MEVBundle) Clone() *MEVBundle {
	copyBuilders := make(MEVBundleBuilders)
	for key, value := range m.MEVBuilders {
		copyBuilders[key] = value
	}
	return &MEVBundle{
		BroadcastHeader:           m.BroadcastHeader,
		ID:                        m.ID,
		JSONRPC:                   m.JSONRPC,
		Method:                    m.Method,
		UUID:                      m.UUID,
		Transactions:              m.Transactions,
		BlockNumber:               m.BlockNumber,
		MinTimestamp:              m.MinTimestamp,
		MaxTimestamp:              m.MaxTimestamp,
		RevertingHashes:           m.RevertingHashes,
		PerformanceTimestamp:      m.PerformanceTimestamp,
		BundleHash:                m.BundleHash,
		MEVBuilders:               copyBuilders,
		OriginalSenderAccountID:   m.OriginalSenderAccountID,
		OriginalSenderAccountTier: m.OriginalSenderAccountTier,
		SentFromCloudAPI:          m.SentFromCloudAPI,
		AvoidMixedBundles:         m.AvoidMixedBundles,
		PriorityFeeRefund:         m.PriorityFeeRefund,
	}
}
