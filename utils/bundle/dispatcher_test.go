package bundle

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/sourcegraph/jsonrpc2"
	"github.com/stretchr/testify/assert"
)

var (
	testTx1     = "0xf85d016480945444d5db68dbfe553afa40d83d056a1cfe281ef7018037a032935466558b727cabbf96f9bd27edd7d705ba2cc864470b2b3da65d4d50a39fa05b22610aa673d37ac3e8bf3ce17ca11b4e47b9113acb39f76530e5ac0b50a5e8"
	testTx2     = "0xf85d026480945444d5db68dbfe553afa40d83d056a1cfe281ef7018037a0a37f641223532c786b3432cad7c6148e4efdbd3600215c9b4deb684bfc320875a00ce6cd896863c73668495437a51dbf240f2971d56d86560fe1f13cfe4b2a717c"
	testTx1Hash = "0xae65f9be2a406d92db019bfec33508c235dd1c0ee734b57940acaaf634c9c244"
)

func makeBuildersMap(prefix string, builders []string) map[string]*Builder {
	buildersMap := make(map[string]*Builder)
	for _, builder := range builders {
		buildersMap[builder] = &Builder{
			Name:      builder,
			Endpoints: []string{fmt.Sprintf("%s%s", prefix, builder)},
		}
	}
	return buildersMap
}

