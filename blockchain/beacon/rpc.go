package beacon

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	libp2pcore "github.com/libp2p/go-libp2p-core"
	p2pNetwork "github.com/libp2p/go-libp2p-core/network"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p"
	p2ptypes "github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p/types"
	types "github.com/prysmaticlabs/prysm/v3/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v3/consensus-types/wrapper"
	pb "github.com/prysmaticlabs/prysm/v3/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v3/runtime/version"
)

func (n *Node) subscribeRPC(topic string, handler p2pNetwork.StreamHandler) {
	n.p2p.SetStreamHandler(topic+n.p2p.Encoding().ProtocolSuffix(), handler)
}

func (n *Node) statusRPCHandler(stream libp2pcore.Stream) {
	SetRPCStreamDeadlines(stream)

	log := n.log.WithField("peer", stream.Conn().RemotePeer())
	log.Debug("receiving status RPC request")

	msg := new(pb.Status)
	if err := n.p2p.Encoding().DecodeWithMaxLength(stream, msg); err != nil {
		log.Errorf("could not decode status RPC request: %v", err)
		return
	}

	n.p2p.Peers().SetChainState(stream.Conn().RemotePeer(), msg)

	n.updateLastStatus(msg)

	status, err := n.status(n.ctx)
	if err != nil {
		log.Errorf("could not get status: %v", err)
		return
	}

	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		log.Errorf("could not write to stream: %v", err)
		return
	}

	if _, err := n.p2p.Encoding().EncodeWithMaxLength(stream, status); err != nil {
		log.Errorf("could not encode status RPC message: %v", err)
		return
	}

	if err := stream.Close(); err != nil {
		log.Errorf("could not close the stream: %v", err)
		return
	}
}

func (n *Node) goodbyeRPCHandler(stream libp2pcore.Stream) {
	SetRPCStreamDeadlines(stream)

	log := n.log.WithField("peer", stream.Conn().RemotePeer())

	msg := new(types.SSZUint64)
	if err := n.p2p.Encoding().DecodeWithMaxLength(stream, msg); err != nil {
		log.Errorf("could not decode goodbye RPC request: %v", err)
		return
	}

	code := p2ptypes.RPCGoodbyeCode(*msg)

	log.WithField("reason", goodbyeMessage(code)).Debug("peer has sent a goodbye message")

	n.p2p.Disconnect(stream.Conn().RemotePeer())
}

func goodbyeMessage(num p2ptypes.RPCGoodbyeCode) string {
	reason, ok := p2ptypes.GoodbyeCodeMessages[num]
	if ok {
		return reason
	}
	return fmt.Sprintf("unknown goodbye value of %d received", num)
}

func (n *Node) metadataRPCHandler(stream libp2pcore.Stream) {
	SetRPCStreamDeadlines(stream)

	log := n.log.WithField("peer", stream.Conn().RemotePeer())
	log.Debug("receiving metadata RPC request")

	_, _, streamVersion, err := p2p.TopicDeconstructor(string(stream.Protocol()))
	if err != nil {
		buf := bytes.NewBuffer([]byte{responseCodeServerError})
		errMsg := p2ptypes.ErrorMessage(p2ptypes.ErrGeneric.Error())
		if _, err := n.p2p.Encoding().EncodeWithMaxLength(buf, &errMsg); err != nil {
			log.Errorf("could not encode error msg: %v", err)
			return
		}
		return
	}
	currMd := n.p2p.Metadata()
	switch streamVersion {
	case p2p.SchemaVersionV1:
		// We have a v1 metadata object saved locally, so we
		// convert it back to a v0 metadata object.
		if currMd.Version() != version.Phase0 {
			currMd = wrapper.WrappedMetadataV0(
				&pb.MetaDataV0{
					Attnets:   currMd.AttnetsBitfield(),
					SeqNumber: currMd.SequenceNumber(),
				})
		}
	case p2p.SchemaVersionV2:
		// We have a v0 metadata object saved locally, so we
		// convert it to a v1 metadata object.
		if currMd.Version() != version.Altair {
			currMd = wrapper.WrappedMetadataV1(
				&pb.MetaDataV1{
					Attnets:   currMd.AttnetsBitfield(),
					SeqNumber: currMd.SequenceNumber(),
					Syncnets:  bitfield.Bitvector4{byte(0x00)},
				})
		}
	}

	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		log.Errorf("could not respond on RPC metadata: %v", err)
		return
	}
	_, err = n.p2p.Encoding().EncodeWithMaxLength(stream, currMd)
	if err != nil {
		log.Errorf("could not encode on RPC metadata: %v", err)
		return
	}
	if err := stream.Close(); err != nil {
		log.Errorf("could not close the stream: %v", err)
		return
	}
}

func (n *Node) pingRPCHandler(stream libp2pcore.Stream) {
	SetRPCStreamDeadlines(stream)

	log := n.log.WithField("peer", stream.Conn().RemotePeer())
	log.Debug("receiving ping RPC request")

	msg := new(types.SSZUint64)
	if err := n.p2p.Encoding().DecodeWithMaxLength(stream, msg); err != nil {
		log.Errorf("could not decode ping request: %v", err)
		return
	}

	valid, err := n.validateSequenceNum(*msg, stream.Conn().RemotePeer())
	if err != nil {
		// Descore peer for giving us a bad sequence number.
		if errors.Is(err, p2ptypes.ErrInvalidSequenceNum) {
			buf := bytes.NewBuffer([]byte{responseCodeInvalidRequest})
			errMsg := p2ptypes.ErrorMessage(p2ptypes.ErrInvalidSequenceNum.Error())
			if _, err := n.p2p.Encoding().EncodeWithMaxLength(buf, &errMsg); err != nil {
				log.Errorf("could not encode error msg: %v", err)
				return
			}
		}
		return
	}

	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		log.Errorf("could not respond on RPC ping: %v", err)
		return
	}
	sq := types.SSZUint64(n.p2p.MetadataSeq())
	if _, err := n.p2p.Encoding().EncodeWithMaxLength(stream, &sq); err != nil {
		log.Errorf("could not encode on RPC ping: %v", err)
		return
	}

	if err := stream.Close(); err != nil {
		log.Errorf("could not close the stream: %v", err)
		return
	}

	if valid {
		return
	}

	go func() {
		md, err := n.sendMetaDataRequest(n.ctx, stream.Conn().RemotePeer())
		if err != nil {
			if !strings.Contains(err.Error(), p2ptypes.ErrIODeadline.Error()) {
				log.Debugf("could not send metadata request: %v", err)
			}
			return
		}

		// update metadata if there is no error
		n.p2p.Peers().SetMetadata(stream.Conn().RemotePeer(), md)
	}()
}
