package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/cert"
	bxcli "github.com/bloXroute-Labs/bxcommon-go/cli"
	"github.com/bloXroute-Labs/bxcommon-go/sdnsdk"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/urfave/cli/v2"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"
	"github.com/bloXroute-Labs/gateway/v2/config"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/rpc"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

func main() {
	app := &cli.App{
		UseShortOptionHandling: true,
		Name:                   "bxcli",
		Usage:                  "interact with bloxroute gateway",
		Commands: []*cli.Command{
			{
				Name:  "newTxs",
				Usage: "provides a stream of new txs",
				Flags: []cli.Flag{
					utils.GRPCHostFlag,
					utils.GRPCPortFlag,
					utils.GRPCUserFlag,
					utils.GRPCPasswordFlag,
					utils.GRPCAuthFlag,
					&cli.StringFlag{
						Name:     "filters",
						Required: false,
					},
					&cli.StringSliceFlag{
						Name:     "include",
						Required: false,
					},
				},
				Before: beforeBxCli,
				Action: cmdNewTXs,
			},
			{
				Name:  "pendingTxs",
				Usage: "provides a stream of pending txs",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "filters",
						Required: false,
					},
					&cli.StringSliceFlag{
						Name:     "include",
						Required: false,
					},
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: beforeBxCli,
				Action: cmdPendingTXs,
			},
			{
				Name:  "newBlocks",
				Usage: "provides a stream of new blocks",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:     "include",
						Required: false,
					},
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: beforeBxCli,
				Action: cmdNewBlocks,
			},
			{
				Name:  "bdnBlocks",
				Usage: "provides a stream of new blocks",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:     "include",
						Required: false,
					},
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: beforeBxCli,
				Action: cmdBdnBlocks,
			},
			{
				Name:  "ethOnBlock",
				Usage: "provides a stream of changes in the EVM state when a new block is mined",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:     "include",
						Required: false,
					},
					&cli.GenericFlag{
						Name:     "call-params",
						Value:    &types.CallParamSliceFlag{},
						Required: true,
					},
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: beforeBxCli,
				Action: cmdEthOnBlock,
			},
			{
				Name:  "txReceipts",
				Usage: "provides a stream of all transaction receipts in each newly mined block",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:     "include",
						Required: false,
					},
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: beforeBxCli,
				Action: cmdTxReceipts,
			},
			{
				Name:  "blxrtx",
				Usage: "send paid transaction",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "transaction",
						Required: true,
					},
					&cli.BoolFlag{
						Name: "validators-only",
					},
					&cli.BoolFlag{
						Name: "next-validator",
					},
					&cli.IntFlag{
						Name: "fallback",
					},
					&cli.BoolFlag{
						Name: "node-validation",
					},
					&cli.BoolFlag{
						Name: "front-running-protection",
					},
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: beforeBxCli,
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
					&cli.BoolFlag{
						Name: "front-running-protection",
					},
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: beforeBxCli,
				Action: cmdBlxrBatchTX,
			},
			{
				Name:  "getinfo",
				Usage: "query information on running instance",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: beforeBxCli,
				Action: cmdGetInfo,
			},
			{
				Name:  "listpeers",
				Usage: "list current connected peers",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: beforeBxCli,
				Action: cmdListPeers,
			},
			{
				Name:  "txservice",
				Usage: "query information related to the TxStore",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: beforeBxCli,
				Action: cmdTxService,
			},
			{
				Name: "stop",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: beforeBxCli,
				Action: cmdStop,
			},
			{
				Name:  "version",
				Usage: "query information related to the TxService",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: beforeBxCli,
				Action: cmdVersion,
			},
			{
				Name:  "status",
				Usage: "query gateway status",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: beforeBxCli,
				Action: cmdStatus,
			},
			{
				Name:  "listsubscriptions",
				Usage: "query information related to the Subscriptions",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: beforeBxCli,
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
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: beforeBxCli,
				Action: cmdDisconnectInboundPeer,
			},
			{
				Name:  "shortids",
				Usage: "return shortIDs to txhashs",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:     "transaction-hashes",
						Required: true,
					},
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: beforeBxCli,
				Action: cmdShortIDs,
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