func TestDispatcher(t *testing.T) {
	testCases := []struct {
		// TODO: test logs or handle them in a better way
		name        string
		builders    []string
		bundle      bxmessage.MEVBundle
		expPayload  *jsonrpc.RPCSendBundle
		expBuilders []string
	}{
		{
			name:     "one builder called",
			builders: []string{"builder1"},
			bundle: bxmessage.MEVBundle{
				Method:          string(jsonrpc.RPCEthSendBundle),
				Transactions:    []string{testTx1, testTx2},
				UUID:            "e2a1c984-b31c-4bc6-a2eb-d2d903aab6d8",
				BlockNumber:     fmt.Sprintf("0x%x", 123),
				MinTimestamp:    1686120664,
				MaxTimestamp:    1686120884,
				RevertingHashes: []string{testTx1Hash},
				MEVBuilders:     bxmessage.MEVBundleBuilders{"builder1": ""},
			},
			expPayload: &jsonrpc.RPCSendBundle{
				Txs:               []string{testTx1, testTx2},
				UUID:              "e2a1c984-b31c-4bc6-a2eb-d2d903aab6d8",
				BlockNumber:       fmt.Sprintf("0x%x", 123),
				MinTimestamp:      1686120664,
				MaxTimestamp:      1686120884,
				RevertingTxHashes: []string{testTx1Hash},
			},
			expBuilders: []string{"builder1"},
		},
		{
			name:     "multiple builders called",
			builders: []string{"builder1", "builder2"},
			bundle: bxmessage.MEVBundle{
				Method:          string(jsonrpc.RPCEthSendBundle),
				Transactions:    []string{testTx1, testTx2},
				UUID:            "e2a1c984-b31c-4bc6-a2eb-d2d903aab6d8",
				BlockNumber:     fmt.Sprintf("0x%x", 123),
				MinTimestamp:    1686120664,
				MaxTimestamp:    1686120884,
				RevertingHashes: []string{testTx1Hash},
				MEVBuilders:     bxmessage.MEVBundleBuilders{"builder1": "", "builder2": ""},
			},
			expPayload: &jsonrpc.RPCSendBundle{
				Txs:               []string{testTx1, testTx2},
				UUID:              "e2a1c984-b31c-4bc6-a2eb-d2d903aab6d8",
				BlockNumber:       fmt.Sprintf("0x%x", 123),
				MinTimestamp:      1686120664,
				MaxTimestamp:      1686120884,
				RevertingTxHashes: []string{testTx1Hash},
			},
			expBuilders: []string{"builder1", "builder2"},
		},
		{
			name:     "all builders called",
			builders: []string{"builder1", "builder2"},
			bundle: bxmessage.MEVBundle{
				Method:          string(jsonrpc.RPCEthSendBundle),
				Transactions:    []string{testTx1, testTx2},
				UUID:            "e2a1c984-b31c-4bc6-a2eb-d2d903aab6d8",
				BlockNumber:     fmt.Sprintf("0x%x", 123),
				MinTimestamp:    1686120664,
				MaxTimestamp:    1686120884,
				RevertingHashes: []string{testTx1Hash},
				MEVBuilders:     bxmessage.MEVBundleBuilders{"all": ""},
			},
			expPayload: &jsonrpc.RPCSendBundle{
				Txs:               []string{testTx1, testTx2},
				UUID:              "e2a1c984-b31c-4bc6-a2eb-d2d903aab6d8",
				BlockNumber:       fmt.Sprintf("0x%x", 123),
				MinTimestamp:      1686120664,
				MaxTimestamp:      1686120884,
				RevertingTxHashes: []string{testTx1Hash},
			},
			expBuilders: []string{"builder1", "builder2"},
		},
		{
			name:     "one builder of multiple called",
			builders: []string{"builder1", "builder2"},
			bundle: bxmessage.MEVBundle{
				Method:          string(jsonrpc.RPCEthSendBundle),
				Transactions:    []string{testTx1, testTx2},
				UUID:            "e2a1c984-b31c-4bc6-a2eb-d2d903aab6d8",
				BlockNumber:     fmt.Sprintf("0x%x", 123),
				MinTimestamp:    1686120664,
				MaxTimestamp:    1686120884,
				RevertingHashes: []string{testTx1Hash},
				MEVBuilders:     bxmessage.MEVBundleBuilders{"builder2": ""},
			},
			expPayload: &jsonrpc.RPCSendBundle{
				Txs:               []string{testTx1, testTx2},
				UUID:              "e2a1c984-b31c-4bc6-a2eb-d2d903aab6d8",
				BlockNumber:       fmt.Sprintf("0x%x", 123),
				MinTimestamp:      1686120664,
				MaxTimestamp:      1686120884,
				RevertingTxHashes: []string{testTx1Hash},
			},
			expBuilders: []string{"builder2"},
		},
		{
			name:     "partial intersection of builders called",
			builders: []string{"builder1", "builder2"},
			bundle: bxmessage.MEVBundle{
				Method:          string(jsonrpc.RPCEthSendBundle),
				Transactions:    []string{testTx1, testTx2},
				UUID:            "e2a1c984-b31c-4bc6-a2eb-d2d903aab6d8",
				BlockNumber:     fmt.Sprintf("0x%x", 123),
				MinTimestamp:    1686120664,
				MaxTimestamp:    1686120884,
				RevertingHashes: []string{testTx1Hash},
				MEVBuilders:     bxmessage.MEVBundleBuilders{"builder1": "", "builder3": ""},
			},
			expPayload: &jsonrpc.RPCSendBundle{
				Txs:               []string{testTx1, testTx2},
				UUID:              "e2a1c984-b31c-4bc6-a2eb-d2d903aab6d8",
				BlockNumber:       fmt.Sprintf("0x%x", 123),
				MinTimestamp:      1686120664,
				MaxTimestamp:      1686120884,
				RevertingTxHashes: []string{testTx1Hash},
			},
			expBuilders: []string{"builder1"},
		},
		{
			name:     "default bloxroute builder called for empty builders if configured",
			builders: []string{"bloxroute", "builder2"},
			bundle: bxmessage.MEVBundle{
				Method:          string(jsonrpc.RPCEthSendBundle),
				Transactions:    []string{testTx1, testTx2},
				UUID:            "e2a1c984-b31c-4bc6-a2eb-d2d903aab6d8",
				BlockNumber:     fmt.Sprintf("0x%x", 123),
				MinTimestamp:    1686120664,
				MaxTimestamp:    1686120884,
				RevertingHashes: []string{testTx1Hash},
				MEVBuilders:     bxmessage.MEVBundleBuilders{},
			},
			expPayload: &jsonrpc.RPCSendBundle{
				Txs:               []string{testTx1, testTx2},
				UUID:              "e2a1c984-b31c-4bc6-a2eb-d2d903aab6d8",
				BlockNumber:       fmt.Sprintf("0x%x", 123),
				MinTimestamp:      1686120664,
				MaxTimestamp:      1686120884,
				RevertingTxHashes: []string{testTx1Hash},
			},
			expBuilders: []string{"bloxroute"},
		},
		{
			name:     "default bloxroute builder not called for empty builders if not configured",
			builders: []string{},
			bundle: bxmessage.MEVBundle{
				Method:          string(jsonrpc.RPCEthSendBundle),
				Transactions:    []string{testTx1, testTx2},
				UUID:            "e2a1c984-b31c-4bc6-a2eb-d2d903aab6d8",
				BlockNumber:     fmt.Sprintf("0x%x", 123),
				MinTimestamp:    1686120664,
				MaxTimestamp:    1686120884,
				RevertingHashes: []string{testTx1Hash},
				MEVBuilders:     bxmessage.MEVBundleBuilders{},
			},
			expPayload: &jsonrpc.RPCSendBundle{
				Txs:               []string{testTx1, testTx2},
				UUID:              "e2a1c984-b31c-4bc6-a2eb-d2d903aab6d8",
				BlockNumber:       fmt.Sprintf("0x%x", 123),
				MinTimestamp:      1686120664,
				MaxTimestamp:      1686120884,
				RevertingTxHashes: []string{testTx1Hash},
			},
			expBuilders: []string{},
		},
		{
			name:     "no builders called if no configured",
			builders: []string{},
			bundle: bxmessage.MEVBundle{
				Method:          string(jsonrpc.RPCEthSendBundle),
				Transactions:    []string{testTx1, testTx2},
				UUID:            "e2a1c984-b31c-4bc6-a2eb-d2d903aab6d8",
				BlockNumber:     fmt.Sprintf("0x%x", 123),
				MinTimestamp:    1686120664,
				MaxTimestamp:    1686120884,
				RevertingHashes: []string{testTx1Hash},
				MEVBuilders:     bxmessage.MEVBundleBuilders{"builder1": ""},
			},
			expPayload:  nil,
			expBuilders: []string{},
		},
		{
			name:     "no builders called if no overlap",
			builders: []string{"builder3", "builder4"},
			bundle: bxmessage.MEVBundle{
				Method:          string(jsonrpc.RPCEthSendBundle),
				Transactions:    []string{testTx1, testTx2},
				UUID:            "e2a1c984-b31c-4bc6-a2eb-d2d903aab6d8",
				BlockNumber:     fmt.Sprintf("0x%x", 123),
				MinTimestamp:    1686120664,
				MaxTimestamp:    1686120884,
				RevertingHashes: []string{testTx1Hash},
				MEVBuilders:     bxmessage.MEVBundleBuilders{"builder1": "", "builder2": ""},
			},
			expPayload:  nil,
			expBuilders: []string{},
		},
		{
			name:     "no builders called if no builders provided",
			builders: []string{"builder1", "builder2"},
			bundle: bxmessage.MEVBundle{
				Method:          string(jsonrpc.RPCEthSendBundle),
				Transactions:    []string{testTx1, testTx2},
				UUID:            "e2a1c984-b31c-4bc6-a2eb-d2d903aab6d8",
				BlockNumber:     fmt.Sprintf("0x%x", 123),
				MinTimestamp:    1686120664,
				MaxTimestamp:    1686120884,
				RevertingHashes: []string{testTx1Hash},
				MEVBuilders:     bxmessage.MEVBundleBuilders{},
			},
			expPayload:  nil,
			expBuilders: []string{},
		},
	}

	for _, tc := range testCases {
		bundle := tc.bundle
		t.Run(tc.name, func(t *testing.T) {
			mu := &sync.Mutex{}
			wg := &sync.WaitGroup{}
			buildersMap := makeBuildersMap("", tc.expBuilders)
			wg.Add(len(tc.expBuilders))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer wg.Done()

				assert.Equal(t, "POST", r.Method)

				path := strings.TrimPrefix(r.URL.Path, "/")
				mu.Lock()
				_, ok := buildersMap[path]
				assert.True(t, ok)
				delete(buildersMap, path)
				mu.Unlock()

				var req jsonrpc2.Request
				err := json.NewDecoder(r.Body).Decode(&req)
				assert.NoError(t, err)

				assert.Equal(t, string(jsonrpc.RPCEthSendBundle), req.Method)

				var payload []jsonrpc.RPCSendBundle
				err = json.Unmarshal(*req.Params, &payload)
				assert.NoError(t, err)

				assert.Equal(t, *tc.expPayload, payload[0])
			}))
			defer server.Close()

			d := NewDispatcher(statistics.NoStats{}, makeBuildersMap(fmt.Sprintf("%s/", server.URL), tc.builders))
			err := d.Dispatch(&bundle)
			assert.NoError(t, err)

			wg.Wait()
			time.Sleep(5 * time.Millisecond) // wait if there are any non-expected calls
			assert.Empty(t, buildersMap)
		})
	}
}
