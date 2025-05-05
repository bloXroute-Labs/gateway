package services

import (
	"bytes"
	"encoding/hex"
	"fmt"

	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/beacon"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// BlobProcessor is responsible for processing blob sidecars from BDN and to BDN
type BlobProcessor interface {
	BeaconMessageToBxBeaconMessage(beaconMessage *bxmessage.BeaconMessage) (*types.BxBeaconMessage, error)
	BxBeaconMessageToBeaconMessage(bxBeaconMessage *types.BxBeaconMessage, networkNum bxtypes.NetworkNum) (*bxmessage.BeaconMessage, error)
}

type blobProcessor struct {
	txStore      TxStore
	blobsManager *beacon.BlobSidecarCacheManager
}

// NewBlobProcessor creates a new BlobProcessor object with the provided TxStore and BlobSidecarCacheManager
func NewBlobProcessor(txStore TxStore, blobsManager *beacon.BlobSidecarCacheManager) BlobProcessor {
	return &blobProcessor{
		txStore:      txStore,
		blobsManager: blobsManager,
	}
}

func (bp *blobProcessor) propagateEthBlobSidecarToBlobManager(ethBlobSidecar *ethpb.BlobSidecar) {
	if bp.blobsManager != nil {
		// blobsManager can be nil in some cases, be careful
		err := bp.blobsManager.AddBlobSidecar(ethBlobSidecar)
		if err != nil {
			log.Warnf("failed to add blob sidecar: %v", err)
			// We don't return an error here because we can still broadcast the message even if blobsManager fails
			// Warn log is enough to notify that something went wrong
		}
	}
}

func (bp *blobProcessor) compressedBlobSidecarToEthBlobSidecar(blob *types.CompressedEthBlobSidecar, fullBlob []byte) *ethpb.BlobSidecar {
	return &ethpb.BlobSidecar{
		Index:                    blob.Index,
		Blob:                     fullBlob,
		KzgCommitment:            blob.KzgCommitment,
		KzgProof:                 blob.KzgProof,
		SignedBlockHeader:        blob.SignedBlockHeader,
		CommitmentInclusionProof: blob.CommitmentInclusionProof,
	}
}

func (bp *blobProcessor) ethBlobSidecarToCompressedBlobSidecar(ethBlobSidecar *ethpb.BlobSidecar, txHash []byte) *types.CompressedEthBlobSidecar {
	return &types.CompressedEthBlobSidecar{
		Index:                    ethBlobSidecar.GetIndex(),
		TxHash:                   txHash,
		KzgCommitment:            ethBlobSidecar.GetKzgCommitment(),
		KzgProof:                 ethBlobSidecar.GetKzgProof(),
		SignedBlockHeader:        ethBlobSidecar.GetSignedBlockHeader(),
		CommitmentInclusionProof: ethBlobSidecar.GetCommitmentInclusionProof(),
	}
}

func (bp *blobProcessor) compressForBDN(bxBeaconMessage *types.BxBeaconMessage) (*types.BxBeaconMessage, error) {
	sidecar := new(ethpb.BlobSidecar)
	if err := sidecar.UnmarshalSSZ(bxBeaconMessage.Data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal eth blob sidecar: %v", err)
	}

	bp.propagateEthBlobSidecarToBlobManager(sidecar)

	kzg := hex.EncodeToString(sidecar.KzgCommitment)
	bxTx, err := bp.txStore.GetTxByKzgCommitment(kzg)
	if err != nil {
		log.Tracef("not found tx for kzg commitment %s, broadcasting original eth sidecar: %v", kzg, err)
		return bxBeaconMessage, nil
	}

	compressedSidecar := bp.ethBlobSidecarToCompressedBlobSidecar(sidecar, bxTx.Hash().Bytes())

	newSidecarData, err := compressedSidecar.MarshalSSZ()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal eth blob sidecar: %v", err)
	}

	log.Tracef("successfully compressed eth blob sidecar, kzg commitment: %v", hex.EncodeToString(compressedSidecar.KzgCommitment))

	return types.NewBxBeaconMessage(
		bxBeaconMessage.Hash,
		bxBeaconMessage.BlockHash,
		types.BxBeaconMessageTypeCompressedEthBlob, // New compressed type
		newSidecarData,
		bxBeaconMessage.Index,
		bxBeaconMessage.Slot,
	), nil
}

func findBlobValueByKzgCommitmentFromBlobTxSidecar(targetCommitment []byte, sidecar *ethtypes.BlobTxSidecar) ([]byte, bool) {
	for i, commitment := range sidecar.Commitments {
		if bytes.Equal(targetCommitment, commitment[:]) {
			return sidecar.Blobs[i][:], true
		}
	}

	return nil, false
}

