module github.com/bloXroute-Labs/gateway/v2

go 1.16

require (
	github.com/bmizerany/assert v0.0.0-20160611221934-b7ed37b82869
	github.com/ethereum/go-ethereum v1.10.18
	github.com/evalphobia/logrus_fluent v0.5.4
	github.com/fluent/fluent-logger-golang v1.5.0
	github.com/golang/mock v1.6.0
	github.com/google/go-cmp v0.5.6 // indirect
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.5.0
	github.com/jinzhu/copier v0.3.5
	github.com/onsi/gomega v1.15.0 // indirect
	github.com/orandin/lumberjackrus v1.0.1
	github.com/orcaman/concurrent-map v0.0.0-20210106121528-16402b402231
	github.com/prysmaticlabs/prysm v0.0.0-20220703035333-73237826d39e
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v1.8.1
	github.com/sourcegraph/jsonrpc2 v0.0.0-20200429184054-15c2290dcb37
	github.com/stretchr/testify v1.7.0
	github.com/struCoder/pidusage v0.1.3
	github.com/tinylib/msgp v1.1.5 // indirect
	github.com/urfave/cli/v2 v2.3.0
	github.com/zhouzhuojie/conditions v0.2.3
	go.uber.org/atomic v1.9.0
	golang.org/x/crypto v0.0.0-20220622213112-05595931fe9d
	golang.org/x/sync v0.0.0-20220601150217-0de741cfad7f
	google.golang.org/grpc v1.40.0
	google.golang.org/protobuf v1.28.0
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
// PLEASE DO NOT ADD gotest.tools v2.2.0+incompatible, this package sucks
)

replace github.com/btcsuite/btcd/btcec v0.23.1 => github.com/btcsuite/btcd/btcec/v2 v2.0.0-20220506033535-4550049281fc
