package nodes

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/bloXroute-Labs/gateway/v2/services"
)

// Node represents the basic node interface
type Node interface {
	Run() error
	Close() error
}

// Abstract represents a basic bloxroute node interface
type Abstract struct {
	TxStore services.TxStore
}

func notImplError(funcName string) error {
	return fmt.Errorf("func %v not implemented", funcName)
}

// Run starts running the abstract node
func (an Abstract) Run(ctx *cli.Context) error {
	return notImplError("Run")
}
