package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/utils/bundle"
	"github.com/urfave/cli/v2"
)

const (
	defaultRPCTimeout = 1 * time.Second
	// this effects the client and not the server for the grpc connection
	defaultStreamTimeout = 24 * time.Hour
)

// Todo: separate GW and relay config

// Bx represents generic node configuration
type Bx struct {
	Host               string
	OverrideExternalIP bool
	ExternalIP         string
	ExternalPort       int64
	BlockchainNetwork  string
	PrioritySending    bool
	NodeType           utils.NodeType
	LogNetworkContent  bool
	FluentDEnabled     bool
	FluentDHost        string

	Relays string

	WebsocketEnabled    bool
	WebsocketTLSEnabled bool
	WebsocketHost       string
	WebsocketPort       int
	ManageWSServer      bool
	HTTPPort            int

	BlocksOnly       bool
	AllTransactions  bool
	SendConfirmation bool
	MEVBuilders      map[string]*bundle.Builder

	MevMinerSendBundleMethodName string
	ForwardTransactionEndpoint   string
	ForwardTransactionMethod     string
	EnableDynamicPeers           bool
	EnableBlockchainRPC          bool
	PendingTxsSourceFromNode     bool
	NoTxsToBlockchain            bool
	NoBlocks                     bool
	NoStats                      bool
	AllowIntroductoryTierAccess  bool

	*GRPC
	*Env
	*logger.Config
	*TxTraceLog
}

// NewBxFromCLI builds bx node configuration from the CLI context
func NewBxFromCLI(ctx *cli.Context) (*Bx, error) {
	env, err := NewEnvFromCLI(ctx.String(utils.EnvFlag.Name), ctx)
	if err != nil {
		return nil, err
	}

	log, txTraceLog, err := NewLogFromCLI(ctx)
	if err != nil {
		return nil, err
	}

	grpcConfig := NewGRPCFromCLI(ctx)

	nodeType, err := utils.FromStringToNodeType(ctx.String(utils.NodeTypeFlag.Name))
	if err != nil {
		return nil, err
	}

	var mevBuilders map[string]*bundle.Builder
	if ctx.IsSet(utils.MEVBuildersFilePathFlag.Name) {
		contents, err := os.ReadFile(ctx.String(utils.MEVBuildersFilePathFlag.Name))
		if err != nil {
			return nil, fmt.Errorf("failed to open mev builders file: %s", err)
		}

		if err := json.Unmarshal(contents, &mevBuilders); err != nil {
			return nil, fmt.Errorf("failed to decode mev builders file: %s", err)
		}
	}

	bxConfig := &Bx{
		Host:               ctx.String(utils.HostFlag.Name),
		OverrideExternalIP: ctx.IsSet(utils.ExternalIPFlag.Name),
		ExternalIP:         ctx.String(utils.ExternalIPFlag.Name),
		ExternalPort:       ctx.Int64(utils.PortFlag.Name),
		BlockchainNetwork:  ctx.String(utils.BlockchainNetworkFlag.Name),
		PrioritySending:    !ctx.Bool(utils.AvoidPrioritySendingFlag.Name),
		Relays:             ctx.String(utils.RelayHostsFlag.Name),
		NodeType:           nodeType,
		LogNetworkContent:  ctx.Bool(utils.LogNetworkContentFlag.Name),
		FluentDEnabled:     ctx.Bool(utils.FluentDFlag.Name),
		FluentDHost:        ctx.String(utils.FluentdHostFlag.Name),

		WebsocketEnabled:    ctx.Bool(utils.WSFlag.Name),
		WebsocketTLSEnabled: ctx.Bool(utils.WSTLSFlag.Name),
		WebsocketHost:       ctx.String(utils.WSHostFlag.Name),
		WebsocketPort:       ctx.Int(utils.WSPortFlag.Name),
		ManageWSServer:      ctx.Bool(utils.ManageWSServer.Name),

		HTTPPort: ctx.Int(utils.HTTPPortFlag.Name),

		BlocksOnly:       ctx.Bool(utils.BlocksOnlyFlag.Name),
		SendConfirmation: ctx.Bool(utils.SendBlockConfirmation.Name),
		AllTransactions:  ctx.Bool(utils.AllTransactionsFlag.Name),

		MEVBuilders: mevBuilders,

		ForwardTransactionEndpoint:  ctx.String(utils.ForwardTransactionEndpoint.Name),
		ForwardTransactionMethod:    ctx.String(utils.ForwardTransactionMethod.Name),
		EnableDynamicPeers:          ctx.Bool(utils.EnableDynamicPeers.Name),
		EnableBlockchainRPC:         ctx.Bool(utils.EnableBlockchainRPCMethodSupport.Name),
		PendingTxsSourceFromNode:    ctx.Bool(utils.PendingTxsSourceFromNode.Name),
		NoTxsToBlockchain:           ctx.Bool(utils.NoTxsToBlockchain.Name),
		NoBlocks:                    ctx.Bool(utils.NoBlocks.Name),
		NoStats:                     ctx.Bool(utils.NoStats.Name),
		AllowIntroductoryTierAccess: ctx.Bool(utils.EnableIntroductoryIntentsAccess.Name),

		GRPC:       grpcConfig,
		Env:        env,
		Config:     log,
		TxTraceLog: txTraceLog,
	}

	if bxConfig.BlocksOnly && bxConfig.AllTransactions {
		return bxConfig, errors.New("cannot set both --blocks-only and --all-txs")
	}

	return bxConfig, nil
}

