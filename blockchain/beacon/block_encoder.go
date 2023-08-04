package beacon

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v4/beacon-chain/rpc/apimiddleware"
	"github.com/prysmaticlabs/prysm/v4/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v4/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v4/encoding/bytesutil"
	enginev1 "github.com/prysmaticlabs/prysm/v4/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v4/proto/prysm/v1alpha1"
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
	var signedCapella *ethpb.SignedBeaconBlockCapella
	signedCapella, err := block.PbCapellaBlock()
	if err != nil {
		return nil, fmt.Errorf("failed to get generic capella: %v", err)
	}
	rawBlock, err := marshallBeaconBlockCapella(signedCapella)
	if err != nil {
		return nil, fmt.Errorf("failed to marshall blinded capella beacon block: %v", err)
	}
	return rawBlock, nil
}

func uint64ToString[T uint64 | primitives.Slot | primitives.ValidatorIndex | primitives.CommitteeIndex | primitives.Epoch](val T) string {
	return strconv.FormatUint(uint64(val), 10)
}

func marshallBeaconBlockCapella(block *ethpb.SignedBeaconBlockCapella) ([]byte, error) {
	signedBeaconBlockCapellaJSON := &apimiddleware.SignedBeaconBlockCapellaContainerJson{
		Signature: hexutil.Encode(block.Signature),
		Message: &apimiddleware.BeaconBlockCapellaJson{
			ParentRoot:    hexutil.Encode(block.Block.ParentRoot),
			ProposerIndex: uint64ToString(block.Block.ProposerIndex),
			Slot:          uint64ToString(block.Block.Slot),
			StateRoot:     hexutil.Encode(block.Block.StateRoot),
			Body: &apimiddleware.BeaconBlockBodyCapellaJson{
				Attestations:      jsonifyAttestations(block.Block.Body.Attestations),
				AttesterSlashings: jsonifyAttesterSlashings(block.Block.Body.AttesterSlashings),
				Deposits:          jsonifyDeposits(block.Block.Body.Deposits),
				Eth1Data:          jsonifyEth1Data(block.Block.Body.Eth1Data),
				Graffiti:          hexutil.Encode(block.Block.Body.Graffiti),
				ProposerSlashings: jsonifyProposerSlashings(block.Block.Body.ProposerSlashings),
				RandaoReveal:      hexutil.Encode(block.Block.Body.RandaoReveal),
				VoluntaryExits:    JsonifySignedVoluntaryExits(block.Block.Body.VoluntaryExits),
				SyncAggregate: &apimiddleware.SyncAggregateJson{
					SyncCommitteeBits:      hexutil.Encode(block.Block.Body.SyncAggregate.SyncCommitteeBits),
					SyncCommitteeSignature: hexutil.Encode(block.Block.Body.SyncAggregate.SyncCommitteeSignature),
				},
				ExecutionPayload: &apimiddleware.ExecutionPayloadCapellaJson{
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
					TimeStamp:     uint64ToString(block.Block.Body.ExecutionPayload.Timestamp),
					Transactions:  jsonifyTransactions(block.Block.Body.ExecutionPayload.Transactions),
					Withdrawals:   jsonifyWithdrawals(block.Block.Body.ExecutionPayload.Withdrawals),
				},
				BLSToExecutionChanges: jsonifyBlsToExecutionChanges(block.Block.Body.BlsToExecutionChanges),
			},
		},
	}

	return json.Marshal(signedBeaconBlockCapellaJSON)
}

func jsonifyTransactions(transactions [][]byte) []string {
	jsonTransactions := make([]string, len(transactions))
	for index, transaction := range transactions {
		jsonTransaction := hexutil.Encode(transaction)
		jsonTransactions[index] = jsonTransaction
	}
	return jsonTransactions
}

