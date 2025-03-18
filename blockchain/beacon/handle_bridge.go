package beacon

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	prysmTypes "github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
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
				go broadcastToClients(bdnBlock.Hash().String(), castedBlock, beaconAPIClients, blobsManager)
			}
		case <-ctx.Done():
			log.Infof("ending handleBDNBlocksBridge")
			return
		}
	}
}

func broadcastToClients(blockHash string, castedBlock interfaces.ReadOnlySignedBeaconBlock, beaconAPIClients []*APIClient, blobsManager *BlobSidecarCacheManager) {
	castedBlock.Block().Slot()

	blockContents, ethConsensusVersion, err := fillBlockContents(blobsManager, castedBlock, blockHash)
	if err != nil {
		log.Errorf("failed to fill block contents for block %s: %v, skipping block broadcasting", blockHash, err)
		return
	}

	for _, client := range beaconAPIClients {
		go func(client *APIClient) {
			shouldBroadcast, err := client.shouldBroadcastBlock(uint64(castedBlock.Block().Slot()))
			if err != nil {
				log.Errorf("failed to broadcast block to Beacon API endpoint %s: %v", client.URL, err)
				return
			}

			if !shouldBroadcast {
				return
			}

			if err := client.BroadcastBlock(blockContents, ethConsensusVersion); err != nil {
				log.Errorf("failed to broadcast block to Beacon API endpoint %s, block hash: %v, err %v", client.URL, blockHash, err)
			} else {
				log.Debugf("broadcasted block to Beacon API endpoint: %v, block hash: %v", client.URL, blockHash)
			}
		}(client)
	}
}

func fillBlockContents(blobsManager *BlobSidecarCacheManager, castedBlock interfaces.ReadOnlySignedBeaconBlock, blockHash string) (ssz.Marshaler, string, error) {
	bp, err := castedBlock.Proto()
	if err != nil {
		return nil, "", fmt.Errorf("failed to convert block to proto message: %v", err)
	}

	switch block := bp.(type) {
	case *ethpb.SignedBeaconBlockDeneb:
		kzgCommits := block.Block.Body.BlobKzgCommitments
		expectedBlobSidecarAmount := len(kzgCommits)

		denebBlockContents := &ethpb.SignedBeaconBlockContentsDeneb{
			Block:     block,
			KzgProofs: make([][]byte, expectedBlobSidecarAmount),
			Blobs:     make([][]byte, expectedBlobSidecarAmount),
		}

		if expectedBlobSidecarAmount == 0 {
			log.Tracef("no blob sidecars expected for block hash %s, broadcasting", blockHash)
			return denebBlockContents, version.String(version.Deneb), nil
		}

		kzgProofs, blobs, err := proofsAndBlobs(blobsManager, kzgCommits, blockHash, castedBlock.Block().Slot(), uint64(expectedBlobSidecarAmount))
		if err != nil {
			return nil, "", err
		}

		denebBlockContents.KzgProofs = kzgProofs
		denebBlockContents.Blobs = blobs

		return denebBlockContents, version.String(version.Deneb), nil
	case *ethpb.SignedBeaconBlockElectra:
		kzgCommits := block.Block.Body.BlobKzgCommitments
		expectedBlobSidecarAmount := len(kzgCommits)

		electraBlockContents := &ethpb.SignedBeaconBlockContentsElectra{
			Block:     block,
			KzgProofs: make([][]byte, expectedBlobSidecarAmount),
			Blobs:     make([][]byte, expectedBlobSidecarAmount),
		}

		if expectedBlobSidecarAmount == 0 {
			log.Tracef("no blob sidecars expected for block hash %s, broadcasting", blockHash)
			return electraBlockContents, version.String(version.Electra), nil
		}

		kzgProofs, blobs, err := proofsAndBlobs(blobsManager, kzgCommits, blockHash, castedBlock.Block().Slot(), uint64(expectedBlobSidecarAmount))
		if err != nil {
			return nil, "", err
		}

		electraBlockContents.KzgProofs = kzgProofs
		electraBlockContents.Blobs = blobs
		return electraBlockContents, version.String(version.Electra), nil
	default:
		return nil, "", fmt.Errorf("unrecognized block type: %T", block)
	}
}

func proofsAndBlobs(blobsManager *BlobSidecarCacheManager, kzgCommits [][]byte, blockHash string, slot prysmTypes.Slot, expectedBlobSidecarAmount uint64) (kzgProofs, blobs [][]byte, err error) {
	kzgProofs = make([][]byte, expectedBlobSidecarAmount)
	blobs = make([][]byte, expectedBlobSidecarAmount)
	receivedBlobSidecarAmount := uint64(0)

	log.Tracef("waiting for %d blob sidecars for block hash %s", expectedBlobSidecarAmount, blockHash)

	waitingStartingTime := time.Now()
	blobsCh := blobsManager.SubscribeToBlobByBlockHash(blockHash, slot)

	for blob := range blobsCh {
		log.Tracef("received blob sidecar for block hash %s, index: %d", blockHash, blob.Index)

		if blob.Index >= expectedBlobSidecarAmount {
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

		kzgProofs[blob.Index] = blob.KzgProof
		blobs[blob.Index] = blob.Blob

		if receivedBlobSidecarAmount == expectedBlobSidecarAmount {
			break
		}
	}

	blobsManager.UnsubscribeFromBlobByBlockHash(blockHash)

	if receivedBlobSidecarAmount != expectedBlobSidecarAmount {
		// not all blob sidecars were received and the block should not be broadcasted
		// also it means that received channel for blobs was closed, so we don't need to unsubscribe
		err = fmt.Errorf("received %d blob sidecars, expected %d", receivedBlobSidecarAmount, expectedBlobSidecarAmount)

		return
	}

	log.Tracef("received all %d blob sidecars for block hash %s, waited %d ms", expectedBlobSidecarAmount, blockHash, time.Since(waitingStartingTime).Milliseconds())

	return
}
