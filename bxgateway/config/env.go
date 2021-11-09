package config

import (
	"fmt"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/utils"
	"github.com/urfave/cli/v2"
	"path"
	"strconv"
)

// Env represents configuration pertaining to a specific development environment
type Env struct {
	SDNURL              string
	RegistrationCertDir string
	CACertURL           string
	DataDir             string
	Environment         string
}

// constants for identifying environment configurations
const (
	Local       = "local"
	LocalTunnel = "localtunnel"
	Testnet     = "testnet"
	Mainnet     = "mainnet"
)

// NewEnvFromCLI parses an environment from a CLI provided string
// provided arguments override defaults from the --env argument
func NewEnvFromCLI(env string, ctx *cli.Context) (*Env, error) {
	gatewayEnv, err := NewEnv(env)
	if err != nil {
		return gatewayEnv, err
	}

	if ctx.IsSet(utils.RegistrationCertDirFlag.Name) {
		gatewayEnv.RegistrationCertDir = ctx.String(utils.RegistrationCertDirFlag.Name)
	}
	if ctx.IsSet(utils.SDNURLFlag.Name) {
		gatewayEnv.SDNURL = ctx.String(utils.SDNURLFlag.Name)
	}
	if ctx.IsSet(utils.CACertURLFlag.Name) {
		gatewayEnv.CACertURL = ctx.String(utils.CACertURLFlag.Name)
	}
	if ctx.IsSet(utils.DataDirFlag.Name) {
		gatewayEnv.DataDir = ctx.String(utils.DataDirFlag.Name)
	}
	gatewayEnv.DataDir = path.Join(gatewayEnv.DataDir, env, strconv.Itoa(ctx.Int(utils.PortFlag.Name)))
	return gatewayEnv, nil
}

// NewEnv returns the preconfigured environment without the need for CLI overrides
func NewEnv(env string) (*Env, error) {
	var gatewayEnv Env
	switch env {
	case Local:
		gatewayEnv = LocalEnv
	case LocalTunnel:
		gatewayEnv = LocalTunnelEnv
	case Testnet:
		gatewayEnv = TestnetEnv
	case Mainnet:
		gatewayEnv = MainnetEnv
	default:
		return nil, fmt.Errorf("could not parse unrecognized env: %v", env)
	}
	return &gatewayEnv, nil
}
