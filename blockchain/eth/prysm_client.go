package eth

import (
	"context"
	"crypto/elliptic"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/version"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/beacon"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	ethProtocols "github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/prysmaticlabs/prysm/encoding/bytesutil"
	prysm "github.com/prysmaticlabs/prysm/proto/prysm/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const prysmClientTimeout = 10 * time.Second

// PrysmClient is gRPC Prysm client
type PrysmClient struct {
	ctx     context.Context
	addr    string
	bridge  blockchain.Bridge
	backend Backend
	peer    *Peer
	log     *log.Entry
}

// NewPrysmClient creates new Prysm gRPC client
func NewPrysmClient(ctx context.Context, addr string, bridge blockchain.Bridge, backend Backend, enode *enode.Node) *PrysmClient {
	peer := NewPeer(ctx, p2p.NewPeer(enode.ID(), fmt.Sprintf("bloXroute Gateway Go v%v", version.BuildVersion), nil), nil, 0)
	pubKey := elliptic.Marshal(enode.Pubkey().Curve, enode.Pubkey().X, enode.Pubkey().Y)

	// Manually created peer has nil sign so this information is lost
	peer.endpoint = types.NodeEndpoint{IP: enode.IP().String(), Port: enode.TCP(), PublicKey: fmt.Sprintf("%x", pubKey)}

	return &PrysmClient{
		ctx:     ctx,
		addr:    addr,
		bridge:  bridge,
		backend: backend,
		peer:    peer,
		log: log.WithFields(log.Fields{
			"connType":   "prysm",
			"remoteAddr": addr,
		}),
	}
}

// Start starts subscription to prysm blocks
func (c *PrysmClient) Start() {
	go c.run()
}

func (c *PrysmClient) run() {
	for {
		func() {
			c.log.Trace("connecting to prysm")

			conn, err := grpc.Dial(c.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				c.log.Warningf("could not establish a connection to the Prysm: %v, retrying in %v seconds...", err, prysmClientTimeout)
				return
			}
			defer conn.Close()

			client := prysm.NewBeaconNodeValidatorClient(conn)

			resp, err := client.StreamBlocksAltair(context.TODO(), &prysm.StreamBlocksRequest{VerifiedOnly: false})
			if err != nil {
				c.log.Errorf("could not subscribe to Prysm: %v, retrying.", err)
				return
			}

			for {
				block, err := resp.Recv()
				if err != nil {
					c.log.Errorf("connection to the prysm was broken because: %v, retrying.", err)
					return
				}

				ethBlock, err := c.convertPrysmBlockToEthBlock(block.GetBellatrixBlock())
				if err != nil {
					c.log.Errorf("could not convert beacon block to eth: %v", err)
					continue

				}
				blockInfo := NewBlockInfo(ethBlock, nil)

				bdnBlock, err := c.bridge.BlockBlockchainToBDN(blockInfo)
				if err != nil {
					c.log.Errorf("failed to convert block %v to BDN format: %v", utils.SerializeStruct(ethBlock), err)
					continue
				}

				if err := c.bridge.SendBlockToBDN(bdnBlock, c.peer.endpoint); err != nil {
					c.log.Errorf("could not send block %v to BDN: %v", ethBlock.Hash(), err)
					continue
				}

				if c.backend != nil {
					blockPacket := &ethProtocols.NewBlockPacket{Block: ethBlock, TD: blockInfo.TotalDifficulty()}

					if err := c.backend.Handle(c.peer, blockPacket); err != nil {
						c.log.Errorf("eth2 cannot handle block %v: %v", blockInfo.Block.Hash(), err)
						continue
					}
				}
				c.log.Tracef("eth2 block %v sent to BDN", blockInfo.Block.Hash())
			}
		}()

		time.Sleep(prysmClientTimeout)
	}
}

func (c *PrysmClient) convertPrysmBlockToEthBlock(block *prysm.SignedBeaconBlockBellatrix) (*ethTypes.Block, error) {
	payload := block.GetBlock().GetBody().GetExecutionPayload()

	if payload == nil {
		return nil, errors.New("payload is empty")
	}

	transactions := make([][]byte, len(payload.GetTransactions()))
	for i, tx := range payload.GetTransactions() {
		var t ethTypes.Transaction
		if err := t.UnmarshalBinary(tx); err != nil {
			return nil, fmt.Errorf("invalid transaction %d: %v", i, err)
		}

		transactions[i] = tx
	}

	return beacon.ExecutableDataToBlock(beacon.ExecutableDataV1{
		ParentHash:    ethcommon.BytesToHash(payload.GetParentHash()),
		FeeRecipient:  ethcommon.BytesToAddress(payload.GetFeeRecipient()),
		StateRoot:     ethcommon.BytesToHash(payload.GetStateRoot()),
		ReceiptsRoot:  ethcommon.BytesToHash(payload.GetReceiptsRoot()),
		LogsBloom:     payload.GetLogsBloom(),
		Random:        ethcommon.BytesToHash(payload.GetPrevRandao()),
		Number:        payload.GetBlockNumber(),
		GasLimit:      payload.GetGasLimit(),
		GasUsed:       payload.GetGasUsed(),
		Timestamp:     payload.GetTimestamp(),
		ExtraData:     payload.GetExtraData(),
		BaseFeePerGas: new(big.Int).SetBytes(bytesutil.ReverseByteOrder(payload.GetBaseFeePerGas())),
		BlockHash:     ethcommon.BytesToHash(payload.GetBlockHash()),
		Transactions:  transactions,
	})
}
