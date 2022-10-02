package beacon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p"
	p2ptypes "github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v3/config/params"
	types "github.com/prysmaticlabs/prysm/v3/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v3/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v3/network/forks"
	pb "github.com/prysmaticlabs/prysm/v3/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v3/proto/prysm/v1alpha1/metadata"
	"github.com/prysmaticlabs/prysm/v3/runtime/version"
	"github.com/prysmaticlabs/prysm/v3/time/slots"
)

func (n *Node) sendStatusRequest(ctx context.Context, id peer.ID) (*pb.Status, error) {
	status, err := n.status(ctx)
	if err != nil {
		return nil, err
	}

	topic, err := p2p.TopicFromMessage(p2p.StatusMessageName, types.Epoch(n.currentSlot()))
	if err != nil {
		return nil, err
	}
	stream, err := n.p2p.Send(ctx, status, topic, id)
	if err != nil {
		return nil, fmt.Errorf("could not handshake: %v", err)
	}
	defer stream.Close()

	if err := readStatusCode(stream, n.p2p.Encoding()); err != nil {
		return nil, fmt.Errorf("bad handshake status code: %v", err)
	}

	msg := new(pb.Status)
	if err := n.p2p.Encoding().DecodeWithMaxLength(stream, msg); err != nil {
		return nil, fmt.Errorf("could not decode message: %v", err)
	}

	n.p2p.Peers().Scorers().PeerStatusScorer().SetPeerStatus(id, msg, nil)

	// Copy peer status
	n.updateLastStatus(msg)

	return msg, nil
}

func (n *Node) sendPingRequest(ctx context.Context, id peer.ID) error {
	n.log.WithField("peer", id).Debug("sending ping request")

	ctx, cancel := context.WithTimeout(ctx, params.BeaconNetworkConfig().RespTimeout)
	defer cancel()

	metadataSeq := types.SSZUint64(n.p2p.MetadataSeq())
	topic, err := p2p.TopicFromMessage(p2p.PingMessageName, slots.ToEpoch(n.currentSlot()))
	if err != nil {
		return err
	}
	stream, err := n.p2p.Send(ctx, &metadataSeq, topic, id)
	if err != nil {
		return err
	}
	currentTime := time.Now()
	defer stream.Close()

	if err := readStatusCode(stream, n.p2p.Encoding()); err != nil {
		return fmt.Errorf("ping bad status code: %v", err)
	}
	// Records the latency of the ping request for that peer.
	n.p2p.Host().Peerstore().RecordLatency(id, time.Since(currentTime))

	msg := new(types.SSZUint64)
	if err := n.p2p.Encoding().DecodeWithMaxLength(stream, msg); err != nil {
		return err
	}

	valid, err := n.validateSequenceNum(*msg, stream.Conn().RemotePeer())
	if err != nil {
		// Descore peer for giving us a bad sequence number.
		if errors.Is(err, p2ptypes.ErrInvalidSequenceNum) {
			n.p2p.Peers().Scorers().BadResponsesScorer().Increment(stream.Conn().RemotePeer())
		}
		return err
	}

	if valid {
		return nil
	}
	md, err := n.sendMetaDataRequest(ctx, stream.Conn().RemotePeer())
	if err != nil {
		// do not increment bad responses, as its
		// already done in the request method.
		return fmt.Errorf("could not send meta data request: %v", err)
	}

	n.p2p.Peers().SetMetadata(stream.Conn().RemotePeer(), md)
	return nil
}

func (n *Node) sendMetaDataRequest(ctx context.Context, id peer.ID) (metadata.Metadata, error) {
	ctx, cancel := context.WithTimeout(ctx, params.BeaconNetworkConfig().RespTimeout)
	defer cancel()

	topic, err := p2p.TopicFromMessage(p2p.MetadataMessageName, slots.ToEpoch(n.currentSlot()))
	if err != nil {
		return nil, err
	}
	stream, err := n.p2p.Send(ctx, new(interface{}), topic, id)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	if err := readStatusCode(stream, n.p2p.Encoding()); err != nil {
		return nil, fmt.Errorf("invalid status for meta data request: %v", err)
	}

	genesisTime := time.Unix(int64(n.genesisState.GenesisTime()), 0)
	genesisValidatorsRoot := n.genesisState.GenesisValidatorsRoot()

	digest, err := forks.CreateForkDigest(genesisTime, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}

	msg, err := extractMetaDataType(digest[:], genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	// Defensive check to ensure valid objects are being sent.
	topicVersion := ""
	switch msg.Version() {
	case version.Phase0:
		topicVersion = p2p.SchemaVersionV1
	case version.Altair:
		topicVersion = p2p.SchemaVersionV2
	}
	if err := validateVersion(topicVersion, stream); err != nil {
		return nil, err
	}
	if err := n.p2p.Encoding().DecodeWithMaxLength(stream, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func extractMetaDataType(digest, vRoot []byte) (metadata.Metadata, error) {
	for k, mdFunc := range p2ptypes.MetaDataMap {
		rDigest, err := signing.ComputeForkDigest(k[:], vRoot[:])
		if err != nil {
			return nil, err
		}
		if rDigest == bytesutil.ToBytes4(digest) {
			return mdFunc(), nil
		}
	}
	return nil, errors.New("no valid digest matched")
}

// Minimal interface for a stream with a protocol.
type withProtocol interface {
	Protocol() protocol.ID
}

// Validates that the rpc topic matches the provided version.
func validateVersion(version string, stream withProtocol) error {
	_, _, streamVersion, err := p2p.TopicDeconstructor(string(stream.Protocol()))
	if err != nil {
		return err
	}
	if streamVersion != version {
		return fmt.Errorf("stream version of %s doesn't match provided version %s", streamVersion, version)
	}
	return nil
}
