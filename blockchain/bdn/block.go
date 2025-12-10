package bdn

import (
	"errors"

	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// ErrNotDenebBlock is returned when the block is not a Deneb block.
var (
	ErrNotDenebBlock   = errors.New("block is not Deneb block")
	ErrNotElectraBlock = errors.New("block is not Electra block")
	ErrNotFuluBlock    = errors.New("block is not Fulu block")
)

// PbGenericBlock returns a generic signed beacon block.
//
//nolint:staticcheck // GenericSignedBeaconBlock is deprecated but still required by the interface
func PbGenericBlock(b interfaces.ReadOnlySignedBeaconBlock) (*ethpb.GenericSignedBeaconBlock, error) {
	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	switch b.Version() {
	case version.Deneb:
		if b.IsBlinded() {
			return &ethpb.GenericSignedBeaconBlock{
				Block: &ethpb.GenericSignedBeaconBlock_BlindedDeneb{BlindedDeneb: pb.(*ethpb.SignedBlindedBeaconBlockDeneb)},
			}, nil
		}

		block, ok := pb.(*ethpb.SignedBeaconBlockDeneb)
		if !ok {
			return b.PbGenericBlock()
		}

		return &ethpb.GenericSignedBeaconBlock{
			Block: &ethpb.GenericSignedBeaconBlock_Deneb{
				Deneb: &ethpb.SignedBeaconBlockContentsDeneb{
					Block: block,
				},
			},
		}, nil
	case version.Electra:
		if b.IsBlinded() {
			return &ethpb.GenericSignedBeaconBlock{
				Block: &ethpb.GenericSignedBeaconBlock_BlindedElectra{BlindedElectra: pb.(*ethpb.SignedBlindedBeaconBlockElectra)},
			}, nil
		}

		block, ok := pb.(*ethpb.SignedBeaconBlockElectra)
		if !ok {
			return b.PbGenericBlock()
		}

		return &ethpb.GenericSignedBeaconBlock{
			Block: &ethpb.GenericSignedBeaconBlock_Electra{
				Electra: &ethpb.SignedBeaconBlockContentsElectra{
					Block: block,
				},
			},
		}, nil
	case version.Fulu:
		if b.IsBlinded() {
			return &ethpb.GenericSignedBeaconBlock{
				Block: &ethpb.GenericSignedBeaconBlock_BlindedFulu{BlindedFulu: pb.(*ethpb.SignedBlindedBeaconBlockFulu)},
			}, nil
		}

		block, ok := pb.(*ethpb.SignedBeaconBlockFulu)
		if !ok {
			return b.PbGenericBlock()
		}

		return &ethpb.GenericSignedBeaconBlock{
			Block: &ethpb.GenericSignedBeaconBlock_Fulu{
				Fulu: &ethpb.SignedBeaconBlockContentsFulu{
					Block: block,
				},
			},
		}, nil

	default:
		return b.PbGenericBlock()
	}
}
