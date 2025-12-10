package types

import (
	"encoding/hex"
	"os"
	"testing"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/stretchr/testify/require"
)

const (
	mockTxHash                             = "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	expectedIndex                          = (uint64)(0)
	expectedKzgCommitment                  = "97ff6a6a81f46fd20a68cfe70b57ca00fe27077b766941f6c9450e4c717f4cb63058e2b3ba947ea227092a7d93776f49"
	expectedKzgProof                       = "96566c60180e5f18068be71d45e575ac1f978602b57a481771b7d19cf1a5d7007dc140f6c5a1441d930b4e81ca362b20"
	expectedSignedBlockHeaderSlot          = uint64(8941204)
	expectedSignedBlockHeaderProposerIndex = uint64(1275098)
	expectedSignedBlockHeaderParentRoot    = "3e718330fce74463476a9429e93007e4eada936482104815348374b8b2a5bbef"
	expectedSignedBlockHeaderStateRoot     = "2394ebfd6087a169727df8cc7f28e2d902f595d45db259cad783af9d13d2ddba"
	expectedSignedBlockHeaderBodyRoot      = "4d55fc06580a1d49c5b6fd85a5c130d84ddd2496940b314d6cd5ffe4ee3202b3"
	expectedSignedBlockHeaderSignature     = "936048b89f700bee4374250b0d47a097d2f48bddead507016fcd6f4c70489d9c237ff74958779570b2ab9587bcd27eae0cf26897b82a0bf0be52f42cbd0b5eae75acc83c7ea9022664127fafc1a2186468d9c6a1c4f6f9a77553016da535a7c5"
)

var expectedCommitmentInclusionProof = [17]string{
	"fb7d227a9a5bee95f3ebcbba7409a4b2f6c47031044513c16808fa2eda352537",
	"f5a5fd42d16a20302798ef6ed309979b43003d2320d9f0e8ea9831a92759fb4b",
	"db56114e00fdd4c1f85c892bf35ac9a89289aaecb1ebd0a96cde606a748b5d71",
	"c78009fdf07fc56a11f122370658a353aaa542ed63e44c4bc15ff4cd105ab33c",
	"536d98837f2dd165a55d5eeae91485954472d56f246df256bf3cae19352a123c",
	"9efde052aa15429fae05bad4d0b1d7c64da64d03d7a1854a588c2cb8430c0d30",
	"d88ddfeed400a8755596b21942c1497e114c302e6118290f91e6772976041fa1",
	"87eb0ddba57e35f6d286673802a4af5975e22506c7cf4c64bb6be5ee11527f2c",
	"26846476fd5fc54a5d43385167c95144f2643f533cc85bb9d16b782f8d7db193",
	"506d86582d252405b840018792cad2bf1259f1ef5aa5f887e13cb2f0094f51e1",
	"ffff0ad7e659772f9534c195c815efc4014ef1e1daed4404c06385d11192e92b",
	"6cf04127db05441cd833107a52be852868890e4317e6a02ab47683aa75964220",
	"0200000000000000000000000000000000000000000000000000000000000000",
	"792930bbd5baac43bcc798ee49aa8185ef76bb3b44ba62b91d86ae569e4bb535",
	"8f4299b1172bda2dbec5bbbf43aea85905c8e7f46ee1f96bb5a4d7e712d69860",
	"db56114e00fdd4c1f85c892bf35ac9a89289aaecb1ebd0a96cde606a748b5d71",
	"b1a520c67f6fb6e356f414094164ab8555ee0b998aa059601a942057847fc419"}