func beforeBxCli(ctx *cli.Context) error {
	authHeader := ctx.String("auth-header")
	if ctx.IsSet("auth-header") && authHeader == "" {
		return fmt.Errorf("auth-header provided but is empty")
	}

	if ctx.String("auth-header") == "" {
		header, err := extractHeaderFromCerts()
		if err != nil {
			return fmt.Errorf("failed to extract header from gateway's certs. Please provide one instead by using the auth-header flag, error: %v", err)
		}
		err = ctx.Set("auth-header", header)
		if err != nil {
			return err
		}
	}
	return nil
}

func extractHeaderFromCerts() (string, error) {
	gatewayFlags, err := readGatewayArgs()
	if err != nil {
		return "", fmt.Errorf("failed to read gateway's flags, %v", err)
	}

	env, ok := gatewayFlags[utils.EnvFlag.Name]
	if !ok {
		env = utils.EnvFlag.Value
	}

	gatewayEnv, err := config.NewEnv(env)
	if err != nil {
		return "", err
	}

	dataDir, ok := gatewayFlags[utils.DataDirFlag.Name]
	if ok {
		gatewayEnv.DataDir = dataDir
	}
	registrationCertDir, ok := gatewayFlags[utils.RegistrationCertDirFlag.Name]
	if ok {
		gatewayEnv.RegistrationCertDir = registrationCertDir
	}
	sdnURL, ok := gatewayFlags[utils.SDNURLFlag.Name]
	if ok {
		gatewayEnv.SDNURL = sdnURL
	}

	nodeType, ok := gatewayFlags[utils.NodeTypeFlag.Name]
	if !ok {
		nodeType = utils.NodeTypeFlag.Value
	}
	port, ok := gatewayFlags[utils.PortFlag.Name]
	if !ok {
		port = strconv.Itoa(utils.PortFlag.Value)
	}
	gatewayEnv.DataDir = path.Join(gatewayEnv.DataDir, env, port)

	privateCertDir := path.Join(gatewayEnv.DataDir, "ssl")
	privateCertFile, privateKeyFile, registrationOnlyCertFile, registrationOnlyKeyFile := cert.GetCertDir(gatewayEnv.RegistrationCertDir, privateCertDir, nodeType)
	sslCerts := cert.NewSSLCertsFromFiles(privateCertFile, privateKeyFile, registrationOnlyCertFile, registrationOnlyKeyFile)

	// IP is set to dummy because we don't care about nodeModel, and we want to avoid the call to determine it
	sdn := sdnsdk.NewSDNHTTP(&sslCerts, gatewayEnv.SDNURL, sdnmessage.NodeModel{ExternalIP: "dummy"}, gatewayEnv.DataDir)

	accountID, err := sslCerts.GetAccountID()
	if err != nil {
		return "", err
	}
	accountModel, err := sdn.FetchCustomerAccountModel(accountID)
	if err != nil {
		return "", err
	}

	accountIDAndHash := fmt.Sprintf("%s:%s", accountModel.AccountID, accountModel.SecretHash)
	return base64.StdEncoding.EncodeToString([]byte(accountIDAndHash)), nil
}

func readGatewayArgs() (map[string]string, error) {
	psCmd := exec.Command("ps", "-ef")
	psOutput, err := psCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read running processes")
	}

	grepCmd := exec.Command("grep", "[ /]gateway ")
	grepCmd.Stdin = strings.NewReader(string(psOutput))

	grepOutput, err := grepCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read gateway processes")
	}

	lines := strings.Split(string(grepOutput), "\n")
	lines = removeEmptyLines(lines)

	if len(lines) > 1 {
		return nil, fmt.Errorf("more than two gateway processes are running")
	}

	if len(lines) < 1 {
		return nil, fmt.Errorf("no gateway processes are running")
	}

	return bxcli.ExtractArgsToMap(lines[0]), nil
}