func jsonifyBlsToExecutionChanges(blsToExecutionChanges []*ethpb.SignedBLSToExecutionChange) []*apimiddleware.SignedBLSToExecutionChangeJson {
	jsonBlsToExecutionChanges := make([]*apimiddleware.SignedBLSToExecutionChangeJson, len(blsToExecutionChanges))
	for index, signedBlsToExecutionChange := range blsToExecutionChanges {
		blsToExecutionChangeJSON := &apimiddleware.BLSToExecutionChangeJson{
			ValidatorIndex:     uint64ToString(signedBlsToExecutionChange.Message.ValidatorIndex),
			FromBLSPubkey:      hexutil.Encode(signedBlsToExecutionChange.Message.FromBlsPubkey),
			ToExecutionAddress: hexutil.Encode(signedBlsToExecutionChange.Message.ToExecutionAddress),
		}
		signedJSON := &apimiddleware.SignedBLSToExecutionChangeJson{
			Message:   blsToExecutionChangeJSON,
			Signature: hexutil.Encode(signedBlsToExecutionChange.Signature),
		}
		jsonBlsToExecutionChanges[index] = signedJSON
	}
	return jsonBlsToExecutionChanges
}

func jsonifyEth1Data(eth1Data *ethpb.Eth1Data) *apimiddleware.Eth1DataJson {
	return &apimiddleware.Eth1DataJson{
		BlockHash:    hexutil.Encode(eth1Data.BlockHash),
		DepositCount: uint64ToString(eth1Data.DepositCount),
		DepositRoot:  hexutil.Encode(eth1Data.DepositRoot),
	}
}

func jsonifyAttestations(attestations []*ethpb.Attestation) []*apimiddleware.AttestationJson {
	jsonAttestations := make([]*apimiddleware.AttestationJson, len(attestations))
	for index, attestation := range attestations {
		jsonAttestations[index] = jsonifyAttestation(attestation)
	}
	return jsonAttestations
}

func jsonifyAttesterSlashings(attesterSlashings []*ethpb.AttesterSlashing) []*apimiddleware.AttesterSlashingJson {
	jsonAttesterSlashings := make([]*apimiddleware.AttesterSlashingJson, len(attesterSlashings))
	for index, attesterSlashing := range attesterSlashings {
		jsonAttesterSlashing := &apimiddleware.AttesterSlashingJson{
			Attestation_1: jsonifyIndexedAttestation(attesterSlashing.Attestation_1),
			Attestation_2: jsonifyIndexedAttestation(attesterSlashing.Attestation_2),
		}
		jsonAttesterSlashings[index] = jsonAttesterSlashing
	}
	return jsonAttesterSlashings
}

func jsonifyDeposits(deposits []*ethpb.Deposit) []*apimiddleware.DepositJson {
	jsonDeposits := make([]*apimiddleware.DepositJson, len(deposits))
	for depositIndex, deposit := range deposits {
		proofs := make([]string, len(deposit.Proof))
		for proofIndex, proof := range deposit.Proof {
			proofs[proofIndex] = hexutil.Encode(proof)
		}

		jsonDeposit := &apimiddleware.DepositJson{
			Data: &apimiddleware.Deposit_DataJson{
				Amount:                uint64ToString(deposit.Data.Amount),
				PublicKey:             hexutil.Encode(deposit.Data.PublicKey),
				Signature:             hexutil.Encode(deposit.Data.Signature),
				WithdrawalCredentials: hexutil.Encode(deposit.Data.WithdrawalCredentials),
			},
			Proof: proofs,
		}
		jsonDeposits[depositIndex] = jsonDeposit
	}
	return jsonDeposits
}

func jsonifyProposerSlashings(proposerSlashings []*ethpb.ProposerSlashing) []*apimiddleware.ProposerSlashingJson {
	jsonProposerSlashings := make([]*apimiddleware.ProposerSlashingJson, len(proposerSlashings))
	for index, proposerSlashing := range proposerSlashings {
		jsonProposerSlashing := &apimiddleware.ProposerSlashingJson{
			Header_1: jsonifySignedBeaconBlockHeader(proposerSlashing.Header_1),
			Header_2: jsonifySignedBeaconBlockHeader(proposerSlashing.Header_2),
		}
		jsonProposerSlashings[index] = jsonProposerSlashing
	}
	return jsonProposerSlashings
}

