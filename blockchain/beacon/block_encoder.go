package beacon

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v4/beacon-chain/rpc/eth/shared"
	"github.com/prysmaticlabs/prysm/v4/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v4/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v4/encoding/bytesutil"
	enginev1 "github.com/prysmaticlabs/prysm/v4/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v4/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v4/runtime/version"
)

type consensusBlockEncoder interface {
	contentType() string
	encodeBlock(block interfaces.ReadOnlySignedBeaconBlock) ([]byte, error)
}

func newSSZConsensusBlockEncoder() consensusBlockEncoder {
	return &sszConsensusBlockEncoder{}
}

func newJSONConsensusBlockEncoder() consensusBlockEncoder {
	return &jsonConsensusBlockEncoder{}
}

type sszConsensusBlockEncoder struct{}

func (c *sszConsensusBlockEncoder) contentType() string {
	return "application/octet-stream"
}

func (c *sszConsensusBlockEncoder) encodeBlock(block interfaces.ReadOnlySignedBeaconBlock) ([]byte, error) {
	rawBlock, err := block.MarshalSSZ()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal block %v", err)
	}
	return rawBlock, nil
}

type jsonConsensusBlockEncoder struct{}

func (c *jsonConsensusBlockEncoder) contentType() string {
	return "application/json"
}

func (c *jsonConsensusBlockEncoder) encodeBlock(block interfaces.ReadOnlySignedBeaconBlock) ([]byte, error) {
	var rawBlock []byte

	switch block.Version() {
	case version.Capella:
		var signedCapella *ethpb.SignedBeaconBlockCapella
		signedCapella, err := block.PbCapellaBlock()
		if err != nil {
			return nil, fmt.Errorf("failed to get generic capella: %v", err)
		}
		rawBlock, err = marshallBeaconBlockCapella(signedCapella)
		if err != nil {
			return nil, fmt.Errorf("failed to marshall blinded capella beacon block: %v", err)
		}
	case version.Deneb:
		denebBlock, err := block.PbDenebBlock()
		if err != nil {
			return nil, fmt.Errorf("failed to get generic deneb: %v", err)
		}

		signedDenebBlock := &ethpb.SignedBeaconBlockContentsDeneb{
			Block:     denebBlock,
			KzgProofs: make([][]byte, 0),
			Blobs:     make([][]byte, 0),
		}

		signedBlockFromConsensus, err := shared.SignedBeaconBlockContentsDenebFromConsensus(signedDenebBlock)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert deneb beacon block contents")
		}

		rawBlock, err = json.Marshal(signedBlockFromConsensus)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal deneb beacon block contents")
		}
	default:
		return nil, fmt.Errorf("unsupported block version: %v", block.Version())
	}

	return rawBlock, nil
}

func uint64ToString[T uint64 | primitives.Slot | primitives.ValidatorIndex | primitives.CommitteeIndex | primitives.Epoch](val T) string {
	return strconv.FormatUint(uint64(val), 10)
}

func marshallBeaconBlockCapella(block *ethpb.SignedBeaconBlockCapella) ([]byte, error) {
	signedBeaconBlockCapella := &shared.SignedBeaconBlockCapella{

		Signature: hexutil.Encode(block.Signature),
		Message: &shared.BeaconBlockCapella{
			ParentRoot:    hexutil.Encode(block.Block.ParentRoot),
			ProposerIndex: uint64ToString(block.Block.ProposerIndex),
			Slot:          uint64ToString(block.Block.Slot),
			StateRoot:     hexutil.Encode(block.Block.StateRoot),
			Body: &shared.BeaconBlockBodyCapella{
				Attestations:      jsonifyAttestations(block.Block.Body.Attestations),
				AttesterSlashings: jsonifyAttesterSlashings(block.Block.Body.AttesterSlashings),
				Deposits:          jsonifyDeposits(block.Block.Body.Deposits),
				Eth1Data:          jsonifyEth1Data(block.Block.Body.Eth1Data),
				Graffiti:          hexutil.Encode(block.Block.Body.Graffiti),
				ProposerSlashings: jsonifyProposerSlashings(block.Block.Body.ProposerSlashings),
				RandaoReveal:      hexutil.Encode(block.Block.Body.RandaoReveal),
				VoluntaryExits:    ifySignedVoluntaryExits(block.Block.Body.VoluntaryExits),
				SyncAggregate: &shared.SyncAggregate{
					SyncCommitteeBits:      hexutil.Encode(block.Block.Body.SyncAggregate.SyncCommitteeBits),
					SyncCommitteeSignature: hexutil.Encode(block.Block.Body.SyncAggregate.SyncCommitteeSignature),
				},
				ExecutionPayload: &shared.ExecutionPayloadCapella{
					BaseFeePerGas: bytesutil.LittleEndianBytesToBigInt(block.Block.Body.ExecutionPayload.BaseFeePerGas).String(),
					BlockHash:     hexutil.Encode(block.Block.Body.ExecutionPayload.BlockHash),
					BlockNumber:   uint64ToString(block.Block.Body.ExecutionPayload.BlockNumber),
					ExtraData:     hexutil.Encode(block.Block.Body.ExecutionPayload.ExtraData),
					FeeRecipient:  hexutil.Encode(block.Block.Body.ExecutionPayload.FeeRecipient),
					GasLimit:      uint64ToString(block.Block.Body.ExecutionPayload.GasLimit),
					GasUsed:       uint64ToString(block.Block.Body.ExecutionPayload.GasUsed),
					LogsBloom:     hexutil.Encode(block.Block.Body.ExecutionPayload.LogsBloom),
					ParentHash:    hexutil.Encode(block.Block.Body.ExecutionPayload.ParentHash),
					PrevRandao:    hexutil.Encode(block.Block.Body.ExecutionPayload.PrevRandao),
					ReceiptsRoot:  hexutil.Encode(block.Block.Body.ExecutionPayload.ReceiptsRoot),
					StateRoot:     hexutil.Encode(block.Block.Body.ExecutionPayload.StateRoot),
					Timestamp:     uint64ToString(block.Block.Body.ExecutionPayload.Timestamp),
					Transactions:  jsonifyTransactions(block.Block.Body.ExecutionPayload.Transactions),
					Withdrawals:   jsonifyWithdrawals(block.Block.Body.ExecutionPayload.Withdrawals),
				},
				BLSToExecutionChanges: jsonifyBlsToExecutionChanges(block.Block.Body.BlsToExecutionChanges),
			},
		},
	}

	return json.Marshal(signedBeaconBlockCapella)
}

