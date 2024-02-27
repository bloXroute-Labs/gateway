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
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils/ofac"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
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

// ParseRawTransaction is a helper function used by blxr_tx and blxr_batch_tx for processing a rawTransaction
// here it is used with the blxr_submit_bundle method on gateway
func ParseRawTransaction(tx string) (*ethtypes.Transaction, error) {
	txBytes, err := types.DecodeHex(tx)
	if err != nil {
		return nil, fmt.Errorf("invalid hex string: %w", err)
	}

	var ethTx ethtypes.Transaction
	err = ethTx.UnmarshalBinary(txBytes)

	if err != nil {
		// If UnmarshalBinary failed, we will try RLP in case user made mistake
		e := rlp.DecodeBytes(txBytes, &ethTx)
		if e != nil {
			return nil, fmt.Errorf("could not decode Ethereum transaction: %v", err)
		}
		log.Warnf("Ethereum transaction was in RLP format instead of binary, transaction has been processed anyway, but it'd be best to use the Ethereum binary standard encoding. err: %v", err)
	}
	return &ethTx, nil
}

// GetRawTxsHashes is a helper function used to parse a list of raw txs to hex
func GetRawTxsHashes(transactions []string) ([]string, error) {
	txHashes := make([]string, 0, len(transactions))
	for _, tx := range transactions {
		transaction, err := ParseRawTransaction(tx)
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
		transaction, err := ParseRawTransaction(tx)
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

	mevBundle, err := bxmessage.NewMEVBundle(
		parsedBundle.rawTxHexStrings,
		payload.UUID,
		payload.BlockNumber,
		payload.MinTimestamp,
		payload.MaxTimestamp,
		payload.RevertingHashes,
		payload.Frontrunning,
		payload.MEVBuilders,
		parsedBundle.bundleHash,
		payload.BundlePrice,
		payload.EnforcePayout,
	)
	if err != nil {
		// Validated before, should not happen
		return nil, "", err
	}

	mevBundle.SetHash()

	return &mevBundle, parsedBundle.bundleHash, nil
}
