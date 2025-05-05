package types

import (
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	ssz "github.com/prysmaticlabs/fastssz"
)

// CompressedEthBlobSidecar is a struct that contains same data as ethpb.BlobSidecar
// but with different blob size, what is needed to provide BDN compression
type CompressedEthBlobSidecar struct {
	Index                    uint64                         `protobuf:"varint,1,opt,name=index,proto3" json:"index,omitempty"`
	TxHash                   []byte                         `protobuf:"bytes,2,opt,name=blob,proto3" json:"tx_hash,omitempty" ssz-size:"32"`
	KzgCommitment            []byte                         `protobuf:"bytes,3,opt,name=kzg_commitment,json=kzgCommitment,proto3" json:"kzg_commitment,omitempty" ssz-size:"48"`
	KzgProof                 []byte                         `protobuf:"bytes,4,opt,name=kzg_proof,json=kzgProof,proto3" json:"kzg_proof,omitempty" ssz-size:"48"`
	SignedBlockHeader        *ethpb.SignedBeaconBlockHeader `protobuf:"bytes,5,opt,name=signed_block_header,json=signedBlockHeader,proto3" json:"signed_block_header,omitempty"`
	CommitmentInclusionProof [][]byte                       `protobuf:"bytes,6,rep,name=commitment_inclusion_proof,json=commitmentInclusionProof,proto3" json:"commitment_inclusion_proof,omitempty" ssz-size:"17,32"`
}

const (
	fullBlobSize       = 131072
	compressedBlobSize = 32
	blobSizeDiff       = fullBlobSize - compressedBlobSize
)

// MarshalSSZTo ssz marshals the CompressedEthBlobSidecar object to a target array
func (b *CompressedEthBlobSidecar) MarshalSSZTo(buf []byte) (dst []byte, err error) {
	dst = buf

	// Field (0) 'Index'
	dst = ssz.MarshalUint64(dst, b.Index)

	// Field (1) 'TxHash'
	if size := len(b.TxHash); size != compressedBlobSize {
		err = ssz.ErrBytesLengthFn("--.TxHash", size, compressedBlobSize)
		return
	}
	dst = append(dst, b.TxHash...)

	// Field (2) 'KzgCommitment'
	if size := len(b.KzgCommitment); size != 48 {
		err = ssz.ErrBytesLengthFn("--.KzgCommitment", size, 48)
		return
	}
	dst = append(dst, b.KzgCommitment...)

	// Field (3) 'KzgProof'
	if size := len(b.KzgProof); size != 48 {
		err = ssz.ErrBytesLengthFn("--.KzgProof", size, 48)
		return
	}
	dst = append(dst, b.KzgProof...)

	// Field (4) 'SignedBlockHeader'
	if b.SignedBlockHeader == nil {
		b.SignedBlockHeader = new(ethpb.SignedBeaconBlockHeader)
	}
	if dst, err = b.SignedBlockHeader.MarshalSSZTo(dst); err != nil {
		return
	}

	// Field (5) 'CommitmentInclusionProof'
	if size := len(b.CommitmentInclusionProof); size != 17 {
		err = ssz.ErrVectorLengthFn("--.CommitmentInclusionProof", size, 17)
		return
	}
	for ii := 0; ii < 17; ii++ {
		if size := len(b.CommitmentInclusionProof[ii]); size != 32 {
			err = ssz.ErrBytesLengthFn("--.CommitmentInclusionProof[ii]", size, 32)
			return
		}
		dst = append(dst, b.CommitmentInclusionProof[ii]...)
	}

	return
}

// UnmarshalSSZ ssz unmarshals the CompressedEthBlobSidecar object from a source array
func (b *CompressedEthBlobSidecar) UnmarshalSSZ(buf []byte) error {

	var err error
	size := uint64(len(buf))
	if size != 131928-blobSizeDiff {
		return ssz.ErrSize
	}

	// Field (0) 'Index'
	b.Index = ssz.UnmarshallUint64(buf[0:8])

	// Field (1) 'TxHash'
	if cap(b.TxHash) == 0 {
		b.TxHash = make([]byte, 0, len(buf[8:131080-blobSizeDiff]))
	}
	b.TxHash = append(b.TxHash, buf[8:131080-blobSizeDiff]...)

	// Field (2) 'KzgCommitment'
	if cap(b.KzgCommitment) == 0 {
		b.KzgCommitment = make([]byte, 0, len(buf[131080-blobSizeDiff:131128-blobSizeDiff]))
	}
	b.KzgCommitment = append(b.KzgCommitment, buf[131080-blobSizeDiff:131128-blobSizeDiff]...)

	// Field (3) 'KzgProof'
	if cap(b.KzgProof) == 0 {
		b.KzgProof = make([]byte, 0, len(buf[131128-blobSizeDiff:131176-blobSizeDiff]))
	}
	b.KzgProof = append(b.KzgProof, buf[131128-blobSizeDiff:131176-blobSizeDiff]...)

	// Field (4) 'SignedBlockHeader'
	if b.SignedBlockHeader == nil {
		b.SignedBlockHeader = new(ethpb.SignedBeaconBlockHeader)
	}
	if err = b.SignedBlockHeader.UnmarshalSSZ(buf[131176-blobSizeDiff : 131384-blobSizeDiff]); err != nil {
		return err
	}

	// Field (5) 'CommitmentInclusionProof'
	b.CommitmentInclusionProof = make([][]byte, 17)
	for ii := 0; ii < 17; ii++ {
		if cap(b.CommitmentInclusionProof[ii]) == 0 {
			b.CommitmentInclusionProof[ii] = make([]byte, 0, len(buf[131384-blobSizeDiff : 131928-blobSizeDiff][ii*32:(ii+1)*32]))
		}
		b.CommitmentInclusionProof[ii] = append(b.CommitmentInclusionProof[ii], buf[131384-blobSizeDiff : 131928-blobSizeDiff][ii*32:(ii+1)*32]...)
	}

	return err
}

// MarshalSSZ ssz marshals the CompressedEthBlobSidecar object to a new byte array
func (b *CompressedEthBlobSidecar) MarshalSSZ() ([]byte, error) {
	return ssz.MarshalSSZ(b)
}

// SizeSSZ returns the size of the CompressedEthBlobSidecar object in bytesg
func (b *CompressedEthBlobSidecar) SizeSSZ() (size int) {
	size = 131928 - blobSizeDiff
	return
}
