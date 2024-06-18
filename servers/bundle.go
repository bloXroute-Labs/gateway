package servers

import (
	"errors"
	"fmt"
	"hash"
	"math/big"
	"strconv"
	"strings"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/utils/ofac"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"golang.org/x/crypto/sha3"
)

var (
	errInvalidPayload      = errors.New("invalid payload")
	errUnableToParseBundle = errors.New("unable to parse bundle")
	errInvalidNetwork      = errors.New("invalid network")
	errBlockedTxHashes     = errors.New("found blocked tx hashes in bundle")
)

// RawTransactionGroupData is a helper data structure used as a return value from ParseRawTransactionGroup()
type RawTransactionGroupData struct {
	BundleHash          hash.Hash
	RawTxs              []string
	TxHashes            []string
	BlockedTxHashes     []string
	SanctionedAddresses []string
}

// GetRawTxsHashes is a helper function used to parse a list of raw txs to hex
func GetRawTxsHashes(transactions []string) ([]string, error) {
	txHashes := make([]string, 0, len(transactions))
	for _, tx := range transactions {
		transaction, err := utils.ParseStringTransaction(tx)
		if err != nil {
			return nil, err
		}
		txHash := transaction.Hash().Hex()
		txHashes = append(txHashes, txHash)
	}
	return txHashes, nil
}

// ParseRawTransactionGroup is a helper function used to process a group of rawTransactions
func ParseRawTransactionGroup(transactions []string, trimTxHashPrefix bool, chainID int64) (*RawTransactionGroupData, error) {
	bundleHash := sha3.NewLegacyKeccak256()
	rawTxs := make([]string, 0, len(transactions))
	txHashes := make([]string, 0, len(transactions))
	blockedTxHashes := make([]string, 0, len(transactions))
	sanctionedAddresses := make([]string, 0)
	sanctionedAddressMap := make(map[string]bool)

	for i, tx := range transactions {
		transaction, err := utils.ParseStringTransaction(tx)
		if err != nil {
			return nil, fmt.Errorf("unable to parse %d transaction error: %v", i, err)
		}

		_, err = ethtypes.NewCancunSigner(big.NewInt(chainID)).Sender(transaction)
		if err != nil {
			return nil, err
		}

		txHash := transaction.Hash().Hex()
		trimmedHash := strings.TrimPrefix(txHash, "0x")

		if trimTxHashPrefix {
			txHashes = append(txHashes, trimmedHash)
		} else {
			txHashes = append(txHashes, txHash)
		}

		if strings.HasPrefix(tx, "0x") {
			rawTxs = append(rawTxs, tx)
		} else {
			rawTxs = append(rawTxs, "0x"+tx)
		}

		bundleHash.Write(transaction.Hash().Bytes())

		if addresses, shouldBlock := ofac.ShouldBlockTransaction(transaction); shouldBlock {
			blockedTxHashes = append(blockedTxHashes, trimmedHash)
			for _, address := range addresses {
				if _, found := sanctionedAddressMap[address]; !found {
					sanctionedAddressMap[address] = true
					sanctionedAddresses = append(sanctionedAddresses, address)
				}
			}
		}
	}

	return &RawTransactionGroupData{
		BundleHash:          bundleHash,
		RawTxs:              rawTxs,
		TxHashes:            txHashes,
		BlockedTxHashes:     blockedTxHashes,
		SanctionedAddresses: sanctionedAddresses,
	}, nil
}

// parseBundle is a function used by the blxr_submit_bundle handler on the gateway
// includes OFAC checks
func parseBundle(transactions []string, chainID int64) (*GatewayParsedBundle, error) {
	parsedBundle := GatewayParsedBundle{}
	txGroupData, err := ParseRawTransactionGroup(transactions, false, chainID)
	if err != nil {
		return nil, err
	}

	parsedBundle.rawTxHexStrings = txGroupData.RawTxs
	parsedBundle.txHashses = txGroupData.TxHashes
	parsedBundle.bundleHashBytes = txGroupData.BundleHash.Sum(nil)
	parsedBundle.bundleHash = "0x" + common.Bytes2Hex(parsedBundle.bundleHashBytes)
	parsedBundle.blockedTxHashes = txGroupData.BlockedTxHashes
	parsedBundle.sanctionedAddresses = txGroupData.SanctionedAddresses
	return &parsedBundle, nil
}

// GatewayParsedBundle represents a parsed bundle used
type GatewayParsedBundle struct {
	txHashses           []string
	rawTxHexStrings     []string
	bundleHashBytes     []byte
	bundleHash          string
	blockedTxHashes     []string
	sanctionedAddresses []string
}

// GatewayBundleResponse Response struct including bundle hash
type GatewayBundleResponse struct {
	BundleHash string `json:"bundleHash"`
}

func trimZeroFromHEX(hex string) (string, error) {
	trimmedHash := strings.TrimPrefix(hex, "0x")
	value, err := strconv.ParseUint(trimmedHash, 16, 64)
	if err != nil {
		return "", err
	}

	return strings.ToLower(hexutil.EncodeUint64(value)), nil
}

func mevBundleFromRequest(payload *jsonrpc.RPCBundleSubmissionPayload, networkNum types.NetworkNum) (*bxmessage.MEVBundle, string, error) {
	if err := payload.Validate(); err != nil {
		return nil, "", fmt.Errorf("%w: %v", errInvalidPayload, err)
	}

	if networkNum == bxgateway.PolygonMainnetNum || networkNum == bxgateway.PolygonMumbaiNum {
		return nil, "", fmt.Errorf("%w: %v", errInvalidNetwork, networkNum)
	}

	parsedBundle, err := parseBundle(payload.Transaction, int64(bxgateway.NetworkNumToChainID[networkNum]))
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", errUnableToParseBundle, err)
	}

	// remove leading zeroes in hex block number
	payload.BlockNumber, err = trimZeroFromHEX(payload.BlockNumber)
	if err != nil {
		// Validated before, should not happen
		return nil, "", err
	}

	// skip processing if there are OFAC blocked txns
	if len(parsedBundle.blockedTxHashes) > 0 {
		return nil, parsedBundle.bundleHash, errBlockedTxHashes
	}

	var avoidMixedBundles bool
	if networkNum == bxgateway.BSCMainnetNum {
		avoidMixedBundles = payload.AvoidMixedBundles
	}

	mevBundle, err := bxmessage.NewMEVBundle(
		parsedBundle.rawTxHexStrings,
		payload.UUID,
		payload.BlockNumber,
		payload.MinTimestamp,
		payload.MaxTimestamp,
		payload.RevertingHashes,
		payload.MEVBuilders,
		parsedBundle.bundleHash,
		payload.BundlePrice,
		payload.EnforcePayout,
		avoidMixedBundles,
		payload.PriorityFeeRefund,
		payload.IncomingRefundRecipient,
	)
	if err != nil {
		// Validated before, should not happen
		return nil, "", err
	}

	mevBundle.SetHash()

	return &mevBundle, parsedBundle.bundleHash, nil
}
