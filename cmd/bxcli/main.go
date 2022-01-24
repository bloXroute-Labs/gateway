package main

import (
	"context"
	"fmt"
	"github.com/bloXroute-Labs/gateway/config"
	pb "github.com/bloXroute-Labs/gateway/protobuf"
	"github.com/bloXroute-Labs/gateway/rpc"
	"github.com/bloXroute-Labs/gateway/utils"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"os"
)

func main() {
	app := &cli.App{
		UseShortOptionHandling: true,
		Name:                   "bxcli",
		Usage:                  "interact with bloxroute gateway",
		Commands: []*cli.Command{
			{
				Name:   "newtxs",
				Usage:  "provides a stream of new txs",
				Flags:  []cli.Flag{},
				Action: cmdNewTXs,
			},
			{
				Name:   "pendingtxs",
				Usage:  "provides a stream of pending txs",
				Flags:  []cli.Flag{},
				Action: cmdPendingTXs,
			},
			{
				Name:  "blxrtx",
				Usage: "send paid transaction",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "transaction",
						Required: true,
					},
				},
				Action: cmdBlxrTX,
			},
			{
				Name:   "blxrtxs",
				Usage:  "send multiple paid transaction",
				Flags:  []cli.Flag{},
				Action: cmdBlxrTXs,
			},
			{
				Name:   "getinfo",
				Usage:  "query information on running instance",
				Flags:  []cli.Flag{},
				Action: cmdGetInfo,
			},
			{
				Name:   "listpeers",
				Usage:  "list current connected peers",
				Flags:  []cli.Flag{},
				Action: cmdListPeers,
			},
			{
				Name:   "txservice",
				Usage:  "query information related to the TxStore",
				Flags:  []cli.Flag{},
				Action: cmdTxService,
			},
			{
				Name:   "stop",
				Flags:  []cli.Flag{},
				Action: cmdStop,
			},
			{
				Name:   "version",
				Usage:  "query information related to the TxService",
				Flags:  []cli.Flag{},
				Action: cmdVersion,
			},
		},
		Flags: []cli.Flag{
			utils.GRPCHostFlag,
			utils.GRPCPortFlag,
			utils.GRPCUserFlag,
			utils.GRPCPasswordFlag,
			utils.GRPCAuthFlag,
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func cmdStop(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.Stop(callCtx, &pb.StopRequest{})
		},
	)
	if err != nil {
		return fmt.Errorf("could not run stop: %v", err)
	}
	return nil
}

func cmdVersion(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.Version(callCtx, &pb.VersionRequest{})
		},
	)
	if err != nil {
		return fmt.Errorf("could not fetch version: %v", err)
	}
	return nil
}

func cmdNewTXs(*cli.Context) error {
	fmt.Printf("left to do:")
	return nil
}

func cmdPendingTXs(*cli.Context) error {
	fmt.Printf("left to do:")
	return nil
}

func cmdBlxrTX(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.BlxrTx(callCtx, &pb.BlxrTxRequest{Transaction: ctx.String("transaction")})
		},
	)
	if err != nil {
		return fmt.Errorf("could not process blxr tx: %v", err)
	}
	return nil
}

func cmdBlxrTXs(*cli.Context) error {
	fmt.Printf("left to do:")
	return nil
}

func cmdGetInfo(*cli.Context) error {
	fmt.Printf("left to do:")
	return nil
}

func cmdTxService(*cli.Context) error {
	fmt.Printf("left to do:")
	return nil
}

func cmdListPeers(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.Peers(callCtx, &pb.PeersRequest{})
		},
	)
	if err != nil {
		return fmt.Errorf("could not fetch peers: %v", err)
	}
	return nil
}
