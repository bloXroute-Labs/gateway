package beacon

import (
	"context"
	"encoding/hex"
	"fmt"
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	ssz "github.com/prysmaticlabs/fastssz"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

func init() {
	if err := kzg.Start(); err != nil {
		panic(err)
	}
}

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
			case types.BxBeaconMessageTypeEthDataColumn:
				convertedMessage, err := b.BeaconMessageBDNToBlockchain(beaconMessage)
				if err != nil {
					log.Errorf("failed to convert BDN blob to beacon blob: %v", err)
					continue
				}

				blobSidecar, ok := convertedMessage.(*ethpb.DataColumnSidecar)
				if !ok {
					log.Errorf("failed to convert BDN blob to beacon blob: %v", err)
					continue
				}

				if broadcastP2P {
					go func() {
						if err := n.BroadcastDataColumn(blobSidecar); err != nil {
							if err == errNoPeersFoundToBroadcast {
								return
							}
							log.Errorf("failed to broadcast data column sidecar to P2P connections: %v", err)
						} else {
							n.log.Tracef("Broadcasted data column sidecar message, index %v, block hash: %v", blobSidecar.Index, hex.EncodeToString(beaconMessage.BlockHash[:]))
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
	case *ethpb.SignedBeaconBlockFulu:
		expectedBlobSidecarAmount := len(block.Block.Body.BlobKzgCommitments)

		fuluBlockContents := &ethpb.SignedBeaconBlockContentsFulu{
			Block:     block,
			KzgProofs: make([][]byte, expectedBlobSidecarAmount),
			Blobs:     make([][]byte, expectedBlobSidecarAmount),
		}

		if expectedBlobSidecarAmount == 0 {
			log.Tracef("no blob sidecars expected for block hash %s, broadcasting", blockHash)
			return fuluBlockContents, version.String(version.Fulu), nil
		}

		kzgProofs, blobs, err := retrieveBlobsFromDataColumns(blobsManager, castedBlock, blockHash)
		if err != nil {
			return nil, "", err
		}

		fuluBlockContents.KzgProofs = kzgProofs
		fuluBlockContents.Blobs = blobs

		return fuluBlockContents, version.String(version.Fulu), nil
	default:
		return nil, "", fmt.Errorf("unrecognized block type: %T", block)
	}
}

func retrieveBlobsFromDataColumns(blobsManager *BlobSidecarCacheManager, block interfaces.ReadOnlySignedBeaconBlock, blockHash string) (kzgProofs, blobs [][]byte, err error) {
	kgzCommitements, err := block.Block().Body().BlobKzgCommitments()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get KZG commitments from block body: %v", err)
	}

	expectedBlobs := len(kgzCommitements)

	kzgProofs = make([][]byte, 0, len(kgzCommitements))
	blobs = make([][]byte, 0, len(kgzCommitements))
	var receivedBlobSidecarAmount int

	log.Tracef("waiting for %d blob sidecars for block hash %s", expectedBlobs, blockHash)

	waitingStartingTime := time.Now()
	blobsCh := blobsManager.SubscribeToBlobByBlockHash(blockHash, block.Block().Slot())

	columnSidecar := make([]blocks.VerifiedRODataColumn, 0)
	for blob := range blobsCh {
		log.Tracef("received blob sidecar for block hash %s, index: %d", blockHash, blob.Index)

		roDataColumn, err := blocks.NewRODataColumn(blob)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create RODataColumn from blob sidecar: %v", err)
		}

		receivedBlobSidecarAmount++

		columnSidecar = append(columnSidecar, blocks.NewVerifiedRODataColumn(roDataColumn))

		minColumnCount := peerdas.MinimumColumnCountToReconstruct()
		if receivedBlobSidecarAmount >= 0 && uint64(receivedBlobSidecarAmount) == minColumnCount {
			break
		}
	}

	minColumnCount := peerdas.MinimumColumnCountToReconstruct()
	if receivedBlobSidecarAmount < 0 || uint64(receivedBlobSidecarAmount) < minColumnCount {
		return nil, nil, fmt.Errorf("received only %d blob sidecars, need at least %d to reconstruct blobs", receivedBlobSidecarAmount, peerdas.MinimumColumnCountToReconstruct())
	}

	slices.SortFunc(columnSidecar, func(a, b blocks.VerifiedRODataColumn) int {
		//nolint:gosec
		// G115: safe, max value is 128
		return int(a.GetIndex() - b.GetIndex())
	})

	roBlock, err := blocks.NewROBlock(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create ROBlock from block: %v", err)
	}

	indexes := make([]int, 0, len(kgzCommitements))
	for i := range kgzCommitements {
		indexes = append(indexes, i)
	}

	verifiedBlobs, err := peerdas.ReconstructBlobs(roBlock, columnSidecar, indexes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to reconstruct blobs from column sidecars: %v", err)
	}

	// Extract blobs from verified blobs
	for _, verifiedBlob := range verifiedBlobs {
		blobs = append(blobs, verifiedBlob.GetBlob())
	}

	// Prysm expects flat array of cell proofs: [blob0_cell0_proof, blob0_cell1_proof, ..., blob1_cell0_proof, ...]
	for _, blobBytes := range blobs {
		var kzgBlob kzg.Blob
		if copy(kzgBlob[:], blobBytes) != len(kzgBlob) {
			return nil, nil, fmt.Errorf("wrong blob size during cell proof computation")
		}

		// Compute cells and their KZG proofs for this blob
		cellsAndProofs, err := kzg.ComputeCellsAndKZGProofs(&kzgBlob)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to compute cells and proofs for blob: %v", err)
		}

		// Append all cell proofs for this blob (128 proofs)
		for _, cellProof := range cellsAndProofs.Proofs {
			kzgProofs = append(kzgProofs, cellProof[:])
		}
	}

	blobsManager.UnsubscribeFromBlobByBlockHash(blockHash)

	if len(blobs) != expectedBlobs {
		// not all blob sidecars were received and the block should not be broadcasted
		// also it means that received channel for blobs was closed, so we don't need to unsubscribe
		err = fmt.Errorf("received %d blob sidecars, expected %d", receivedBlobSidecarAmount, expectedBlobs)

		return
	}

	log.Tracef("received all %d blob for block hash %s, waited %d ms", expectedBlobs, blockHash, time.Since(waitingStartingTime).Milliseconds())

	return
}
