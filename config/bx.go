package config

import (
	"errors"
	"github.com/bloXroute-Labs/gateway/utils"
	"github.com/urfave/cli/v2"
	"time"
)

const (
	defaultRPCTimeout = time.Second
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

	OverrideRelay     bool
	OverrideRelayHost string

	WebsocketEnabled bool
	WebsocketHost    string
	WebsocketPort    int

	BlocksOnly      bool
	AllTransactions bool

	*GRPC
	*Env
	*Log
}

// NewBxFromCLI builds bx node configuration from the CLI context
func NewBxFromCLI(ctx *cli.Context) (*Bx, error) {
	env, err := NewEnvFromCLI(ctx.String(utils.EnvFlag.Name), ctx)
	if err != nil {
		return nil, err
	}

	log, err := NewLogFromCLI(ctx)
	if err != nil {
		return nil, err
	}

	grpcConfig := NewGRPCFromCLI(ctx)

	nodeType, err := utils.FromStringToNodeType(ctx.String(utils.NodeTypeFlag.Name))
	bxConfig := &Bx{
		Host:               ctx.String(utils.HostFlag.Name),
		OverrideExternalIP: ctx.IsSet(utils.ExternalIPFlag.Name),
		ExternalIP:         ctx.String(utils.ExternalIPFlag.Name),
		ExternalPort:       ctx.Int64(utils.PortFlag.Name),
		BlockchainNetwork:  ctx.String(utils.BlockchainNetworkFlag.Name),
		PrioritySending:    !ctx.Bool(utils.AvoidPrioritySendingFlag.Name),
		OverrideRelay:      ctx.IsSet(utils.RelayHostFlag.Name),
		OverrideRelayHost:  ctx.String(utils.RelayHostFlag.Name),
		NodeType:           nodeType,
		LogNetworkContent:  ctx.Bool(utils.LogNetworkContentFlag.Name),
		FluentDEnabled:     ctx.Bool(utils.FluentDFlag.Name),
		FluentDHost:        ctx.String(utils.FluentdHostFlag.Name),

		WebsocketEnabled: ctx.Bool(utils.WSFlag.Name),
		WebsocketHost:    ctx.String(utils.WSHostFlag.Name),
		WebsocketPort:    ctx.Int(utils.WSPortFlag.Name),

		BlocksOnly:      ctx.Bool(utils.BlocksOnlyFlag.Name),
		AllTransactions: ctx.Bool(utils.AllTransactionsFlag.Name),

		GRPC: grpcConfig,
		Env:  env,
		Log:  log,
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
		Timeout:        defaultRPCTimeout,
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
