package config

import (
	"context"
	"os"

	"fmt"

	"github.com/bloXroute-Labs/gateway/v2"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/version"
	"github.com/urfave/cli/v2"
)

// ExitErrHandler - handle errors from action function, this function executes just when we have error
func ExitErrHandler(c *cli.Context, err error) {
	if err != nil {
		log.Error(err)
	}
	if closeLogger, ok := c.Context.Value(bxgateway.CloseLoggerKey).(func()); ok && closeLogger != nil {
		closeLogger()
	}
	os.Exit(1)
}

// AfterFunc - execute when action func return without an error or when we have panic
func AfterFunc(c *cli.Context) error {
	if closeLogger, ok := c.Context.Value(bxgateway.CloseLoggerKey).(func()); ok && closeLogger != nil {
		closeLogger()
	}
	return nil
}

// BeforeFunc execute before action func
func BeforeFunc(c *cli.Context) error {
	var err error
	bxConfig, err := NewBxFromCLI(c)
	if err != nil {
		return err
	}

	var fluentdCfg *log.FluentDConfig
	if bxConfig.FluentDEnabled {
		fluentdCfg = &log.FluentDConfig{
			FluentDHost: bxConfig.FluentDHost,
			Level:       bxConfig.ConsoleLevel,
		}
	}

	closeLogger, err := log.Init(bxConfig.Config, fluentdCfg, version.BuildVersion)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	ctx := context.WithValue(c.Context, bxgateway.BxConfigKey, bxConfig)
	ctx = context.WithValue(ctx, bxgateway.CloseLoggerKey, closeLogger)
	c.Context = ctx

	return nil
}
