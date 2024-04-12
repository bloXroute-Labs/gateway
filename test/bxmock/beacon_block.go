package bxmock

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/prysmaticlabs/go-bitfield"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/stretchr/testify/assert"
)

// NewDenebBeaconBlock creates beacon block using execution layer block
func NewDenebBeaconBlock(t *testing.T, slot int, prevBlock interfaces.ReadOnlySignedBeaconBlock, ethBlock *ethtypes.Block) interfaces.ReadOnlySignedBeaconBlock {
	ezDecode := func(t *testing.T, str string) []byte {
		b, err := hex.DecodeString(strings.TrimPrefix(str, "0x"))
		assert.NoError(t, err)

		return b
	}
	makeDepositProof := func() [][]byte {
		proof := make([][]byte, int(params.BeaconConfig().DepositContractTreeDepth)+1)
		for i := range proof {
			proof[i] = make([]byte, 32)
		}
		return proof
	}

	syncBits := bitfield.NewBitvector512()
	for i := range syncBits {
		syncBits[i] = 0xff
	}

	txs := make([][]byte, 0, len(ethBlock.Transactions()))
	for _, tx := range ethBlock.Transactions() {
		txBytes, err := tx.MarshalBinary()
		assert.NoError(t, err)

		txs = append(txs, txBytes)
	}

	block := &ethpb.SignedBeaconBlockDeneb{
		Block: &ethpb.BeaconBlockDeneb{
			Slot:          1,
			ProposerIndex: 1,
			ParentRoot:    ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
			StateRoot:     ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
			Body: &ethpb.BeaconBlockBodyDeneb{
				RandaoReveal: ezDecode(t, "0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505"),
				Eth1Data: &ethpb.Eth1Data{
					DepositRoot:  ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
					DepositCount: 1,
					BlockHash:    ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
				},
				Graffiti: ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
				ProposerSlashings: []*ethpb.ProposerSlashing{
					{
						Header_1: &ethpb.SignedBeaconBlockHeader{
							Header: &ethpb.BeaconBlockHeader{
								Slot:          1,
								ProposerIndex: 1,
								ParentRoot:    ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
								StateRoot:     ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
								BodyRoot:      ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
							},
							Signature: ezDecode(t, "0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505"),
						},
						Header_2: &ethpb.SignedBeaconBlockHeader{
							Header: &ethpb.BeaconBlockHeader{
								Slot:          1,
								ProposerIndex: 1,
								ParentRoot:    ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
								StateRoot:     ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
								BodyRoot:      ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
							},
							Signature: ezDecode(t, "0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505"),
						},
					},
				},
				AttesterSlashings: []*ethpb.AttesterSlashing{
					{
						Attestation_1: &ethpb.IndexedAttestation{
							AttestingIndices: []uint64{1},
							Data: &ethpb.AttestationData{
								Slot:            1,
								CommitteeIndex:  1,
								BeaconBlockRoot: ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
								Source: &ethpb.Checkpoint{
									Epoch: 1,
									Root:  ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
								},
								Target: &ethpb.Checkpoint{
									Epoch: 1,
									Root:  ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
								},
							},
							Signature: ezDecode(t, "0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505"),
						},
						Attestation_2: &ethpb.IndexedAttestation{
							AttestingIndices: []uint64{1},
							Data: &ethpb.AttestationData{
								Slot:            1,
								CommitteeIndex:  1,
								BeaconBlockRoot: ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
								Source: &ethpb.Checkpoint{
									Epoch: 1,
									Root:  ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
								},
								Target: &ethpb.Checkpoint{
									Epoch: 1,
									Root:  ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
								},
							},
							Signature: ezDecode(t, "0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505"),
						},
					},
				},
				Attestations: []*ethpb.Attestation{
					{
						AggregationBits: bitfield.Bitlist{0x01},
						Data: &ethpb.AttestationData{
							Slot:            1,
							CommitteeIndex:  1,
							BeaconBlockRoot: ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
							Source: &ethpb.Checkpoint{
								Epoch: 1,
								Root:  ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
							},
							Target: &ethpb.Checkpoint{
								Epoch: 1,
								Root:  ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
							},
						},
						Signature: ezDecode(t, "0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505"),
					},
				},
				Deposits: []*ethpb.Deposit{
					{
						Proof: makeDepositProof(),
						Data: &ethpb.Deposit_Data{
							PublicKey:             ezDecode(t, "0x93247f2209abcacf57b75a51dafae777f9dd38bc7053d1af526f220a7489a6d3a2753e5f3e8b1cfe39b56f43611df74a"),
							WithdrawalCredentials: ezDecode(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"),
							Amount:                1,
							Signature:             ezDecode(t, "0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505"),
						},
					},
				},
				VoluntaryExits: []*ethpb.SignedVoluntaryExit{
					{
						Exit: &ethpb.VoluntaryExit{
							Epoch:          1,
							ValidatorIndex: 1,
						},
						Signature: ezDecode(t, "0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505"),
					},
				},
				SyncAggregate: &ethpb.SyncAggregate{
					SyncCommitteeSignature: make([]byte, 96),
					SyncCommitteeBits:      syncBits,
				},
				ExecutionPayload: &enginev1.ExecutionPayloadDeneb{
					ParentHash:    ethBlock.Header().ParentHash[:],
					FeeRecipient:  ethBlock.Header().Coinbase[:],
					StateRoot:     ethBlock.Header().Root[:],
					ReceiptsRoot:  ethBlock.Header().ReceiptHash[:],
					LogsBloom:     ethBlock.Header().Bloom[:],
					PrevRandao:    ethBlock.Header().MixDigest[:],
					BlockNumber:   ethBlock.Header().Number.Uint64(),
					GasLimit:      ethBlock.Header().GasLimit,
					GasUsed:       ethBlock.Header().GasUsed,
					Timestamp:     ethBlock.Header().Time,
					ExtraData:     ethBlock.Header().Extra,
					BaseFeePerGas: ethBlock.Header().BaseFee.Bytes(),
					BlockHash:     ethBlock.Hash().Bytes(),
					BlobGasUsed:   *ethBlock.Header().BlobGasUsed,
					ExcessBlobGas: *ethBlock.Header().ExcessBlobGas,
					Transactions:  txs,
					Withdrawals: []*enginev1.Withdrawal{
						{
							Index:          1,
							ValidatorIndex: 1,
							Address:        make([]byte, 20),
							Amount:         1,
						},
					},
				},
			},
		},
		Signature: make([]byte, 96),
	}

	blk, err := blocks.NewSignedBeaconBlock(block)
	assert.NoError(t, err)

	return blk
}
