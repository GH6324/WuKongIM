package replication_test

import (
	"context"
	"testing"

	ch "github.com/WuKongIM/WuKongIM/pkg/channel"
	"github.com/WuKongIM/WuKongIM/pkg/channel/replication"
	"github.com/WuKongIM/WuKongIM/pkg/channel/store"
	"github.com/WuKongIM/WuKongIM/pkg/db/message"
	"github.com/WuKongIM/WuKongIM/pkg/db/message/channelcompat"
	"github.com/WuKongIM/WuKongIM/pkg/quorumlog"
	"github.com/stretchr/testify/require"
)

func TestImportedPrefixSurvivesReplicaLoadAndExchange(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	id := ch.ChannelID{ID: "retained", Type: 2}
	key := ch.ChannelKeyForID(id)
	prefix, boundary, ok := quorumlog.NewImportedPrefix(string(key), [32]byte{1}, 100000)
	require.True(t, ok)
	db, err := message.OpenWithLogger(dir, nil)
	require.NoError(t, err)
	log, err := db.ForChannel(channelcompat.ChannelKey(key), channelcompat.ChannelID{ID: id.ID, Type: id.Type})
	require.NoError(t, err)
	_, err = log.InstallImportedPrefix(ctx, prefix)
	require.NoError(t, err)
	require.NoError(t, log.Close())
	require.NoError(t, db.Close())
	factory := store.NewMessageDBFactory(dir)
	defer factory.Close()
	a, err := replication.NewStoreAdapter(replication.StoreAdapterConfig{Factory: factory, MaxBatchItems: 4, MaxBatchBytes: 64 << 10})
	require.NoError(t, err)
	got, err := a.Load(ctx, replication.LoadBatch{Items: []replication.LoadRequest{{ChannelKey: key, ChannelID: id, ProbeIndexes: []uint64{1, 99999, 100000}}}})
	require.NoError(t, err)
	require.NoError(t, got.Items[0].Err)
	require.Equal(t, prefix, got.Items[0].State.Prefix)
	require.Equal(t, boundary, got.Items[0].State.TailIdentity)
	require.False(t, got.Items[0].Entries[0].Present)
	require.False(t, got.Items[0].Entries[1].Present)
	require.Equal(t, boundary, got.Items[0].Entries[2].Identity)
	result := replication.ExchangeBatchResult{Version: replication.ExchangeVersion, Items: []replication.ExchangeItemResult{{RequestID: 1, Probe: replication.ProbeResult{State: got.Items[0].State, Entries: got.Items[0].Entries}}}}
	b, err := replication.EncodeExchangeBatchResult(result)
	require.NoError(t, err)
	roundtrip, err := replication.DecodeExchangeBatchResult(b)
	require.NoError(t, err)
	require.Equal(t, result, roundtrip)
}

// An imported retained boundary may be installed only through recovery on an
// empty target. It must never be accepted as an ordinary message proposal.
func TestImportedPrefixRepairsEmptyReplicaAndRejectsConflictingOverwrite(t *testing.T) {
	ctx := context.Background()
	id := ch.ChannelID{ID: "repair-prefix", Type: 2}
	key := ch.ChannelKeyForID(id)
	prefix, boundary, ok := quorumlog.NewImportedPrefix(string(key), [32]byte{1}, 100000)
	require.True(t, ok)
	f := store.NewMessageDBFactory(t.TempDir())
	defer f.Close()
	a, err := replication.NewStoreAdapter(replication.StoreAdapterConfig{Factory: f, MaxBatchItems: 4, MaxBatchBytes: 64 << 10})
	require.NoError(t, err)
	ordinary := a.Sync(ctx, []replication.Mutation{{ChannelKey: key, ChannelID: id, Manifest: prefix, Committed: 100000}})
	require.Len(t, ordinary, 1)
	require.Error(t, ordinary[0].Err)
	repaired := a.Replace(ctx, []replication.RecoveryReplacement{{ChannelKey: key, ChannelID: id,
		Proposals: []replication.RecoveryProposal{{Manifest: prefix}}, Committed: 100000}})
	require.Len(t, repaired, 1)
	require.NoError(t, repaired[0].Err)
	require.True(t, repaired[0].Outcome.Durable())
	loaded, err := a.Load(ctx, replication.LoadBatch{Items: []replication.LoadRequest{{ChannelKey: key, ChannelID: id}}})
	require.NoError(t, err)
	require.NoError(t, loaded.Items[0].Err)
	require.Equal(t, replication.ReplicaState{Prefix: prefix, Manifest: prefix, TailIdentity: boundary, LEO: 100000, Committed: 100000}, loaded.Items[0].State)
	other, _, ok := quorumlog.NewImportedPrefix(string(key), [32]byte{2}, 100000)
	require.True(t, ok)
	conflict := a.Replace(ctx, []replication.RecoveryReplacement{{ChannelKey: key, ChannelID: id, Expected: loaded.Items[0].State,
		Proposals: []replication.RecoveryProposal{{Manifest: other}}, Committed: 100000}})
	require.Error(t, conflict[0].Err)
	log, err := f.ChannelStore(key, id)
	require.NoError(t, err)
	defer log.Close()
	read, err := log.ReadCommitted(ctx, store.ReadCommittedRequest{FromSeq: 1, Limit: 10, MaxBytes: 64 << 10})
	require.NoError(t, err)
	require.Empty(t, read.Messages)
}
