package beacon

import (
	"context"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	prysmTypes "github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/bloXroute-Labs/bxcommon-go/clock"
	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

const prysmClientTimeout = 10 * time.Second

// PrysmClient is gRPC Prysm client
type PrysmClient struct {
	ctx         context.Context
	clock       clock.Clock
	config      *network.EthConfig
	addr        string
	bridge      blockchain.Bridge
	endpoint    types.NodeEndpoint
	beaconBlock bool
	log         *log.Entry
	latestSlot  prysmTypes.Slot
}

// NewPrysmClient creates new Prysm gRPC client
func NewPrysmClient(ctx context.Context, config *network.EthConfig, addr string, bridge blockchain.Bridge, endpoint types.NodeEndpoint) *PrysmClient {
	return newPrysmClient(ctx, config, addr, bridge, endpoint, clock.RealClock{})
}

func newPrysmClient(ctx context.Context, config *network.EthConfig, addr string, bridge blockchain.Bridge, endpoint types.NodeEndpoint, clock clock.Clock) *PrysmClient {
	return &PrysmClient{
		ctx:      ctx,
		clock:    clock,
		config:   config,
		addr:     addr,
		bridge:   bridge,
		endpoint: endpoint,
		log: log.WithFields(log.Fields{
			"connType":   "prysm",
			"remoteAddr": addr,
		}),
	}
}

// Start starts subscription to prysm blocks
func (c *PrysmClient) Start() {
	c.run()
}

func (c *PrysmClient) run() {
	for {
		select {
		case <-c.ctx.Done():
			c.log.Info("prysm client stopped")
			return
		default:
			func() {
				c.log.Trace("connecting to prysm")

				conn, err := grpc.NewClient(c.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					c.log.Warnf("could not establish a connection to the Prysm: %v, retrying in %v seconds...", err, prysmClientTimeout)
					return
				}
				defer conn.Close()

				client := ethpb.NewBeaconNodeValidatorClient(conn)

				stream, err := client.StreamBlocksAltair(c.ctx, &ethpb.StreamBlocksRequest{VerifiedOnly: true})
				if err != nil {
					c.log.Errorf("could not subscribe to Prysm: %v, retrying.", err)
					return
				}

				for {
					res, err := stream.Recv()
					if err != nil {
						c.log.Errorf("connection to the prysm was broken because: %v, retrying.", err)
						return
					}

					var blk interfaces.ReadOnlySignedBeaconBlock
					switch b := res.Block.(type) {
					case *ethpb.StreamBlocksResponse_DenebBlock:
						blk, err = blocks.NewSignedBeaconBlock(b.DenebBlock)
					case *ethpb.StreamBlocksResponse_ElectraBlock:
						blk, err = blocks.NewSignedBeaconBlock(b.ElectraBlock)
					}

					if err != nil {
						c.log.Errorf("could not wrap signed beacon block: %v", err)
						continue
					}

					if blk.Block().Slot() <= currentSlot(c.config.GenesisTime)-prysmTypes.Slot(c.config.IgnoreSlotCount) {
						c.log.Errorf("block slot=%d is too old to process", blk.Block().Slot())
						continue
					}

					// only process new blocks
					if c.latestSlot >= blk.Block().Slot() {
						continue
					}
					c.latestSlot = blk.Block().Slot()

					wrappedBlock := NewWrappedReadOnlySignedBeaconBlock(blk)
					blockHash, err := wrappedBlock.HashTreeRoot()
					if err != nil {
						c.log.Errorf("could not get beacon block[slot=%d] hash: %v", blk.Block().Slot(), err)
						continue
					}
					blockHashHex := ethcommon.BytesToHash(blockHash[:]).String()

					if err := SendBlockToBDN(c.clock, c.log, wrappedBlock, c.bridge, c.endpoint); err != nil {
						c.log.Errorf("could not proccess beacon block[slot=%d,hash=%s] to eth: %v", blk.Block().Slot(), blockHashHex, err)
						continue
					}

					c.log.Tracef("received beacon block[slot=%d,hash=%s]", blk.Block().Slot(), blockHashHex)
				}
			}()
		}

		time.Sleep(prysmClientTimeout)
	}
}