// GRPC represents Go RPC configuration details
type GRPC struct {
	Enabled     bool
	Host        string
	Port        int
	User        string
	Password    string
	EncodedAuth string

	AuthEnabled    bool
	EncodedAuthSet bool

	Timeout time.Duration
}

// NewGRPCFromCLI builds GRPC configuration from the CLI context
func NewGRPCFromCLI(ctx *cli.Context) *GRPC {
	grpcConfig := GRPC{
		Enabled:        ctx.Bool(utils.GRPCFlag.Name),
		Host:           ctx.String(utils.GRPCHostFlag.Name),
		Port:           ctx.Int(utils.GRPCPortFlag.Name),
		User:           ctx.String(utils.GRPCUserFlag.Name),
		Password:       ctx.String(utils.GRPCPasswordFlag.Name),
		EncodedAuth:    ctx.String(utils.GRPCAuthFlag.Name),
		EncodedAuthSet: ctx.IsSet(utils.GRPCAuthFlag.Name),
		AuthEnabled:    ctx.IsSet(utils.GRPCAuthFlag.Name) || (ctx.IsSet(utils.GRPCUserFlag.Name) && ctx.IsSet(utils.GRPCPasswordFlag.Name)),
		Timeout:        defaultStreamTimeout,
	}
	return &grpcConfig
}

// NewStreamFromCLI builds GRPC stream configuration from the CLI context
func NewStreamFromCLI(ctx *cli.Context) *GRPC {
	grpcConfig := GRPC{
		Enabled:        ctx.Bool(utils.GRPCFlag.Name),
		Host:           ctx.String(utils.GRPCHostFlag.Name),
		Port:           ctx.Int(utils.GRPCPortFlag.Name),
		User:           ctx.String(utils.GRPCUserFlag.Name),
		Password:       ctx.String(utils.GRPCPasswordFlag.Name),
		EncodedAuth:    ctx.String(utils.GRPCAuthFlag.Name),
		EncodedAuthSet: ctx.IsSet(utils.GRPCAuthFlag.Name),
		AuthEnabled:    ctx.IsSet(utils.GRPCAuthFlag.Name) || (ctx.IsSet(utils.GRPCUserFlag.Name) && ctx.IsSet(utils.GRPCPasswordFlag.Name)),
		Timeout:        defaultStreamTimeout,
	}
	return &grpcConfig
}

// NewGRPC builds a simple GRPC configuration from parameters
func NewGRPC(host string, port int, user string, password string) *GRPC {
	authEnabled := user != "" && password != ""
	return &GRPC{
		Enabled:     true,
		Host:        host,
		Port:        port,
		User:        user,
		Password:    password,
		AuthEnabled: authEnabled,
		Timeout:     defaultRPCTimeout,
	}
}