func (bp *blobProcessor) decompressFromBDN(beaconMessage *bxmessage.BeaconMessage) (*types.BxBeaconMessage, error) {
	sidecarFromBroadcast := new(types.CompressedEthBlobSidecar)
	if err := sidecarFromBroadcast.UnmarshalSSZ(beaconMessage.Data()); err != nil {
		return nil, fmt.Errorf("failed to unmarshal compressed eth blob sidecar: %v", err)
	}

	compressedBlobTxHash, err := types.NewSHA256Hash(sidecarFromBroadcast.TxHash)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate compressed tx hash: %v", err)
	}

	bxTx, ok := bp.txStore.Get(compressedBlobTxHash)
	if !ok {
		return nil, fmt.Errorf("failed to find tx by compressed hash: %v", compressedBlobTxHash)
	}

	var ethTx ethtypes.Transaction
	err = rlp.DecodeBytes(bxTx.Content(), &ethTx)
	if err != nil {
		return nil, fmt.Errorf("failed to decode tx: %v", err)
	}

	if ethTx.BlobTxSidecar() == nil {
		return nil, fmt.Errorf("tx does not have blob sidecar")
	}

	if len(ethTx.BlobTxSidecar().Commitments) == 0 {
		return nil, fmt.Errorf("tx blob sidecar does not have kzg commitments")
	}

	if len(ethTx.BlobTxSidecar().Blobs) == 0 {
		return nil, fmt.Errorf("tx blob sidecar does not have blobs")
	}

	blob, ok := findBlobValueByKzgCommitmentFromBlobTxSidecar(sidecarFromBroadcast.KzgCommitment, ethTx.BlobTxSidecar())

	if !ok {
		return nil, fmt.Errorf("failed to find blob value for commitment: %v", sidecarFromBroadcast.KzgCommitment)
	}

	ethSidecar := bp.compressedBlobSidecarToEthBlobSidecar(sidecarFromBroadcast, blob)

	bp.propagateEthBlobSidecarToBlobManager(ethSidecar)

	uncompressedSidecarBytes, err := ethSidecar.MarshalSSZ()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal eth blob sidecar: %v", err)
	}

	log.Tracef("successfully decompressed blob sidecar, kzg commitment: %v", hex.EncodeToString(ethSidecar.KzgCommitment))

	return &types.BxBeaconMessage{
		Hash:      beaconMessage.Hash(),
		BlockHash: beaconMessage.BlockHash(),
		Type:      types.BxBeaconMessageTypeEthBlob, // New decompressed type
		Data:      uncompressedSidecarBytes,
		Index:     beaconMessage.Index(),
		Slot:      beaconMessage.Slot(),
	}, nil
}

func (bp *blobProcessor) BxBeaconMessageToBeaconMessage(bxBeaconMessage *types.BxBeaconMessage, networkNum bxtypes.NetworkNum) (*bxmessage.BeaconMessage, error) {
	compressedBxBeaconMessage, err := bp.compressForBDN(bxBeaconMessage)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare for BDN: %v", err)
	}

	return bxmessage.NewBeaconMessage(
		compressedBxBeaconMessage.Hash,
		compressedBxBeaconMessage.BlockHash,
		compressedBxBeaconMessage.Type,
		compressedBxBeaconMessage.Data,
		compressedBxBeaconMessage.Index,
		compressedBxBeaconMessage.Slot,
		networkNum,
	), nil
}

func (bp *blobProcessor) BeaconMessageToBxBeaconMessage(beaconMessage *bxmessage.BeaconMessage) (*types.BxBeaconMessage, error) {
	var resultBxBeaconMessage *types.BxBeaconMessage
	var err error

	switch beaconMessage.Type() {
	case types.BxBeaconMessageTypeCompressedEthBlob:
		resultBxBeaconMessage, err = bp.decompressFromBDN(beaconMessage)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress from BDN: %v", err)
		}
	case types.BxBeaconMessageTypeEthBlob:
		resultBxBeaconMessage = types.NewBxBeaconMessage(
			beaconMessage.Hash(),
			beaconMessage.BlockHash(),
			beaconMessage.Type(),
			beaconMessage.Data(),
			beaconMessage.Index(),
			beaconMessage.Slot(),
		)

		if bp.blobsManager != nil {
			ethSidecar := new(ethpb.BlobSidecar)
			if err := ethSidecar.UnmarshalSSZ(beaconMessage.Data()); err != nil {
				return nil, fmt.Errorf("failed to unmarshal eth blob sidecar: %v", err)
			}
			bp.propagateEthBlobSidecarToBlobManager(ethSidecar)
		}

	}

	return resultBxBeaconMessage, nil
}
