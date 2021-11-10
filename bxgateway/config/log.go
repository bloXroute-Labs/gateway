package config

import (
	"fmt"
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/utils"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// TxTraceLog represents tx trace log config
type TxTraceLog struct {
	Enabled      bool
	MaxFileSize  int
	MaxFileCount int
}

// Log represents logger options for where to write data and what data to write
type Log struct {
	AppName      string
	FileName     string
	FileLevel    log.Level
	ConsoleLevel log.Level
	MaxSize      int
	MaxBackups   int
	MaxAge       int
	TxTrace      TxTraceLog
}

// NewLogFromCLI builds new log configuration from the CLI context
func NewLogFromCLI(ctx *cli.Context) (*Log, error) {
	consoleLevel, err := log.ParseLevel(ctx.String(utils.LogLevelFlag.Name))
	if err != nil {
		return nil, err
	}

	fileLevel, err := log.ParseLevel(ctx.String(utils.LogFileLevelFlag.Name))
	if err != nil {
		return nil, err
	}

	logConfig := Log{
		AppName:      ctx.App.Name,
		FileName:     fmt.Sprintf("logs/%v-%v.log", ctx.App.Name, ctx.Int(utils.PortFlag.Name)),
		FileLevel:    fileLevel,
		ConsoleLevel: consoleLevel,
		MaxSize:      ctx.Int(utils.LogMaxSizeFlag.Name),
		MaxBackups:   ctx.Int(utils.LogMaxBackupsFlag.Name),
		MaxAge:       ctx.Int(utils.LogMaxAgeFlag.Name),
		TxTrace: TxTraceLog{
			Enabled:      ctx.Bool(utils.TxTraceEnabledFlag.Name),
			MaxFileSize:  ctx.Int(utils.TxTraceMaxFileSize.Name),
			MaxFileCount: ctx.Int(utils.TxTraceMaxFileCount.Name),
		},
	}
	return &logConfig, nil
}
