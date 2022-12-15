package beacon

import (
	"context"
	"crypto/elliptic"
	"fmt"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/prysmaticlabs/prysm/v3/consensus-types/blocks"
	prysm "github.com/prysmaticlabs/prysm/v3/proto/prysm/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const prysmClientTimeout = 10 * time.Second

// PrysmClient is gRPC Prysm client
type PrysmClient struct {
	ctx            context.Context
	addr           string
	bridge         blockchain.Bridge
	endpoint       types.NodeEndpoint
	blockProcessor *blockProcessor
	beaconBlock    bool
	log            *log.Entry
}

// NewPrysmClient creates new Prysm gRPC client
func NewPrysmClient(ctx context.Context, config *network.EthConfig, ethChain *eth.Chain, addr string, bridge blockchain.Bridge, enode *enode.Node, beaconBlock bool) *PrysmClient {
	pubKey := elliptic.Marshal(enode.Pubkey().Curve, enode.Pubkey().X, enode.Pubkey().Y)
	endpoint := types.NodeEndpoint{IP: enode.IP().String(), Port: enode.TCP(), PublicKey: fmt.Sprintf("%x", pubKey)}

	log := log.WithFields(log.Fields{
		"connType":   "prysm",
		"remoteAddr": addr,
	})

	return &PrysmClient{
		ctx:            ctx,
		addr:           addr,
		bridge:         bridge,
		endpoint:       endpoint,
		blockProcessor: newBlockProcessor(ctx, config, newChainAdapter(beaconBlock, ethChain, NewChain(ctx, config.GenesisTime, config.IgnoreSlotCount)), bridge, nil, log),
		beaconBlock:    beaconBlock,
		log:            log,
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

				blk, err := blocks.NewSignedBeaconBlock(block.GetBellatrixBlock())
				if err != nil {
					c.log.Errorf("could not wrap signed beacon block: %v", err)
					continue
				}

				blockHash, err := blk.Block().HashTreeRoot()
				if err != nil {
					c.log.Errorf("could not get beacon block[slot=%d] hash: %v", blk.Block().Slot(), err)
					continue
				}
				blockHashHex := ethcommon.BytesToHash(blockHash[:]).String()

				execution, err := blk.Block().Body().Execution()
				if err != nil {
					c.log.Errorf("could not get block[slot=%d,hash=%s] execution: %v", blk.Block().Slot(), blockHashHex, err)
					continue
				}

				// If it pre-merge state execution is empty
				if !c.beaconBlock && execution.BlockNumber() == 0 {
					c.log.Tracef("skip eth1 block[slot=%d,hash=%s] for pre-merge", blk.Block().Slot(), blockHashHex)
					continue
				}

				if err := c.blockProcessor.ProcessBlockchainBlock(c.log, c.endpoint, blk); err != nil {
					c.log.Errorf("could not proccess beacon block[slot=%d,hash=%s] to eth: %v", blk.Block().Slot(), blockHashHex, err)
					continue
				}

				c.log.Debugf("eth2 block[slot=%d,hash=%s] sent to BDN", blk.Block().Slot(), blockHashHex)
			}
		}()

		time.Sleep(prysmClientTimeout)
	}
}
