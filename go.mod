module github.com/bloXroute-Labs/bxgateway-private-go

go 1.14

require (
	github.com/alicebob/miniredis/v2 v2.15.1
	github.com/aws/aws-lambda-go v1.26.0
	github.com/bmizerany/assert v0.0.0-20160611221934-b7ed37b82869
	github.com/ethereum/go-ethereum v1.10.8
	github.com/evalphobia/logrus_fluent v0.5.4
	github.com/fluent/fluent-logger-golang v1.5.0
	github.com/go-redis/redis/v8 v8.11.3
	github.com/golang/protobuf v1.5.2
	github.com/gorilla/websocket v1.4.2
	github.com/orandin/lumberjackrus v1.0.1
	github.com/orcaman/concurrent-map v0.0.0-20210106121528-16402b402231
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v1.8.1
	github.com/sourcegraph/jsonrpc2 v0.0.0-20200429184054-15c2290dcb37
	github.com/stretchr/testify v1.7.0
	github.com/struCoder/pidusage v0.1.3
	github.com/tinylib/msgp v1.1.5 // indirect
	github.com/urfave/cli/v2 v2.3.0
	github.com/zhouzhuojie/conditions v0.2.3
	go.uber.org/atomic v1.4.0
	golang.org/x/crypto v0.0.0-20210322153248-0c34fe9e7dc2
	golang.org/x/net v0.0.0-20210805182204-aaa1db679c0d
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	google.golang.org/grpc v1.35.1
	google.golang.org/protobuf v1.27.1
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
// PLEASE DO NOT ADD gotest.tools v2.2.0+incompatible, this package sucks
)
