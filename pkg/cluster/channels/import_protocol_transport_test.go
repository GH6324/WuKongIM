package channels_test

import (
	"bytes"
	"context"
	"testing"

	ch "github.com/WuKongIM/WuKongIM/pkg/channel"
	channeltransport "github.com/WuKongIM/WuKongIM/pkg/channel/transport"
	"github.com/WuKongIM/WuKongIM/pkg/cluster/channels"
	clusternet "github.com/WuKongIM/WuKongIM/pkg/cluster/net"
	"github.com/WuKongIM/WuKongIM/pkg/transport"
	"github.com/stretchr/testify/require"
)

func TestForwardingRefusesToDropHistoricalProtocolOnAnOldPeer(t *testing.T) {
	for _, msg := range []ch.Message{{Protocol: ch.ProtocolFields{Topic: "retained-topic"}}, {SyncOnce: true}} {
		network := clusternet.NewLocalNetwork()
		calls := 0
		handler := clusternet.HandlerFunc(func(context.Context, []byte) ([]byte, error) {
			calls++
			return nil, transport.RemoteError{Code: "remote_error", Message: "channels: invalid frame"}
		})
		network.Register(2, clusternet.RPCChannelAppend, handler)
		network.Register(2, clusternet.RPCChannelAppendBatch, handler)
		client := channels.NewTransportClient(network)
		_, err := client.ForwardAppend(context.Background(), 2, ch.AppendRequest{ChannelID: ch.ChannelID{ID: "group", Type: 2}, Message: msg})
		require.ErrorContains(t, err, "requires channel codec v8")
		require.Equal(t, 1, calls, "an old-format retry must not discard durable fields")
		_, err = client.ForwardAppendBatch(context.Background(), 2, ch.AppendBatchRequest{ChannelID: ch.ChannelID{ID: "group", Type: 2}, Messages: []ch.Message{msg}})
		require.ErrorContains(t, err, "requires channel codec v8")
		require.Equal(t, 1, calls, "cached old peers must fail before sending")
	}
}

func TestAuthorityReadsRemainCompatibleWithV7Peers(t *testing.T) {
	network := clusternet.NewLocalNetwork()
	channels.RegisterHandlers(network, 2, v7AuthorityServer{})
	client := channels.NewTransportClient(v7AuthorityCaller{network})
	read, err := client.Pull(context.Background(), 2, channeltransport.PullRequest{ChannelKey: "2:old-peer", NeedMeta: true})
	require.NoError(t, err)
	require.NotNil(t, read.Meta)
	require.Equal(t, uint64(23), read.Meta.RetentionThroughSeq)
	require.Equal(t, uint64(7), read.Meta.WriteFence.Version)
}

type v7AuthorityServer struct{ channeltransport.Server }

func (v7AuthorityServer) HandlePull(context.Context, channeltransport.PullRequest) (channeltransport.PullResponse, error) {
	return channeltransport.PullResponse{ChannelKey: "2:old-peer", Meta: &ch.Meta{Key: "2:old-peer", ID: ch.ChannelID{ID: "old-peer", Type: 2}, Epoch: 1, LeaderEpoch: 1, Leader: 2, Replicas: []ch.NodeID{2}, ISR: []ch.NodeID{2}, MinISR: 1, Status: ch.StatusActive, RetentionThroughSeq: 23, WriteFence: ch.WriteFence{Version: 7}}}, nil
}

type v7AuthorityCaller struct{ network *clusternet.LocalNetwork }

func (c v7AuthorityCaller) Call(ctx context.Context, node uint64, service uint8, payload []byte) ([]byte, error) {
	// These record-free authority frames have identical v7/v8 bodies. Exercise
	// the public peer boundary in both directions with the actual v7 header.
	payload = bytes.Clone(payload)
	payload[0] = 7
	response, err := c.network.Call(ctx, node, service, payload)
	if err == nil {
		response = bytes.Clone(response)
		response[0] = 7
	}
	return response, err
}
