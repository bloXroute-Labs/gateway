package config

import (
	"fmt"
	log "github.com/bloXroute-Labs/gateway/logger"
	"github.com/bloXroute-Labs/gateway/utils"
	"github.com/urfave/cli/v2"
)

// TxTraceLog represents tx trace log config
type TxTraceLog struct {
	Enabled        bool
	MaxFileSize    int
	MaxBackupFiles int
}

// NewLogFromCLI builds new log configuration from the CLI context
func NewLogFromCLI(ctx *cli.Context) (*log.Config, *TxTraceLog, error) {
	consoleLevel, err := log.ParseLevel(ctx.String(utils.LogLevelFlag.Name))
	if err != nil {
		return nil, nil, err
	}

	fileLevel, err := log.ParseLevel(ctx.String(utils.LogFileLevelFlag.Name))
	if err != nil {
		return nil, nil, err
	}

	logConfig := log.Config{
		AppName:      ctx.App.Name,
		FileName:     fmt.Sprintf("logs/%v-%v.log", ctx.App.Name, ctx.Int(utils.PortFlag.Name)),
		FileLevel:    fileLevel,
		ConsoleLevel: consoleLevel,
		MaxSize:      ctx.Int(utils.LogMaxSizeFlag.Name),
		MaxBackups:   ctx.Int(utils.LogMaxBackupsFlag.Name),
		MaxAge:       ctx.Int(utils.LogMaxAgeFlag.Name),
	}
	txTraceConfig := TxTraceLog{
		Enabled:        ctx.Bool(utils.TxTraceEnabledFlag.Name),
		MaxFileSize:    ctx.Int(utils.TxTraceMaxFileSizeFlag.Name),
		MaxBackupFiles: ctx.Int(utils.TxTraceMaxBackupFilesFlag.Name),
	}

	return &logConfig, &txTraceConfig, nil
}