// JsonifySignedVoluntaryExits converts an array of voluntary exit structs to a JSON hex string compatible format.
func JsonifySignedVoluntaryExits(voluntaryExits []*ethpb.SignedVoluntaryExit) []*apimiddleware.SignedVoluntaryExitJson {
	jsonSignedVoluntaryExits := make([]*apimiddleware.SignedVoluntaryExitJson, len(voluntaryExits))
	for index, signedVoluntaryExit := range voluntaryExits {
		jsonSignedVoluntaryExit := &apimiddleware.SignedVoluntaryExitJson{
			Exit: &apimiddleware.VoluntaryExitJson{
				Epoch:          uint64ToString(signedVoluntaryExit.Exit.Epoch),
				ValidatorIndex: uint64ToString(signedVoluntaryExit.Exit.ValidatorIndex),
			},
			Signature: hexutil.Encode(signedVoluntaryExit.Signature),
		}
		jsonSignedVoluntaryExits[index] = jsonSignedVoluntaryExit
	}
	return jsonSignedVoluntaryExits
}

func jsonifySignedBeaconBlockHeader(signedBeaconBlockHeader *ethpb.SignedBeaconBlockHeader) *apimiddleware.SignedBeaconBlockHeaderJson {
	return &apimiddleware.SignedBeaconBlockHeaderJson{
		Header: &apimiddleware.BeaconBlockHeaderJson{
			BodyRoot:      hexutil.Encode(signedBeaconBlockHeader.Header.BodyRoot),
			ParentRoot:    hexutil.Encode(signedBeaconBlockHeader.Header.ParentRoot),
			ProposerIndex: uint64ToString(signedBeaconBlockHeader.Header.ProposerIndex),
			Slot:          uint64ToString(signedBeaconBlockHeader.Header.Slot),
			StateRoot:     hexutil.Encode(signedBeaconBlockHeader.Header.StateRoot),
		},
		Signature: hexutil.Encode(signedBeaconBlockHeader.Signature),
	}
}

func jsonifyIndexedAttestation(indexedAttestation *ethpb.IndexedAttestation) *apimiddleware.IndexedAttestationJson {
	attestingIndices := make([]string, len(indexedAttestation.AttestingIndices))
	for index, attestingIndex := range indexedAttestation.AttestingIndices {
		attestingIndex := uint64ToString(attestingIndex)
		attestingIndices[index] = attestingIndex
	}

	return &apimiddleware.IndexedAttestationJson{
		AttestingIndices: attestingIndices,
		Data:             jsonifyAttestationData(indexedAttestation.Data),
		Signature:        hexutil.Encode(indexedAttestation.Signature),
	}
}

func jsonifyAttestationData(attestationData *ethpb.AttestationData) *apimiddleware.AttestationDataJson {
	return &apimiddleware.AttestationDataJson{
		BeaconBlockRoot: hexutil.Encode(attestationData.BeaconBlockRoot),
		CommitteeIndex:  uint64ToString(attestationData.CommitteeIndex),
		Slot:            uint64ToString(attestationData.Slot),
		Source: &apimiddleware.CheckpointJson{
			Epoch: uint64ToString(attestationData.Source.Epoch),
			Root:  hexutil.Encode(attestationData.Source.Root),
		},
		Target: &apimiddleware.CheckpointJson{
			Epoch: uint64ToString(attestationData.Target.Epoch),
			Root:  hexutil.Encode(attestationData.Target.Root),
		},
	}
}

func jsonifyAttestation(attestation *ethpb.Attestation) *apimiddleware.AttestationJson {
	return &apimiddleware.AttestationJson{
		AggregationBits: hexutil.Encode(attestation.AggregationBits),
		Data:            jsonifyAttestationData(attestation.Data),
		Signature:       hexutil.Encode(attestation.Signature),
	}
}

func jsonifyWithdrawals(withdrawals []*enginev1.Withdrawal) []*apimiddleware.WithdrawalJson {
	jsonWithdrawals := make([]*apimiddleware.WithdrawalJson, len(withdrawals))
	for index, withdrawal := range withdrawals {
		jsonWithdrawals[index] = &apimiddleware.WithdrawalJson{
			WithdrawalIndex:  strconv.FormatUint(withdrawal.Index, 10),
			ValidatorIndex:   strconv.FormatUint(uint64(withdrawal.ValidatorIndex), 10),
			ExecutionAddress: hexutil.Encode(withdrawal.Address),
			Amount:           strconv.FormatUint(withdrawal.Amount, 10),
		}
	}
	return jsonWithdrawals
}
