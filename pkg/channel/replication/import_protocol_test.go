package replication_test

import (
	"context"
	"strings"
	"testing"

	ch "github.com/WuKongIM/WuKongIM/pkg/channel"
	"github.com/WuKongIM/WuKongIM/pkg/channel/replication"
	"github.com/WuKongIM/WuKongIM/pkg/channel/store"
	"github.com/stretchr/testify/require"
)

func TestImportedProtocolFieldsSurviveReplicationExchange(t *testing.T) {
	id := ch.ChannelID{ID: "群:一-2", Type: 2}
	records := []ch.Record{{ID: 9007199254740999, Index: 1, Epoch: 1, FromUID: "用户:一", ClientMsgNo: "client-一", ServerTimestampMS: 1700000001000, SyncOnce: true, Payload: []byte{0, 1, 255, 0},
		Protocol: ch.ProtocolFields{FramerFlags: 10, Expire: 3600, Timestamp: 1700000001, Topic: "topic-一", StreamNo: "stream-一", MsgKey: "key", ClientSeq: 11, StreamID: 17, StreamFlag: 2}}}
	manifest, _, ok := ch.SealProposalManifest(ch.ProposalManifest{Version: ch.FullMessageProposalVersion, ChannelEpoch: 1, LeaderTerm: 1, FenceVersion: 1, CommandID: ch.CommandID{31: 1}, LastOffset: 1}, records)
	require.True(t, ok)
	batch := replication.ExchangeBatch{Version: replication.ExchangeVersion, Items: []replication.ExchangeItem{{RequestID: 1, Kind: replication.ExchangeReplicate, Replicate: &replication.ReplicateRequest{
		ChannelKey: ch.ChannelKeyForID(id), ChannelID: id, Leader: 1, Follower: 2, Manifest: manifest, Records: records, Committed: 1,
	}}}}
	b, err := replication.EncodeExchangeBatch(batch)
	require.NoError(t, err)
	got, err := replication.DecodeExchangeBatch(b)
	require.NoError(t, err)
	require.Equal(t, batch, got)

	result := replication.ExchangeBatchResult{Version: replication.ExchangeVersion, Items: []replication.ExchangeItemResult{{RequestID: 1, Fetch: replication.FetchResult{Proposals: []replication.RecoveryProposal{{Manifest: manifest, Records: records}}}}}}
	b, err = replication.EncodeExchangeBatchResult(result)
	require.NoError(t, err)
	gotResult, err := replication.DecodeExchangeBatchResult(b)
	require.NoError(t, err)
	require.Equal(t, result, gotResult)
}

func TestImportedProtocolFieldsCountAgainstReplicaByteBudget(t *testing.T) {
	a, err := replication.NewStoreAdapter(replication.StoreAdapterConfig{Factory: store.NewMemoryFactory(), MaxBatchItems: 4, MaxBatchBytes: 4096})
	require.NoError(t, err)
	records := []ch.Record{{ID: 1, Index: 1, Epoch: 1, ServerTimestampMS: 1, Protocol: ch.ProtocolFields{Topic: strings.Repeat("x", 8192)}}}
	manifest, _, ok := ch.SealProposalManifest(ch.ProposalManifest{Version: ch.FullMessageProposalVersion, ChannelEpoch: 1, LeaderTerm: 1, FenceVersion: 1, CommandID: ch.CommandID{1}, LastOffset: 1}, records)
	require.True(t, ok)
	result := a.Sync(context.Background(), []replication.Mutation{{ChannelKey: "2:budget", ChannelID: ch.ChannelID{ID: "budget", Type: 2}, Records: records, Manifest: manifest}})
	require.Len(t, result, 1)
	require.ErrorIs(t, result[0].Err, ch.ErrBackpressured)
	require.False(t, result[0].Outcome.Durable())
}

func TestImportedProtocolFieldsRepairAnEmptyReplica(t *testing.T) {
	ctx := context.Background()
	id := ch.ChannelID{ID: "repair-import", Type: 2}
	key := ch.ChannelKeyForID(id)
	open := func() (replication.ReplicaStore, *store.MessageDBFactory) {
		f := store.NewMessageDBFactory(t.TempDir())
		t.Cleanup(func() { require.NoError(t, f.Close()) })
		a, err := replication.NewStoreAdapter(replication.StoreAdapterConfig{Factory: f, MaxBatchItems: 4, MaxBatchBytes: 64 << 10})
		require.NoError(t, err)
		return a, f
	}
	donor, _ := open()
	target, targetFactory := open()
	records := []ch.Record{{ID: 9007199254740999, Index: 1, Epoch: 1, FromUID: "user", ClientMsgNo: "client", ServerTimestampMS: 1700000001000, Payload: []byte("original"), Protocol: ch.ProtocolFields{Expire: 3600, FramerFlags: 2, Topic: "topic", StreamNo: "stream", Timestamp: 1700000001}}}
	manifest, _, ok := ch.SealProposalManifest(ch.ProposalManifest{Version: ch.FullMessageProposalVersion, ChannelEpoch: 1, LeaderTerm: 1, FenceVersion: 1, CommandID: ch.CommandID{1}, LastOffset: 1}, records)
	require.True(t, ok)
	results := donor.Sync(ctx, []replication.Mutation{{ChannelKey: key, ChannelID: id, Manifest: manifest, Records: records, Committed: 1}})
	require.Len(t, results, 1)
	require.NoError(t, results[0].Err)
	loaded, err := donor.Load(ctx, replication.LoadBatch{Items: []replication.LoadRequest{{ChannelKey: key, ChannelID: id, ProbeIndexes: []uint64{1}}}})
	require.NoError(t, err)
	require.NoError(t, loaded.Items[0].Err)
	pages := donor.Fetch(ctx, []replication.FetchRange{{ChannelKey: key, ChannelID: id, Expected: loaded.Items[0].State, From: 1, Through: 1, MaxBytes: 32 << 10}})
	require.Len(t, pages, 1)
	require.NoError(t, pages[0].Err)
	repaired := target.Replace(ctx, []replication.RecoveryReplacement{{ChannelKey: key, ChannelID: id, Proposals: pages[0].Proposals, Committed: 1}})
	require.Len(t, repaired, 1)
	require.NoError(t, repaired[0].Err)
	require.True(t, repaired[0].Outcome.Durable())
	log, err := targetFactory.ChannelStore(key, id)
	require.NoError(t, err)
	defer log.Close()
	read, err := log.ReadCommitted(ctx, store.ReadCommittedRequest{FromSeq: 1, Limit: 10, MaxBytes: 64 << 10})
	require.NoError(t, err)
	require.Len(t, read.Messages, 1)
	require.Equal(t, records[0].Protocol, read.Messages[0].Protocol)
	require.Equal(t, records[0].Payload, read.Messages[0].Payload)
}