func jsonifyTransactions(transactions [][]byte) []string {
	jsonTransactions := make([]string, len(transactions))
	for index, transaction := range transactions {
		jsonTransaction := hexutil.Encode(transaction)
		jsonTransactions[index] = jsonTransaction
	}
	return jsonTransactions
}

func jsonifyBlsToExecutionChanges(blsToExecutionChanges []*ethpb.SignedBLSToExecutionChange) []*shared.SignedBLSToExecutionChange {
	jsonBlsToExecutionChanges := make([]*shared.SignedBLSToExecutionChange, len(blsToExecutionChanges))
	for index, signedBlsToExecutionChange := range blsToExecutionChanges {
		blsToExecutionChangeJSON := &shared.BLSToExecutionChange{
			ValidatorIndex:     uint64ToString(signedBlsToExecutionChange.Message.ValidatorIndex),
			FromBLSPubkey:      hexutil.Encode(signedBlsToExecutionChange.Message.FromBlsPubkey),
			ToExecutionAddress: hexutil.Encode(signedBlsToExecutionChange.Message.ToExecutionAddress),
		}
		signedJSON := &shared.SignedBLSToExecutionChange{
			Message:   blsToExecutionChangeJSON,
			Signature: hexutil.Encode(signedBlsToExecutionChange.Signature),
		}
		jsonBlsToExecutionChanges[index] = signedJSON
	}
	return jsonBlsToExecutionChanges
}

func jsonifyEth1Data(eth1Data *ethpb.Eth1Data) *shared.Eth1Data {
	return &shared.Eth1Data{
		BlockHash:    hexutil.Encode(eth1Data.BlockHash),
		DepositCount: uint64ToString(eth1Data.DepositCount),
		DepositRoot:  hexutil.Encode(eth1Data.DepositRoot),
	}
}

func jsonifyAttestations(attestations []*ethpb.Attestation) []*shared.Attestation {
	jsonAttestations := make([]*shared.Attestation, len(attestations))
	for index, attestation := range attestations {
		jsonAttestations[index] = jsonifyAttestation(attestation)
	}
	return jsonAttestations
}

func jsonifyAttesterSlashings(attesterSlashings []*ethpb.AttesterSlashing) []*shared.AttesterSlashing {
	jsonAttesterSlashings := make([]*shared.AttesterSlashing, len(attesterSlashings))
	for index, attesterSlashing := range attesterSlashings {
		jsonAttesterSlashing := &shared.AttesterSlashing{
			Attestation1: jsonifyIndexedAttestation(attesterSlashing.Attestation_1),
			Attestation2: jsonifyIndexedAttestation(attesterSlashing.Attestation_2),
		}
		jsonAttesterSlashings[index] = jsonAttesterSlashing
	}
	return jsonAttesterSlashings
}

func jsonifyDeposits(deposits []*ethpb.Deposit) []*shared.Deposit {
	jsonDeposits := make([]*shared.Deposit, len(deposits))
	for depositIndex, deposit := range deposits {
		proofs := make([]string, len(deposit.Proof))
		for proofIndex, proof := range deposit.Proof {
			proofs[proofIndex] = hexutil.Encode(proof)
		}

		jsonDeposit := &shared.Deposit{
			Data: &shared.DepositData{
				Amount:                uint64ToString(deposit.Data.Amount),
				Pubkey:                hexutil.Encode(deposit.Data.PublicKey),
				Signature:             hexutil.Encode(deposit.Data.Signature),
				WithdrawalCredentials: hexutil.Encode(deposit.Data.WithdrawalCredentials),
			},
			Proof: proofs,
		}
		jsonDeposits[depositIndex] = jsonDeposit
	}
	return jsonDeposits
}

