package beacon

import (
	"context"
	"sync"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/prysmaticlabs/prysm/v4/consensus-types/interfaces"
)

// HandleBDNBlocksBridge waits for block from BDN and broadcast it to the connected nodes using P2P and Beacon API
func HandleBDNBlocksBridge(ctx context.Context, b blockchain.Bridge, n *Node, beaconAPIClients []*APIClient) {
	broadcastP2P := n != nil
	broadcastBeaconAPI := len(beaconAPIClients) > 0

	for {
		select {
		case bdnBlock := <-b.ReceiveBeaconBlockFromBDN():
			beaconBlock, err := b.BlockBDNtoBlockchain(bdnBlock)
			if err != nil {
				log.Errorf("could not convert BDN block to beacon block: %v", err)
				continue
			}
			castedBlock := beaconBlock.(interfaces.ReadOnlySignedBeaconBlock)

			var wg sync.WaitGroup

			if broadcastP2P {
				wg.Add(1)

				go func() {
					defer wg.Done()
					if err := n.BroadcastBlock(castedBlock); err != nil {
						log.Errorf("could not broadcast block to p2p connection, block_hash: %v, err: %v", bdnBlock.Hash(), err)
					} else {
						log.Tracef("broadcasted block to blockchain: p2p, block_hash: %v", bdnBlock.Hash())
					}
				}()

			}

			if broadcastBeaconAPI {
				for _, client := range beaconAPIClients {
					wg.Add(1)

					go func(client *APIClient) {
						defer wg.Done()
						if err := client.BroadcastBlock(castedBlock); err != nil {
							log.Errorf("could not broadcast block to beacon API endpoint %s, block hash: %v, err %v", client.URL, bdnBlock.Hash(), err)
						} else {
							log.Tracef("broadcasted block to blockchain: beacon API :%v, block_hash: %v", client.URL, bdnBlock.Hash())
						}
					}(client)
				}
			}

			wg.Wait()
		case <-ctx.Done():
			log.Infof("ending handleBDNBlocksBridge")
			return
		}
	}
}
