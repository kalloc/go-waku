// Waku Relay module. Thin layer on top of GossipSub.
//
// See https://github.com/vacp2p/specs/blob/master/specs/waku/v2/waku-relay.md
// for spec.
package protocol

import (
	"context"
	"crypto/sha256"

	"github.com/libp2p/go-libp2p-core/host"
	libp2pProtocol "github.com/libp2p/go-libp2p-core/protocol"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
)

const WakuRelayProtocol = libp2pProtocol.ID("/vac/waku/relay/2.0.0-beta2")

type WakuRelay struct {
	p *pubsub.PubSub
}

// Once https://github.com/status-im/nim-waku/issues/420 is fixed, implement a custom messageIdFn
func msgIdFn(pmsg *pubsub_pb.Message) string {
	hash := sha256.Sum256(pmsg.Data)
	return string(hash[:])
}

func NewWakuRelay(ctx context.Context, h host.Host, opts ...pubsub.Option) (*pubsub.PubSub, error) {
	opts = append(opts, pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign))
	opts = append(opts, pubsub.WithNoAuthor())
	opts = append(opts, pubsub.WithMessageIdFn(msgIdFn))

	gossipSub, err := pubsub.NewGossipSub(ctx, h, []libp2pProtocol.ID{WakuRelayProtocol}, opts...)

	if err != nil {
		return nil, err
	}

	w := new(WakuRelay)
	w.p = gossipSub
	return gossipSub, nil
}
