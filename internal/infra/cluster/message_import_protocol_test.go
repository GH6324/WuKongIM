package cluster_test

import (
	"bytes"
	"context"
	api "github.com/WuKongIM/WuKongIM/internal/access/api"
	"github.com/WuKongIM/WuKongIM/internal/usecase/cmdsync"
	"github.com/WuKongIM/WuKongIM/pkg/db/meta"
	"net/http/httptest"
	"testing"

	infra "github.com/WuKongIM/WuKongIM/internal/infra/cluster"
	"github.com/WuKongIM/WuKongIM/internal/usecase/message"
	ch "github.com/WuKongIM/WuKongIM/pkg/channel"
	"github.com/WuKongIM/WuKongIM/pkg/channel/store"
	transport "github.com/WuKongIM/WuKongIM/pkg/channel/transport"
	"github.com/WuKongIM/WuKongIM/pkg/cluster/channels"
	net "github.com/WuKongIM/WuKongIM/pkg/cluster/net"
	"github.com/stretchr/testify/require"
)

func TestImportedFieldsReachMessageUsecaseThroughRoutedCommittedRead(t *testing.T) {
	ctx := context.Background()
	id := ch.ChannelID{ID: "imported-read", Type: 2}
	key := ch.ChannelKeyForID(id)
	factory := store.NewMessageDBFactory(t.TempDir())
	defer factory.Close()
	log, err := factory.ChannelStore(key, id)
	require.NoError(t, err)
	record := ch.Record{ID: 9007199254740999, Index: 1, Epoch: 1, Setting: 2, SyncOnce: true, ServerTimestampMS: 1700000001000,
		FromUID: "user", ClientMsgNo: "client", Payload: []byte{0, 255}, Protocol: ch.ProtocolFields{FramerFlags: 10, Expire: 3600, Topic: "topic", StreamNo: "stream", Timestamp: 1700000001}}
	manifest, _, ok := ch.SealProposalManifest(ch.ProposalManifest{Version: ch.FullMessageProposalVersion, ChannelEpoch: 1, LeaderTerm: 1, FenceVersion: 1, CommandID: ch.CommandID{1}, LastOffset: 1}, []ch.Record{record})
	require.True(t, ok)
	_, err = log.AppendLeader(ctx, store.AppendLeaderRequest{Records: []ch.Record{record}, ExactBaseOffset: true, Proposal: manifest, Committed: 1})
	require.NoError(t, err)
	require.NoError(t, log.Close())
	metas := channels.NewStaticMetaSource([]ch.Meta{{ID: id, Epoch: 1, LeaderEpoch: 1, Leader: 2, Replicas: []ch.NodeID{1, 2}, ISR: []ch.NodeID{1, 2}, MinISR: 2, Status: ch.StatusActive}})
	network := net.NewLocalNetwork()
	destination, err := channels.NewService(channels.Config{Runtime: unusedReadRuntime{}, LocalNode: 2, MetaSource: metas, Store: factory})
	require.NoError(t, err)
	channels.RegisterServiceHandlers(network, 2, destination)
	origin, err := channels.NewService(channels.Config{Runtime: unusedReadRuntime{}, LocalNode: 1, MetaSource: metas, Forward: channels.NewTransportClient(network)})
	require.NoError(t, err)
	routed, err := origin.ReadCommittedBatch(ctx, []channels.CommittedRead{{ChannelID: id, Request: store.ReadCommittedRequest{FromSeq: 1, Limit: 10, MaxBytes: 64 << 10}}})
	require.NoError(t, err)
	require.Len(t, routed, 1)
	require.NoError(t, routed[0].Err)
	require.Len(t, routed[0].Read.Messages, 1)
	require.Equal(t, record.Protocol, routed[0].Read.Messages[0].Protocol)
	require.True(t, routed[0].Read.Messages[0].SyncOnce)
	reader := infra.NewCommittedMessageReader(routedReadNode{service: origin})
	response, err := reader.ReadCommittedMessages(ctx, []message.CommittedMessageQuery{{ChannelID: message.ChannelID{ID: id.ID, Type: id.Type}, FromSeq: 1, Limit: 10, MaxBytes: 64 << 10}})
	require.NoError(t, err)
	require.NoError(t, response[0].Err)
	require.Len(t, response[0].Messages, 1)
	got := response[0].Messages[0]
	require.Equal(t, record.ID, got.MessageID)
	require.Equal(t, record.Payload, got.Payload)
	require.Equal(t, record.Protocol.Expire, got.Expire)
	require.Equal(t, record.Protocol.Topic, got.Topic)
	require.Equal(t, record.Protocol.Timestamp, got.Timestamp)
	require.True(t, got.Flags.RedDot)
	require.True(t, got.Flags.SyncOnce)
	cmdStore := infra.NewCMDSyncStore(cmdRoutedReadNode{service: origin})
	commands, err := cmdStore.LoadCommandMessages(ctx, cmdsync.CommandChannelKey{ChannelID: id.ID, ChannelType: id.Type}, 1, 1)
	require.NoError(t, err)
	require.Len(t, commands, 1)
	server := api.New(api.Options{CMDSync: fixedCMDResult{result: cmdsync.SyncResult{Messages: commands}}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/message/sync", bytes.NewBufferString(`{"uid":"user","limit":1}`))
	req.Header.Set("Content-Type", "application/json")
	server.Handler().ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code)
	require.JSONEq(t, `[{"header":{"no_persist":0,"red_dot":1,"sync_once":1},"setting":2,"message_id":9007199254740999,"message_idstr":"9007199254740999","client_msg_no":"client","message_seq":1,"from_uid":"user","channel_id":"imported-read","channel_type":2,"topic":"topic","expire":3600,"timestamp":1700000001,"payload":"AP8="}]`, rec.Body.String())

}

// This read test exercises the persisted checkpoint path. No runtime write or
// protocol handler is expected; embedded nil interfaces fail on accidental use.
type unusedReadRuntime struct {
	ch.Cluster
	transport.Server
}
type routedReadNode struct{ service *channels.Service }

func (n routedReadNode) ReadChannelCommittedBatch(ctx context.Context, reads []channels.CommittedRead) ([]channels.CommittedReadResult, error) {
	return n.service.ReadCommittedBatch(ctx, reads)
}
func (n routedReadNode) ReadChannelCommitted(ctx context.Context, id ch.ChannelID, req store.ReadCommittedRequest) (store.ReadCommittedResult, error) {
	r, e := n.service.ReadCommittedBatch(ctx, []channels.CommittedRead{{ChannelID: id, Request: req}})
	if e != nil {
		return store.ReadCommittedResult{}, e
	}
	return r[0].Read, r[0].Err
}

type cmdRoutedReadNode struct {
	infra.CMDSyncNode
	service *channels.Service
}

func (n cmdRoutedReadNode) ReadChannelCommittedBatch(ctx context.Context, reads []channels.CommittedRead) ([]channels.CommittedReadResult, error) {
	return n.service.ReadCommittedBatch(ctx, reads)
}
func (n cmdRoutedReadNode) GetChannelMetadataAuthoritative(context.Context, string, int64) (meta.Channel, error) {
	return meta.Channel{}, meta.ErrNotFound
}

type fixedCMDResult struct {
	api.CMDSyncUsecase
	result cmdsync.SyncResult
}

func (f fixedCMDResult) Sync(context.Context, cmdsync.SyncQuery) (cmdsync.SyncResult, error) {
	return f.result, nil
}
