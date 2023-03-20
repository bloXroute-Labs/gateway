package polygon_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gorilla/websocket"
	"github.com/sourcegraph/jsonrpc2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/polygon/bor"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
)

const (
	msgSubscribe      = `{"id": 1, "method": "subscribe", "params": ["newBlocks", {"include": ["header", "future_validator_info"], "blockchain_network": "Polygon-Mainnet"}]}`
	msgUnsubscribeFmt = `{"id": 1, "method": "unsubscribe", "params": ["%v"]}`
	msgGetBlockFmt    = `{"id": 1, "method": "eth_getBlockByNumber","params": ["0x%x", false]}`

	testDuration  = 8 * time.Hour
	retryDuration = 10 * time.Second

	blocksChBufferSize = 1000
)

type Notification struct {
	Params struct {
		Result *types.EthBlockNotification `json:"result"`
	} `json:"params"`
}

type rpcResp[Type any] struct {
	Error any `json:"error"`

	Result Type `json:"result"`
}

type testNotificationArgs struct {
	notification *types.EthBlockNotification

	header *ethtypes.Header

	validatorInfo [2]*ethtypes.Header

	timestamps [2]uint64
}

func getTestName(blockNum uint64, producer common.Address, flags ...string) string {
	const testNameFlag = "%s-[%s]"

	testName := fmt.Sprintf("%d", blockNum)
	if producer != (common.Address{}) {
		testName = fmt.Sprintf(testNameFlag, testName, strings.ToLower(producer.String()))
	}

	if bor.IsSprintStart(blockNum) {
		testName = fmt.Sprintf(testNameFlag, testName, "sprint")
	}

	if bor.IsSpanStart(blockNum) {
		testName = fmt.Sprintf(testNameFlag, testName, "span")
	}

	for _, flag := range flags {
		if flag == "" {
			continue
		}

		testName = fmt.Sprintf(testNameFlag, testName, flag)
	}

	return testName
}

func TestFutureValidatorPredictionLive(t *testing.T) {
	authHeader, ok := os.LookupEnv("PREDICTION_AUTH_HEADER")
	if !ok {
		t.Skip("authHeader required")
	}

	gatewayURI, ok := os.LookupEnv("PREDICTION_GATEWAY_URI")
	if !ok {
		t.Skip("gatewayURI required")
	}

	providerURI, ok := os.LookupEnv("PREDICTION_PROVIDER_URI")
	if !ok {
		t.Skip("gatewayURI required")
	}

	header := make(http.Header)
	header.Set("Authorization", authHeader)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, dialResp, err := websocket.DefaultDialer.DialContext(ctx, gatewayURI, header)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.NotNil(t, dialResp)

	defer func() { assert.NoError(t, conn.Close()) }()

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(msgSubscribe)))

	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	require.NotNil(t, msg)

	var resp jsonrpc2.Response
	require.NoError(t, json.Unmarshal(msg, &resp))

	var subscriptionID string
	require.NoError(t, json.Unmarshal(*resp.Result, &subscriptionID))

	defer func() {
		assert.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(msgUnsubscribeFmt, subscriptionID))))
	}()

	wg := new(sync.WaitGroup)

	blocksCh := make(chan uint64, blocksChBufferSize)

	blockMap := syncmap.New[uint64, *types.EthBlockNotification]()

	wg.Add(1)
	go func() {
		defer wg.Done()

		newBlocksCtx, newBlocksCancel := context.WithDeadline(ctx, time.Now().Add(testDuration))
		defer newBlocksCancel()

		newBlocksWG := new(sync.WaitGroup)

		timer := time.NewTimer(time.Minute)
		defer timer.Stop()

		for {
			select {
			case <-timer.C:
				newBlocksCancel()
			case <-newBlocksCtx.Done():
				t.Log("stop receiving of new blocks")
				newBlocksWG.Wait()
				close(blocksCh)

				return
			default:
				_, notification, notificationErr := conn.ReadMessage()
				if notificationErr != nil {
					newBlocksCancel()

					continue
				}

				assert.NoError(t, notificationErr)

				newBlocksWG.Add(1)
				go processNotification(t, newBlocksWG, notification, blocksCh, blockMap)

				timer.Reset(time.Minute)
			}
		}
	}()

	headersMap := syncmap.New[uint64, *ethtypes.Header]()

	wg.Add(1)
	go func() {
		defer wg.Done()

		providerConn, providerDialResp, providerErr := websocket.DefaultDialer.DialContext(ctx, providerURI, header)
		require.NotNil(t, providerConn)
		require.NotNil(t, providerDialResp)
		require.NoError(t, providerErr)

		defer func() { assert.NoError(t, providerConn.Close()) }()

		t.Log("block fetching started")

		for blockNumber := range blocksCh {
			if !processBlock(t, blockNumber, headersMap, providerConn) {
				cancel()

				return
			}
		}

		t.Log("stop fetching of blocks")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		testCtx, testCancel := context.WithCancel(ctx)
		defer testCancel()

		for blockMap.Len() <= int(bor.SprintSize) {
			time.Sleep(retryDuration)
		}

		sortedBlocks := blockMap.Keys()

		sort.Slice(sortedBlocks, func(i, j int) bool { return sortedBlocks[i] < sortedBlocks[j] })

		blockNum := sortedBlocks[0]

		var (
			exist  bool
			opened bool
		)

		for {
			select {
			case <-testCtx.Done():
				return
			default:
				if func() bool {
					args := testNotificationArgs{}

					if args.notification, exist = blockMap.Get(blockNum); !exist {
						if _, opened = <-blocksCh; !opened {
							testCancel()
						}

						return true
					}

					if args.header, exist = headersMap.Get(blockNum); !exist {
						return true
					}

					if args.validatorInfo[0], exist = headersMap.Get(args.notification.ValidatorInfo[0].BlockHeight); !exist {
						return true
					}

					if args.validatorInfo[1], exist = headersMap.Get(args.notification.ValidatorInfo[1].BlockHeight); !exist {
						return true
					}

					args.timestamps[0] = args.validatorInfo[0].Time - args.header.Time
					args.timestamps[1] = args.validatorInfo[1].Time - args.validatorInfo[0].Time

					blockNum++

					testNotification(t, args)

					return false
				}() {
					time.Sleep(time.Second)
				}
			}
		}
	}()

	wg.Wait()
}