func removeEmptyLines(s []string) []string {
	var result []string
	for _, str := range s {
		if strings.TrimSpace(str) != "" {
			result = append(result, str)
		}
	}
	return result
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

type txContent struct {
	From                string                 `json:"from"`
	Gas                 string                 `json:"gas"`
	GasPrice            string                 `json:"gasPrice"`
	GasFeeCap           string                 `json:"gasFeeCap"`
	GasTipCap           string                 `json:"gasTipCap"`
	MaxFeePerBlobGas    string                 `json:"maxFeePerBlobGas"`
	BlobVersionedHashes []string               `json:"blobVersionedHashes"`
	AuthorizationList   []setCodeAuthorization `json:"authorizationList"`
	Hash                string                 `json:"hash"`
	Input               string                 `json:"input"`
	Nonce               string                 `json:"nonce"`
	Value               string                 `json:"value"`
	V                   string                 `json:"v"`
	R                   string                 `json:"r"`
	S                   string                 `json:"s"`
	YParity             string                 `json:"yParity"`
	Type                string                 `json:"type"`
	To                  string                 `json:"to"`
}

type setCodeAuthorization struct {
	ChainID string `json:"chainId"`
	Address string `json:"address"`
	Nonce   string `json:"nonce"`
	V       string `json:"yParity"`
	R       string `json:"r"`
	S       string `json:"s"`
}

type txReply struct {
	TxHash      string    `json:"txHash"`
	TxContents  txContent `json:"txContents"`
	LocalRegion bool      `json:"localRegion"`
}

func parseTxResponse(rawTxs []*pb.Tx) ([]txReply, error) {
	transactions := []txReply{}
	for _, tx := range rawTxs {
		rawTx := tx.RawTx

		var ethTx ethtypes.Transaction
		err := ethTx.UnmarshalBinary(rawTx)
		if err != nil {
			return nil, err
		}

		v, r, s := ethTx.RawSignatureValues()
		hash := strings.ToLower(ethTx.Hash().String())

		from := hex.EncodeToString(tx.From)
		if !strings.HasPrefix(from, "0x") {
			from = "0x" + from
		}

		to := ethTx.To()
		var toHex string
		if to != nil {
			toHex = strings.ToLower(to.Hex())
		}

		txContent := txContent{
			From: from,
			Gas:  hexutil.EncodeUint64(ethTx.Gas()),

			Hash:  hash,
			Input: strings.ToLower(hexutil.Encode(ethTx.Data())),
			Nonce: strings.ToLower(hexutil.EncodeUint64(ethTx.Nonce())),
			Value: hexutil.EncodeBig(ethTx.Value()),
			V:     hexutil.EncodeBig(v),
			R:     hexutil.EncodeBig(r),
			S:     hexutil.EncodeBig(s),
			Type:  hexutil.EncodeUint64(uint64(ethTx.Type())),
			To:    strings.ToLower(toHex),
		}

		if ethTx.Type() != ethtypes.LegacyTxType {
			txContent.YParity = hexutil.Uint64(v.Sign()).String()
		}

		if ethTx.Type() == ethtypes.BlobTxType {
			txContent.MaxFeePerBlobGas = ethTx.BlobGasFeeCap().String()
			txContent.BlobVersionedHashes = make([]string, len(ethTx.BlobHashes()))
			for i, hash := range ethTx.BlobHashes() {
				txContent.BlobVersionedHashes[i] = strings.ToLower(hash.String())
			}
		}

		if ethTx.Type() >= ethtypes.DynamicFeeTxType {
			txContent.GasFeeCap = hexutil.EncodeBig(ethTx.GasFeeCap())
			txContent.GasTipCap = hexutil.EncodeBig(ethTx.GasTipCap())
		} else {
			txContent.GasPrice = hexutil.EncodeBig(ethTx.GasPrice())
		}

		if ethTx.Type() == ethtypes.SetCodeTxType {
			authList := ethTx.SetCodeAuthorizations()
			txContent.AuthorizationList = make([]setCodeAuthorization, len(authList))
			for i := range authList {
				txContent.AuthorizationList[i] = setCodeAuthorization{
					ChainID: authList[i].ChainID.String(),
					Address: strings.ToLower(authList[i].Address.Hex()),
					Nonce:   hexutil.EncodeUint64(authList[i].Nonce),
					V:       strconv.Itoa(int(authList[i].V)),
					R:       authList[i].R.String(),
					S:       authList[i].S.String(),
				}
			}
		}

		reply := txReply{
			TxHash:      hash,
			TxContents:  txContent,
			LocalRegion: tx.LocalRegion,
		}

		transactions = append(transactions, reply)
	}

	return transactions, nil
}

func cmdNewTXs(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewStreamFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			stream, err := client.NewTxs(callCtx, &pb.TxsRequest{Filters: ctx.String("filters"), Includes: ctx.StringSlice("include")})
			if err != nil {
				return nil, err
			}
			defer func() {
				err := stream.CloseSend()
				if err != nil {
					fmt.Printf("failed to close stream: %v", err)
				}
			}()

			for {
				reply, err := stream.Recv()
				if err != nil {
					return nil, err
				}

				parsedTxs, err := parseTxResponse(reply.Tx)
				if err != nil {
					return nil, err
				}

				bytes, _ := json.MarshalIndent(parsedTxs, "", "    ")
				fmt.Println(string(bytes))
			}
		},
	)
	if err != nil {
		return fmt.Errorf("err subscribing to newTxs: %v", err)
	}

	return nil
}

