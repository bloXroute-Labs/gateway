package connections

import (
	"github.com/bloXroute-Labs/gateway/bxgateway/config"
	"github.com/bloXroute-Labs/gateway/bxgateway/sdnmessage"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestBestRelay_IfPingOver40MSLogsWarning(t *testing.T) {
	testTable := []struct {
		name       string
		relayCount int
		latencies  []nodeLatencyInfo
		log        string
	}{
		{"Latency 5", 1, []nodeLatencyInfo{{Latency: 5}}, ""},
		{"Latency 20", 1, []nodeLatencyInfo{{Latency: 20}}, ""},
		{"Latency 5, 41", 2, []nodeLatencyInfo{{Latency: 5}, {Latency: 41}}, ""},
		{"Latency 41", 1, []nodeLatencyInfo{{Latency: 41, IP: "1.1.1.1", Port: 40}},
			"ping latency of the fastest relay 1.1.1.1:40 is 41 ms, which is more than 40 ms"},

		{"Latency 100, 200", 2, []nodeLatencyInfo{{Latency: 1000, IP: "1.1.1.1", Port: 40}, {Latency: 2000, IP: "2.2.2.2"}},
			"ping latency of the fastest relay 1.1.1.1:40 is 1000 ms, which is more than 40 ms"},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			globalHook := test.NewGlobal()
			getPingLatenciesFunction := func(peers sdnmessage.Peers) []nodeLatencyInfo {
				return testCase.latencies
			}
			s := SDNHTTP{relays: make([]sdnmessage.Peer, testCase.relayCount), getPingLatencies: getPingLatenciesFunction}
			b := config.Bx{}

			b.OverrideRelay = false
			_, _, err := s.BestRelay(&b)
			assert.Nil(t, err)

			logs := globalHook.Entries
			if testCase.log == "" {
				assert.Nil(t, logs)
			} else {
				if len(logs) == 0 {
					t.Fail()
				}
				firstLog := logs[0]
				assert.Equal(t, testCase.log, firstLog.Message)
			}
		})
	}
}