func testNotification(t *testing.T, args testNotificationArgs) {
	require.NotNil(t, args.notification)
	require.NotNil(t, args.header)
	require.NotNil(t, args.validatorInfo[0])
	require.NotNil(t, args.validatorInfo[1])

	producer, err := bor.Ecrecover(args.header)

	t.Run(getTestName(args.header.Number.Uint64(), producer), func(t *testing.T) {
		require.NoError(t, err)

		for i := 0; i <= 1; i++ {
			producer, err = bor.Ecrecover(args.validatorInfo[i])

			producerStr := strings.ToLower(producer.String())

			expectedTime := uint64(2)
			if bor.IsSprintStart(args.notification.ValidatorInfo[i].BlockHeight) {
				expectedTime += 2
			}

			var producerOffline string
			var comparison require.ComparisonAssertionFunc
			if (producerStr != args.notification.ValidatorInfo[i].WalletID) && (args.timestamps[i] != expectedTime) {
				producerOffline = "producerOffline"

				comparison = require.NotEqual
			} else {
				comparison = require.Equal
			}

			t.Run(getTestName(args.validatorInfo[i].Number.Uint64(), producer, producerOffline), func(t *testing.T) {
				require.NoError(t, err)
				require.NotEmpty(t, producer)

				if producerOffline != "" {
					t.Log("in-turn validator got offline")
				}

				t.Logf("\n%s - actual\n%s - predicted\n", producerStr, args.notification.ValidatorInfo[i].WalletID)

				comparison(t, producerStr, args.notification.ValidatorInfo[i].WalletID)
			})
		}
	})
}

func processNotification(t *testing.T, wg *sync.WaitGroup, data []byte, blocksCh chan<- uint64, blockMap *syncmap.SyncMap[uint64, *types.EthBlockNotification]) {
	defer wg.Done()

	notification := new(Notification)
	assert.NoError(t, json.Unmarshal(data, notification))

	blockNumberBigInt := new(big.Int)
	blockNumberBigInt.SetString(notification.Params.Result.Header.Number, 0)

	blockNumber := blockNumberBigInt.Uint64()

	blockMap.Set(blockNumber, notification.Params.Result)

	blocksCh <- blockNumber
	blocksCh <- notification.Params.Result.ValidatorInfo[0].BlockHeight
	blocksCh <- notification.Params.Result.ValidatorInfo[1].BlockHeight

	t.Logf("notification (%d) processed", blockNumber)
}

func processBlock(t *testing.T, blockNumber uint64, headersMap *syncmap.SyncMap[uint64, *ethtypes.Header], conn *websocket.Conn) bool {
	if _, exists := headersMap.Get(blockNumber); exists {
		return true
	}

	msg := fmt.Sprintf(msgGetBlockFmt, blockNumber)

	getResp := func() *rpcResp[*ethtypes.Header] {
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(msg)))

		_, body, err := conn.ReadMessage()
		require.NoError(t, err)

		resp := new(rpcResp[*ethtypes.Header])
		require.NoError(t, json.Unmarshal(body, resp))

		return resp
	}

	resp := getResp()

	counter := 0
	for resp.Error != nil || resp.Result == nil {
		if counter >= 10 {
			require.Failf(t, "max retry attempts", "failed to fetch block: %d", blockNumber)
		}

		time.Sleep(retryDuration)

		resp = getResp()
		counter++
	}

	headersMap.Set(blockNumber, resp.Result)

	t.Logf("block (%d) fetched", blockNumber)

	return true
}
