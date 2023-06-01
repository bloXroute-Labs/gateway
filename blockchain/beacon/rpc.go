package beacon

import (
	"bytes"
	"fmt"

	"github.com/prysmaticlabs/go-bitfield"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	libp2pNetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/prysm/v4/beacon-chain/p2p"
	prysmTypes "github.com/prysmaticlabs/prysm/v4/beacon-chain/p2p/types"
	types "github.com/prysmaticlabs/prysm/v4/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v4/consensus-types/wrapper"
	ethpb "github.com/prysmaticlabs/prysm/v4/proto/prysm/v1alpha1"
)

func (n *Node) subscribeRPC(topic string, handler libp2pNetwork.StreamHandler) {
	n.host.SetStreamHandler(protocol.ID(topic+n.encoding.ProtocolSuffix()), handler)
}

func (n *Node) statusRPCHandler(stream libp2pNetwork.Stream) {
	defer n.closeStream(stream)

	SetRPCStreamDeadlines(stream)

	msg := new(ethpb.Status)
	if err := n.encoding.DecodeWithMaxLength(stream, msg); err != nil {
		n.log.Errorf("could not decode status RPC request from peer %v: %v", stream.Conn().RemotePeer(), err)
		return
	}

	peer := n.peers.get(stream.Conn().RemotePeer())
	if peer == nil {
		n.log.Errorf("could not get peer %v for status", stream.Conn().RemotePeer())
		return
	}
	peer.setStatus(msg)

	status, err := n.getStatus()
	if err != nil {
		n.log.Errorf("could not get status for peer %v: %v", stream.Conn().RemotePeer(), err)
		return
	}

	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		n.log.Errorf("could not write to peer %v stream: %v", stream.Conn().RemotePeer(), err)
		return
	}

	if _, err := n.encoding.EncodeWithMaxLength(stream, status); err != nil {
		n.log.Errorf("could not encode status RPC message for peer %v: %v", stream.Conn().RemotePeer(), err)
		return
	}

	n.updateStatus(status)
}

func (n *Node) goodbyeRPCHandler(stream libp2pNetwork.Stream) {
	defer n.closeStream(stream)

	SetRPCStreamDeadlines(stream)

	reason := "unknown"
	msg := new(prysmTypes.RPCGoodbyeCode)
	if err := n.encoding.DecodeWithMaxLength(stream, msg); err != nil {
		n.log.Errorf("could not decode goodbye request from peer %v: %v", stream.Conn().RemotePeer(), err)
	} else {
		var ok bool
		reason, ok = prysmTypes.GoodbyeCodeMessages[*msg]
		if !ok {
			reason = fmt.Sprintf("unknown(code %d)", msg)
		}
	}

	n.log.Tracef("peer %v sent goodbye reason: %v", stream.Conn().RemotePeer(), reason)

	peer := n.peers.get(stream.Conn().RemotePeer())
	if err := n.host.Network().ClosePeer(stream.Conn().RemotePeer()); err != nil {
		n.log.Errorf("could not close peer %v: %v", peer, err)
	}
}

func (n *Node) pingRPCHandler(stream libp2pNetwork.Stream) {
	defer n.closeStream(stream)

	SetRPCStreamDeadlines(stream)

	msg := new(types.SSZUint64)
	if err := n.encoding.DecodeWithMaxLength(stream, msg); err != nil {
		log.Errorf("could not decode ping request from peer %v: %v", stream.Conn().RemotePeer(), err)
		return
	}

	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		log.Errorf("could not respond on RPC ping to peer %v: %v", stream.Conn().RemotePeer(), err)
		return
	}

	sq := types.SSZUint64(0)
	if _, err := n.encoding.EncodeWithMaxLength(stream, &sq); err != nil {
		log.Errorf("could not encode on RPC ping from peer %v: %v", stream.Conn().RemotePeer(), err)
		return
	}
}

func (n *Node) metadataRPCHandler(stream libp2pNetwork.Stream) {
	defer n.closeStream(stream)

	SetRPCStreamDeadlines(stream)

	_, _, streamVersion, err := p2p.TopicDeconstructor(string(stream.Protocol()))
	if err != nil {
		buf := bytes.NewBuffer([]byte{responseCodeServerError})
		errMsg := prysmTypes.ErrorMessage(prysmTypes.ErrGeneric.Error())
		if _, err := n.encoding.EncodeWithMaxLength(buf, &errMsg); err != nil {
			log.Errorf("could not encode error msg: %v", err)
			return
		}
		return
	}

	var currMd ssz.Marshaler
	switch streamVersion {
	case p2p.SchemaVersionV1:
		currMd = wrapper.WrappedMetadataV0(
			&ethpb.MetaDataV0{
				Attnets:   bitfield.NewBitvector64(),
				SeqNumber: 0,
			})
	case p2p.SchemaVersionV2:
		currMd = wrapper.WrappedMetadataV1(
			&ethpb.MetaDataV1{
				Attnets:   bitfield.NewBitvector64(),
				SeqNumber: 0,
				Syncnets:  bitfield.Bitvector4{byte(0xF)},
			})
	}

	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		log.Errorf("could not respond on RPC metadata: %v", err)
		return
	}
	_, err = n.encoding.EncodeWithMaxLength(stream, currMd)
	if err != nil {
		log.Errorf("could not encode on RPC metadata: %v", err)
		return
	}
}
