package main

import (
	"context"
	"fmt"
	"github.com/bloXroute-Labs/gateway/v2/config"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/rpc"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/urfave/cli/v2"
	"io"
	"os"
)

func main() {
	app := &cli.App{
		UseShortOptionHandling: true,
		Name:                   "bxcli",
		Usage:                  "interact with bloxroute gateway",
		Commands: []*cli.Command{
			{
				Name:  "newtxs",
				Usage: "provides a stream of new txs",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "filters",
						Required: false,
					},
				},
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
				Name:  "blxr-batch-tx",
				Usage: "send multiple paid transactions",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:     "transactions",
						Required: true,
					},
					&cli.BoolFlag{
						Name: "nonce-monitoring",
					},
					&cli.BoolFlag{
						Name: "next-validator",
					},
					&cli.BoolFlag{
						Name: "validators-only",
					},
					&cli.IntFlag{
						Name: "fallback",
					},
					&cli.BoolFlag{
						Name: "node-validation",
					},
				},
				Action: cmdBlxrBatchTX,
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
			{
				Name:   "status",
				Usage:  "query gateway status",
				Flags:  []cli.Flag{},
				Action: cmdStatus,
			},
			{
				Name:   "listsubscriptions",
				Usage:  "query information related to the Subscriptions",
				Flags:  []cli.Flag{},
				Action: cmdListSubscriptions,
			},
			{
				Name:  "disconnectinboundpeer",
				Usage: "disconnect inbound node from gateway",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "ip",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "port",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "enode",
						Required: false,
					},
				},
				Action: cmdDisconnectInboundPeer,
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

func cmdNewTXs(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			stream, err := client.NewTxs(callCtx, &pb.NewTxsRequest{Filters: ctx.String("filters")})
			if err != nil {
				return nil, err
			}
			for {
				tx, err := stream.Recv()
				if err == io.EOF {
					log.Errorf("error EOF, %v", err)
					break
				}
				if err != nil {
					log.Errorf("error in recv, %v", err)
				}
				fmt.Println(tx)
			}
			return nil, nil
		},
	)
	if err != nil {
		return fmt.Errorf("err subscribing to feed: %v", err)
	}

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

func cmdDisconnectInboundPeer(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.DisconnectInboundPeer(callCtx, &pb.DisconnectInboundPeerRequest{PeerIp: ctx.String("ip"), PeerPort: ctx.Int64("port"), PublicKey: ctx.String("enode")})
		},
	)
	if err != nil {
		return fmt.Errorf("could not process disconnect inbound node: %v", err)
	}
	return nil
}

func cmdBlxrBatchTX(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.BlxrBatchTX(callCtx, &pb.BlxrBatchTXRequest{
				Transactions:    ctx.StringSlice("transactions"),
				NonceMonitoring: ctx.Bool("nonce-monitoring"),
				NextValidator:   ctx.Bool("next-validator"),
				ValidatorsOnly:  ctx.Bool("validators-only"),
				Fallback:        int32(ctx.Int("fallback")),
				NodeValidation:  ctx.Bool("node-validation"),
			})
		},
	)
	if err != nil {
		return fmt.Errorf("err sending transaction: %v", err)
	}

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

func cmdListSubscriptions(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.Subscriptions(callCtx, &pb.SubscriptionsRequest{})
		},
	)
	if err != nil {
		return fmt.Errorf("could not fetch peers: %v", err)
	}
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

func cmdStatus(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.Status(callCtx, &pb.StatusRequest{})
		},
	)
	if err != nil {
		return fmt.Errorf("could not get status: %v", err)
	}
	return nil
}