func jsonifyProposerSlashings(proposerSlashings []*ethpb.ProposerSlashing) []*shared.ProposerSlashing {
	jsonProposerSlashings := make([]*shared.ProposerSlashing, len(proposerSlashings))
	for index, proposerSlashing := range proposerSlashings {
		jsonProposerSlashing := &shared.ProposerSlashing{
			SignedHeader1: jsonifySignedBeaconBlockHeader(proposerSlashing.Header_1),
			SignedHeader2: jsonifySignedBeaconBlockHeader(proposerSlashing.Header_2),
		}
		jsonProposerSlashings[index] = jsonProposerSlashing
	}
	return jsonProposerSlashings
}

// ifySignedVoluntaryExits converts an array of voluntary exit structs to a JSON hex string compatible format.
func ifySignedVoluntaryExits(voluntaryExits []*ethpb.SignedVoluntaryExit) []*shared.SignedVoluntaryExit {
	jsonSignedVoluntaryExits := make([]*shared.SignedVoluntaryExit, len(voluntaryExits))
	for index, signedVoluntaryExit := range voluntaryExits {
		jsonSignedVoluntaryExit := &shared.SignedVoluntaryExit{
			Message: &shared.VoluntaryExit{
				Epoch:          uint64ToString(signedVoluntaryExit.Exit.Epoch),
				ValidatorIndex: uint64ToString(signedVoluntaryExit.Exit.ValidatorIndex),
			},
			Signature: hexutil.Encode(signedVoluntaryExit.Signature),
		}
		jsonSignedVoluntaryExits[index] = jsonSignedVoluntaryExit
	}
	return jsonSignedVoluntaryExits
}

func jsonifySignedBeaconBlockHeader(signedBeaconBlockHeader *ethpb.SignedBeaconBlockHeader) *shared.SignedBeaconBlockHeader {
	return &shared.SignedBeaconBlockHeader{
		Message: &shared.BeaconBlockHeader{
			BodyRoot:      hexutil.Encode(signedBeaconBlockHeader.Header.BodyRoot),
			ParentRoot:    hexutil.Encode(signedBeaconBlockHeader.Header.ParentRoot),
			ProposerIndex: uint64ToString(signedBeaconBlockHeader.Header.ProposerIndex),
			Slot:          uint64ToString(signedBeaconBlockHeader.Header.Slot),
			StateRoot:     hexutil.Encode(signedBeaconBlockHeader.Header.StateRoot),
		},
		Signature: hexutil.Encode(signedBeaconBlockHeader.Signature),
	}
}

func jsonifyIndexedAttestation(indexedAttestation *ethpb.IndexedAttestation) *shared.IndexedAttestation {
	attestingIndices := make([]string, len(indexedAttestation.AttestingIndices))
	for index, attestingIndex := range indexedAttestation.AttestingIndices {
		attestingIndex := uint64ToString(attestingIndex)
		attestingIndices[index] = attestingIndex
	}

	return &shared.IndexedAttestation{
		AttestingIndices: attestingIndices,
		Data:             jsonifyAttestationData(indexedAttestation.Data),
		Signature:        hexutil.Encode(indexedAttestation.Signature),
	}
}

func jsonifyAttestationData(attestationData *ethpb.AttestationData) *shared.AttestationData {
	return &shared.AttestationData{
		BeaconBlockRoot: hexutil.Encode(attestationData.BeaconBlockRoot),
		CommitteeIndex:  uint64ToString(attestationData.CommitteeIndex),
		Slot:            uint64ToString(attestationData.Slot),
		Source: &shared.Checkpoint{
			Epoch: uint64ToString(attestationData.Source.Epoch),
			Root:  hexutil.Encode(attestationData.Source.Root),
		},
		Target: &shared.Checkpoint{
			Epoch: uint64ToString(attestationData.Target.Epoch),
			Root:  hexutil.Encode(attestationData.Target.Root),
		},
	}
}

func jsonifyAttestation(attestation *ethpb.Attestation) *shared.Attestation {
	return &shared.Attestation{
		AggregationBits: hexutil.Encode(attestation.AggregationBits),
		Data:            jsonifyAttestationData(attestation.Data),
		Signature:       hexutil.Encode(attestation.Signature),
	}
}

func jsonifyWithdrawals(withdrawals []*enginev1.Withdrawal) []*shared.Withdrawal {
	jsonWithdrawals := make([]*shared.Withdrawal, len(withdrawals))
	for index, withdrawal := range withdrawals {
		jsonWithdrawals[index] = &shared.Withdrawal{
			WithdrawalIndex:  strconv.FormatUint(withdrawal.Index, 10),
			ValidatorIndex:   strconv.FormatUint(uint64(withdrawal.ValidatorIndex), 10),
			ExecutionAddress: hexutil.Encode(withdrawal.Address),
			Amount:           strconv.FormatUint(withdrawal.Amount, 10),
		}
	}
	return jsonWithdrawals
}
