package services

import (
	"fmt"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

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

func (bp *blobProcessor) propagateEthBlobSidecarToBlobManager(ethBlobColumnSidecar *ethpb.DataColumnSidecar) {
	if bp.blobsManager != nil {
		// blobsManager can be nil in some cases, be careful
		err := bp.blobsManager.AddBlobSidecar(ethBlobColumnSidecar)
		if err != nil {
			log.Warnf("failed to add blob sidecar: %v", err)
			// We don't return an error here because we can still broadcast the message even if blobsManager fails
			// Warn log is enough to notify that something went wrong
		}
	}
}

func (bp *blobProcessor) BxBeaconMessageToBeaconMessage(bxBeaconMessage *types.BxBeaconMessage, networkNum bxtypes.NetworkNum) (*bxmessage.BeaconMessage, error) {
	// TODO: implement compression
	switch bxBeaconMessage.Type {
	case types.BxBeaconMessageTypeEthDataColumn:
		if bp.blobsManager != nil {
			sidecar := new(ethpb.DataColumnSidecar)
			if err := sidecar.UnmarshalSSZ(bxBeaconMessage.Data); err != nil {
				return nil, fmt.Errorf("failed to unmarshal eth blob sidecar: %v", err)
			}

			bp.propagateEthBlobSidecarToBlobManager(sidecar)
		}

		return bxmessage.NewBeaconMessage(
			bxBeaconMessage.Hash,
			bxBeaconMessage.BlockHash,
			bxBeaconMessage.Type,
			bxBeaconMessage.Data,
			bxBeaconMessage.Index,
			bxBeaconMessage.Slot,
			networkNum,
		), nil
	default:
		return nil, fmt.Errorf("invalid beacon message type %s", bxBeaconMessage.Type)
	}
}

func (bp *blobProcessor) BeaconMessageToBxBeaconMessage(beaconMessage *bxmessage.BeaconMessage) (*types.BxBeaconMessage, error) {
	var resultBxBeaconMessage *types.BxBeaconMessage

	switch beaconMessage.Type() {
	case types.BxBeaconMessageTypeEthDataColumn:
		resultBxBeaconMessage = types.NewBxBeaconMessage(
			beaconMessage.Hash(),
			beaconMessage.BlockHash(),
			beaconMessage.Type(),
			beaconMessage.Data(),
			beaconMessage.Index(),
			beaconMessage.Slot(),
		)

		if bp.blobsManager != nil {
			dataColumnSidecar := new(ethpb.DataColumnSidecar)
			if err := dataColumnSidecar.UnmarshalSSZ(beaconMessage.Data()); err != nil {
				return nil, fmt.Errorf("failed to unmarshal eth blob sidecar: %v", err)
			}
			bp.propagateEthBlobSidecarToBlobManager(dataColumnSidecar)
		}
	default:
		return nil, fmt.Errorf("invalid beacon message type %s", beaconMessage.Type())
	}

	return resultBxBeaconMessage, nil
}