func TestHandler_TestCompressedEthBlobSidecarRoundtrip(t *testing.T) {
	var sidecar ethpb.BlobSidecar
	data, err := os.ReadFile("test_data/blob.ssz")
	require.NoError(t, err)

	mockTxBytes, err := hex.DecodeString(mockTxHash)
	require.NoError(t, err)

	// Unmarshal data into the real sidecar struct
	err = sidecar.UnmarshalSSZ(data)
	require.NoError(t, err)

	// Check that the unmarshaled sidecar has the expected values
	// This test will fale in case Prysm changes the sidecar structure
	require.Equal(t, expectedIndex, sidecar.Index)
	require.Equal(t, expectedKzgCommitment, hex.EncodeToString(sidecar.KzgCommitment))
	require.Equal(t, expectedKzgProof, hex.EncodeToString(sidecar.KzgProof))
	require.Equal(t, expectedSignedBlockHeaderSlot, uint64(sidecar.SignedBlockHeader.Header.Slot))
	require.Equal(t, expectedSignedBlockHeaderProposerIndex, uint64(sidecar.SignedBlockHeader.Header.ProposerIndex))
	require.Equal(t, expectedSignedBlockHeaderParentRoot, hex.EncodeToString(sidecar.SignedBlockHeader.Header.ParentRoot))
	require.Equal(t, expectedSignedBlockHeaderStateRoot, hex.EncodeToString(sidecar.SignedBlockHeader.Header.StateRoot))
	require.Equal(t, expectedSignedBlockHeaderBodyRoot, hex.EncodeToString(sidecar.SignedBlockHeader.Header.BodyRoot))
	require.Equal(t, expectedSignedBlockHeaderSignature, hex.EncodeToString(sidecar.SignedBlockHeader.Signature))

	// encode commitment inclusion proof array to bytes array
	// and compare it with the expected array
	expectedCommitmentInclusionProofBytes := make([][]byte, 0)
	for _, proof := range expectedCommitmentInclusionProof {
		proofBytes, err := hex.DecodeString(proof)
		require.NoError(t, err)

		expectedCommitmentInclusionProofBytes = append(expectedCommitmentInclusionProofBytes, proofBytes)
	}
	require.Equal(t, expectedCommitmentInclusionProofBytes, sidecar.CommitmentInclusionProof)

	// save real blob value for the futher testing
	realBlobValue := sidecar.Blob

	// Test the flow from ethpb.BlobSidecar to CompressedEthBlobSidecar

	newSidecar := CompressedEthBlobSidecar{
		Index:                    sidecar.Index,
		TxHash:                   mockTxBytes,
		KzgCommitment:            sidecar.KzgCommitment,
		KzgProof:                 sidecar.KzgProof,
		SignedBlockHeader:        sidecar.SignedBlockHeader,
		CommitmentInclusionProof: sidecar.CommitmentInclusionProof,
	}

	// Check that MarshalSSZ works correctly for CompressedEthBlobSidecar with the custom blob size
	sidecarBytes, err := newSidecar.MarshalSSZ()
	require.NoError(t, err)

	var newSidecarCheckUnmarshal CompressedEthBlobSidecar

	// Check that UnmarshalSSZ works correctly for CompressedEthBlobSidecar for the same structure
	newSidecarCheckUnmarshal.UnmarshalSSZ(sidecarBytes)

	// Check that the unmarshaled sidecar has the expected values
	// This test will fale in case CompressedEthBlobSidecar structure changes
	require.Equal(t, expectedIndex, newSidecarCheckUnmarshal.Index)
	require.Equal(t, expectedKzgCommitment, hex.EncodeToString(newSidecarCheckUnmarshal.KzgCommitment))
	require.Equal(t, expectedKzgProof, hex.EncodeToString(newSidecarCheckUnmarshal.KzgProof))
	require.Equal(t, expectedSignedBlockHeaderSlot, uint64(newSidecarCheckUnmarshal.SignedBlockHeader.Header.Slot))
	require.Equal(t, expectedSignedBlockHeaderProposerIndex, uint64(newSidecarCheckUnmarshal.SignedBlockHeader.Header.ProposerIndex))
	require.Equal(t, expectedSignedBlockHeaderParentRoot, hex.EncodeToString(newSidecarCheckUnmarshal.SignedBlockHeader.Header.ParentRoot))
	require.Equal(t, expectedSignedBlockHeaderStateRoot, hex.EncodeToString(newSidecarCheckUnmarshal.SignedBlockHeader.Header.StateRoot))
	require.Equal(t, expectedSignedBlockHeaderBodyRoot, hex.EncodeToString(newSidecarCheckUnmarshal.SignedBlockHeader.Header.BodyRoot))
	require.Equal(t, expectedSignedBlockHeaderSignature, hex.EncodeToString(newSidecarCheckUnmarshal.SignedBlockHeader.Signature))
	require.Equal(t, expectedCommitmentInclusionProofBytes, newSidecarCheckUnmarshal.CommitmentInclusionProof)

	// [!TODO] Here should be a convertor
	// Test the flow from CompressedEthBlobSidecar to ethpb.BlobSidecar
	newRealBlobSidecar := ethpb.BlobSidecar{
		Index:                    newSidecarCheckUnmarshal.Index,
		Blob:                     realBlobValue,
		KzgCommitment:            newSidecarCheckUnmarshal.KzgCommitment,
		KzgProof:                 newSidecarCheckUnmarshal.KzgProof,
		SignedBlockHeader:        newSidecarCheckUnmarshal.SignedBlockHeader,
		CommitmentInclusionProof: newSidecarCheckUnmarshal.CommitmentInclusionProof,
	}

	// Check that MarshalSSZ works correctly for ethpb.BlobSidecar after converting from CompressedEthBlobSidecar
	newRealBlobSidecarBytes, err := newRealBlobSidecar.MarshalSSZ()
	require.NoError(t, err)

	// Check that UnmarshalSSZ works correctly for ethpb.BlobSidecar after converting from CompressedEthBlobSidecar
	var newRealBlobSidecarCheckUnmarshal ethpb.BlobSidecar
	err = newRealBlobSidecarCheckUnmarshal.UnmarshalSSZ(newRealBlobSidecarBytes)
	require.NoError(t, err)

	// Check that the unmarshaled real sidecar has the expected values after converting from CompressedEthBlobSidecar
	// This test will fale in case the compatibility between CompressedEthBlobSidecar and ethpb.BlobSidecar breaks
	require.Equal(t, expectedIndex, newRealBlobSidecarCheckUnmarshal.Index)
	require.Equal(t, expectedKzgCommitment, hex.EncodeToString(newRealBlobSidecarCheckUnmarshal.KzgCommitment))
	require.Equal(t, expectedKzgProof, hex.EncodeToString(newRealBlobSidecarCheckUnmarshal.KzgProof))
	require.Equal(t, expectedSignedBlockHeaderSlot, uint64(newRealBlobSidecarCheckUnmarshal.SignedBlockHeader.Header.Slot))
	require.Equal(t, expectedSignedBlockHeaderProposerIndex, uint64(newRealBlobSidecarCheckUnmarshal.SignedBlockHeader.Header.ProposerIndex))
	require.Equal(t, expectedSignedBlockHeaderParentRoot, hex.EncodeToString(newRealBlobSidecarCheckUnmarshal.SignedBlockHeader.Header.ParentRoot))
	require.Equal(t, expectedSignedBlockHeaderStateRoot, hex.EncodeToString(newRealBlobSidecarCheckUnmarshal.SignedBlockHeader.Header.StateRoot))
	require.Equal(t, expectedSignedBlockHeaderBodyRoot, hex.EncodeToString(newRealBlobSidecarCheckUnmarshal.SignedBlockHeader.Header.BodyRoot))
	require.Equal(t, expectedSignedBlockHeaderSignature, hex.EncodeToString(newRealBlobSidecarCheckUnmarshal.SignedBlockHeader.Signature))
	require.Equal(t, expectedCommitmentInclusionProofBytes, newRealBlobSidecarCheckUnmarshal.CommitmentInclusionProof)

	// check that in the end of the trip we have inital byte representation of the blob sidecar
	require.Equal(t, data, newRealBlobSidecarBytes)
}