func cmdPendingTXs(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			stream, err := client.PendingTxs(callCtx, &pb.TxsRequest{Filters: ctx.String("filters"), Includes: ctx.StringSlice("include")})
			if err != nil {
				return nil, err
			}

			defer func() {
				err := stream.CloseSend()
				if err != nil {
					fmt.Printf("failed to close stream: %v", err)
				}
			}()

			for {
				reply, err := stream.Recv()
				if err == io.EOF {
					fmt.Println("pendingTxs error EOF: ", err)
					break
				}

				if err != nil {
					fmt.Println("pendingTxs error in recv: ", err)
					break
				}

				parsedTxs, err := parseTxResponse(reply.Tx)
				if err != nil {
					return nil, err
				}

				bytes, _ := json.MarshalIndent(parsedTxs, "", "    ")
				fmt.Println(string(bytes))
			}
			return nil, nil
		},
	)
	if err != nil {
		return fmt.Errorf("err subscribing to pendingTxs: %v", err)
	}

	return nil
}

type blocksReply struct {
	Hash                string                    `json:"hash"`
	Header              *pb.BlockHeader           `json:"header"`
	FutureValidatorInfo []*pb.FutureValidatorInfo `json:"future_validator_info"`
	Transaction         []txReply                 `json:"transaction"`
}

func parseBlockResponse(message *pb.BlocksReply) (*blocksReply, error) {
	var parsedBlock blocksReply

	parsedBlock.Hash = message.Hash
	parsedBlock.Header = message.Header
	parsedBlock.FutureValidatorInfo = message.FutureValidatorInfo
	var err error
	parsedBlock.Transaction, err = parseTxResponse(message.Transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transactions for the block %s: %v", message.Hash, err)
	}

	return &parsedBlock, nil
}

func cmdNewBlocks(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			stream, err := client.NewBlocks(callCtx, &pb.BlocksRequest{Includes: ctx.StringSlice("include")})
			if err != nil {
				return nil, err
			}
			for {
				reply, err := stream.Recv()
				if err == io.EOF {
					fmt.Println("newBlocks error EOF: ", err)
					break
				}
				if err != nil {
					fmt.Println("newBlocks error in recv: ", err)
					break
				}

				parsedBlock, err := parseBlockResponse(reply)
				bytes, _ := json.MarshalIndent(*parsedBlock, "", "    ")
				fmt.Println(string(bytes))
			}
			return nil, nil
		},
	)
	if err != nil {
		return fmt.Errorf("err subscribing to newBlocks: %v", err)
	}

	return nil
}

func cmdBdnBlocks(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			stream, err := client.BdnBlocks(callCtx, &pb.BlocksRequest{Includes: ctx.StringSlice("include")})
			if err != nil {
				return nil, err
			}
			for {
				reply, err := stream.Recv()
				if err == io.EOF {
					fmt.Println("bdnBlocks error EOF: ", err)
					break
				}
				if err != nil {
					fmt.Println("bdnBlocks error in recv: ", err)
					break
				}

				parsedBlock, err := parseBlockResponse(reply)
				bytes, _ := json.MarshalIndent(*parsedBlock, "", "    ")
				fmt.Println(string(bytes))
			}
			return nil, nil
		},
	)
	if err != nil {
		return fmt.Errorf("err subscribing to bdnBlocks: %v", err)
	}

	return nil
}

