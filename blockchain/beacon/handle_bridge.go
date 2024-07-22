package beacon

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// HandleBDNBeaconMessages waits for beacon messages from BDN and broadcast it to the connected nodes
func HandleBDNBeaconMessages(ctx context.Context, b blockchain.Bridge, n *Node, blobsManager *BlobSidecarCacheManager) {
	broadcastP2P := n != nil

	for {
		select {
		case beaconMessage := <-b.ReceiveBeaconMessageFromBDN():
			switch beaconMessage.Type {
			case types.BxBeaconMessageTypeEthBlob:
				convertedMessage, err := b.BeaconMessageBDNToBlockchain(beaconMessage)
				if err != nil {
					log.Errorf("failed to convert BDN blob to beacon blob: %v", err)
					continue
				}

				blobSidecar, ok := convertedMessage.(*ethpb.BlobSidecar)
				if !ok {
					log.Errorf("failed to convert BDN blob to beacon blob: %v", err)
					continue
				}

				if broadcastP2P {
					go func() {
						if err := n.BroadcastBlob(blobSidecar); err != nil {
							log.Errorf("failed to broadcast blob sidecar to P2P connections: %v", err)
						} else {
							n.log.Tracef("Broadcasted blob sidecar message, index %v, block hash: %v, kzg commitment: %v", blobSidecar.Index, hex.EncodeToString(beaconMessage.BlockHash[:]), hex.EncodeToString(blobSidecar.KzgCommitment[:]))
						}
					}()
				}
			default:
				log.Warnf("received message unknown beacon message from BDN: %v", beaconMessage.Type)
			}
		case <-ctx.Done():
			log.Infof("ending handleBDNBlobsSidecarBridge")
			return
		}
	}
}

// HandleBDNBlocks waits for block from BDN and broadcast it to the connected nodes
func HandleBDNBlocks(ctx context.Context, b blockchain.Bridge, n *Node, beaconAPIClients []*APIClient, blobsManager *BlobSidecarCacheManager) {
	broadcastP2P := n != nil
	broadcastBeaconAPI := len(beaconAPIClients) > 0

	for {
		select {
		case bdnBlock := <-b.ReceiveBeaconBlockFromBDN():
			beaconBlock, err := b.BlockBDNtoBlockchain(bdnBlock)
			if err != nil {
				log.Errorf("failed to convert BDN block to beacon block: %v", err)
				continue
			}
			castedBlock := beaconBlock.(interfaces.ReadOnlySignedBeaconBlock)

			if broadcastP2P {
				go func() {
					if err := n.BroadcastBlock(castedBlock); err != nil {
						log.Errorf("failed to broadcast block to p2p connection, block_hash: %v, err: %v", bdnBlock.Hash(), err)
					} else {
						log.Tracef("broadcasted block to blockchain: p2p, block_hash: %v", bdnBlock.Hash())
					}
				}()

			}

			if broadcastBeaconAPI {
				go func() {
					blockHash := bdnBlock.BeaconHash().String()

					denebBlockContents, err := fillBlockContents(blobsManager, castedBlock, blockHash)
					if err != nil {
						log.Errorf("failed to fill block contents for block %s: %v, skipping block broadcasting", blockHash, err)
						return
					}

					for _, client := range beaconAPIClients {
						go func(client *APIClient) {
							if err := client.BroadcastBlock(denebBlockContents); err != nil {
								log.Errorf("failed to broadcast block to Beacon API endpoint %s, block hash: %v, err %v", client.URL, blockHash, err)
							} else {
								log.Debugf("broadcasted block to Beacon API endpoint: %v, block hash: %v", client.URL, blockHash)
							}
						}(client)
					}
				}()
			}
		case <-ctx.Done():
			log.Infof("ending handleBDNBlocksBridge")
			return
		}
	}
}

func fillBlockContents(blobsManager *BlobSidecarCacheManager, castedBlock interfaces.ReadOnlySignedBeaconBlock, blockHash string) (*ethpb.SignedBeaconBlockContentsDeneb, error) {
	denebBlock, err := castedBlock.PbDenebBlock()
	if err != nil {
		return nil, fmt.Errorf("failed to convert block to Deneb block: %v", err)
	}

	kzgCommits := denebBlock.Block.Body.BlobKzgCommitments
	expectedBlobSidecarAmount := len(kzgCommits)
	receivedBlobSidecarAmount := 0

	denebBlockContents := &ethpb.SignedBeaconBlockContentsDeneb{
		Block:     denebBlock,
		KzgProofs: make([][]byte, expectedBlobSidecarAmount),
		Blobs:     make([][]byte, expectedBlobSidecarAmount),
	}

	if expectedBlobSidecarAmount == 0 {
		log.Tracef("no blob sidecars expected for block hash %s, broadcasting", blockHash)
		return denebBlockContents, nil
	}

	log.Tracef("waiting for %d blob sidecars for block hash %s", expectedBlobSidecarAmount, blockHash)

	blobsCh := blobsManager.SubscribeToBlobByBlockHash(blockHash, castedBlock.Block().Slot())
	waitingStartingTime := time.Now()

	for blob := range blobsCh {
		log.Tracef("received blob sidecar for block hash %s, index: %d", blockHash, blob.Index)

		if blob.Index >= uint64(expectedBlobSidecarAmount) {
			log.Warnf("received blob sidecar with bigger index than expected, index: %v, expected max: %v", blob.Index, expectedBlobSidecarAmount-1)
			continue
		}

		blockCommitment := bytesutil.ToBytes48(kzgCommits[blob.Index])
		blobCommitment := bytesutil.ToBytes48(blob.KzgCommitment)
		if blobCommitment != blockCommitment {
			log.Warnf("commitment %#x != block commitment %#x, at index %d for block hash %#x at slot %d ", blobCommitment, blockCommitment, blob.Index, blockHash, blob.SignedBlockHeader.Header.Slot)
			continue
		}

		receivedBlobSidecarAmount++
		// only success case

		denebBlockContents.KzgProofs[blob.Index] = blob.KzgProof
		denebBlockContents.Blobs[blob.Index] = blob.Blob

		if receivedBlobSidecarAmount == expectedBlobSidecarAmount {
			break
		}
	}

	blobsManager.UnsubscribeFromBlobByBlockHash(blockHash)

	if receivedBlobSidecarAmount != expectedBlobSidecarAmount {
		// not all blob sidecars were received and the block should not be broadcasted
		// also it means that received channel for blobs was closed, so we don't need to unsubscribe
		return nil, fmt.Errorf("received %d blob sidecars, expected %d", receivedBlobSidecarAmount, expectedBlobSidecarAmount)
	}

	waitedTime := time.Since(waitingStartingTime).Milliseconds()

	log.Tracef("received all %d blob sidecars for block hash %s, waited %d ms", expectedBlobSidecarAmount, blockHash, waitedTime)

	return denebBlockContents, nil
}