func cmdEthOnBlock(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			callParams, ok := ctx.Generic("call-params").(*types.CallParamSliceFlag)
			if !ok {
				return nil, fmt.Errorf("call-params argument has wrong format")
			}
			stream, err := client.EthOnBlock(callCtx, &pb.EthOnBlockRequest{
				Includes:   ctx.StringSlice("include"),
				CallParams: callParams.Values,
			})
			if err != nil {
				return nil, err
			}
			for {
				event, err := stream.Recv()
				if err == io.EOF {
					fmt.Println("ethOnBlock event error EOF: ", err)
					break
				}
				if err != nil {
					fmt.Println("ethOnBlock event error in recv: ", err)
					break
				}
				fmt.Println(event)
			}
			return nil, nil
		},
	)
	if err != nil {
		return fmt.Errorf("err subscribing to ethOnBlock: %v", err)
	}

	return nil
}

func cmdTxReceipts(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			stream, err := client.TxReceipts(callCtx, &pb.TxReceiptsRequest{Includes: ctx.StringSlice("include")})
			if err != nil {
				return nil, err
			}
			for {
				txReceipt, err := stream.Recv()
				if err == io.EOF {
					fmt.Println("txReceipts error EOF: ", err)
					break
				}
				if err != nil {
					fmt.Println("txReceipts error in recv: ", err)
					break
				}
				fmt.Println(txReceipt)
			}
			return nil, nil
		},
	)
	if err != nil {
		return fmt.Errorf("err subscribing to txReceipts: %v", err)
	}

	return nil
}

func cmdBlxrTX(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.BlxrTx(callCtx,
				&pb.BlxrTxRequest{
					Transaction:            ctx.String("transaction"),
					ValidatorsOnly:         ctx.Bool("validators-only"),
					NextValidator:          ctx.Bool("next-validator"),
					Fallback:               int32(ctx.Int("fallback")),
					NodeValidation:         ctx.Bool("node-validation"),
					FrontrunningProtection: ctx.Bool("front-running-protection"),
				},
			)
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
	transactions := ctx.StringSlice("transactions")
	var txsAndSenders []*pb.TxAndSender
	for _, transaction := range transactions {
		var ethTx ethtypes.Transaction
		txBytes, err := types.DecodeHex(transaction)
		if err != nil {
			fmt.Printf("Error - failed to decode transaction %v: %v. continue..", transaction, err)
			continue
		}
		err = ethTx.UnmarshalBinary(txBytes)
		if err != nil {
			e := rlp.DecodeBytes(txBytes, &ethTx)
			if e != nil {
				fmt.Printf("Error - failed to decode transaction bytes %v: %v. continue..", transaction, err)
				continue
			}
		}

		ethSender, err := ethtypes.Sender(ethtypes.NewPragueSigner(ethTx.ChainId()), &ethTx)
		if err != nil {
			fmt.Printf("Error - failed to get sender from the transaction %v: %v. continue..", transaction, err)
		}
		txsAndSenders = append(txsAndSenders, &pb.TxAndSender{Transaction: transaction, Sender: ethSender.Bytes()})

	}
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.BlxrBatchTX(callCtx, &pb.BlxrBatchTXRequest{
				TransactionsAndSenders: txsAndSenders,
				NextValidator:          ctx.Bool("next-validator"),
				ValidatorsOnly:         ctx.Bool("validators-only"),
				Fallback:               int32(ctx.Int("fallback")),
				NodeValidation:         ctx.Bool("node-validation"),
				FrontrunningProtection: ctx.Bool("front-running-protection"),
				SendingTime:            time.Now().UnixNano(),
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

func cmdShortIDs(ctx *cli.Context) error {
	transactions := ctx.StringSlice("transaction-hashes")
	txHashes := make([][]byte, len(transactions))
	for i, txHashString := range transactions {
		hash, err := types.NewSHA256HashFromString(txHashString)
		if err != nil {
			return fmt.Errorf("fail to convert text %v in position %v to hash, err %v", txHashString, i, err)
		}
		txHashes[i] = hash.Bytes()
	}
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.ShortIDs(callCtx, &pb.ShortIDsRequest{TxHashes: txHashes})
		},
	)
	if err != nil {
		return fmt.Errorf("err getting list of shortIDs: %v", err)
	}

	return nil
}
