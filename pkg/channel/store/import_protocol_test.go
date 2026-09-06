package store_test

import (
	"context"
	"fmt"
	"testing"

	ch "github.com/WuKongIM/WuKongIM/pkg/channel"
	"github.com/WuKongIM/WuKongIM/pkg/channel/store"
	"github.com/stretchr/testify/require"
)

func TestImportedProtocolFieldsSurviveExactStoreRecovery(t *testing.T) {
	dir := t.TempDir()
	id := ch.ChannelID{ID: "群:一-2", Type: 2}
	record := ch.Record{
		ID: 9007199254740999, Index: 1, Epoch: 1, FromUID: "用户:一", ClientMsgNo: "client-一",
		ServerTimestampMS: 1700000001000, SyncOnce: true, Payload: []byte{0, 1, 255, 0},
		Protocol: ch.ProtocolFields{FramerFlags: 10, Expire: 3600, Timestamp: 1700000001, Topic: "topic-一", StreamNo: "stream-一"},
	}
	manifest, _, ok := ch.SealProposalManifest(ch.ProposalManifest{
		Version: ch.FullMessageProposalVersion, ChannelEpoch: 1, LeaderTerm: 1, FenceVersion: 1,
		CommandID: ch.CommandID{31: 1}, LastOffset: 1,
	}, []ch.Record{record})
	require.True(t, ok)
	factory := store.NewMessageDBFactory(dir)
	log, err := factory.ChannelStore(ch.ChannelKeyForID(id), id)
	require.NoError(t, err)
	result, err := log.AppendLeader(context.Background(), store.AppendLeaderRequest{Records: []ch.Record{record}, ExactBaseOffset: true, Proposal: manifest})
	require.NoError(t, err)
	require.True(t, result.Outcome.Durable())
	require.NoError(t, log.StoreCheckpoint(context.Background(), ch.Checkpoint{HW: 1}))
	require.NoError(t, log.Close())
	require.NoError(t, factory.Close())

	factory = store.NewMessageDBFactory(dir)
	defer factory.Close()
	log, err = factory.ChannelStore(ch.ChannelKeyForID(id), id)
	require.NoError(t, err)
	defer log.Close()
	page, err := log.(store.ExactRecoveryPageReader).ReadExactRecoveryPage(context.Background(), store.ExactRecoveryPageRequest{From: 1, Through: 1, MaxBytes: 64 << 10})
	require.NoError(t, err)
	require.Len(t, page.Records, 1)
	require.Equal(t, record.Protocol, page.Records[0].Protocol)
	require.Equal(t, record.Payload, page.Records[0].Payload)
	sealed, _, ok := ch.SealProposalManifest(manifest, page.Records)
	require.True(t, ok)
	require.Equal(t, manifest, sealed)
	read, err := log.ReadCommitted(context.Background(), store.ReadCommittedRequest{FromSeq: 1, Limit: 10, MaxBytes: 64 << 10})
	require.NoError(t, err)
	require.Len(t, read.Messages, 1)
	require.Equal(t, record.Protocol, read.Messages[0].Protocol)
}

func TestImportedHistoricalTimestampsAreNotReplacedWithCurrentTime(t *testing.T) {
	for _, timestamp := range []int64{0, -1000} {
		t.Run(fmt.Sprint(timestamp), func(t *testing.T) {
			ctx := context.Background()
			f := store.NewMessageDBFactory(t.TempDir())
			defer f.Close()
			id := ch.ChannelID{ID: "historical-time", Type: 2}
			key := ch.ChannelKeyForID(id)
			log, err := f.ChannelStore(key, id)
			require.NoError(t, err)
			defer log.Close()
			records := []ch.Record{{ID: 1, Index: 1, Epoch: 1, ServerTimestampMS: timestamp, Protocol: ch.ProtocolFields{Timestamp: int32(timestamp / 1000)}}}
			manifest, _, ok := ch.SealProposalManifest(ch.ProposalManifest{Version: ch.FullMessageProposalVersion, ChannelEpoch: 1, LeaderTerm: 1, FenceVersion: 1, CommandID: ch.CommandID{1}, LastOffset: 1}, records)
			require.True(t, ok)
			result, err := log.AppendLeader(ctx, store.AppendLeaderRequest{Records: records, ExactBaseOffset: true, Proposal: manifest, Committed: 1})
			require.NoError(t, err)
			require.True(t, result.Outcome.Durable())
			read, err := log.ReadCommitted(ctx, store.ReadCommittedRequest{FromSeq: 1, Limit: 1, MaxBytes: 64 << 10})
			require.NoError(t, err)
			require.Len(t, read.Messages, 1)
			require.Equal(t, timestamp, read.Messages[0].ServerTimestampMS)
			require.Equal(t, int32(timestamp/1000), read.Messages[0].Protocol.Timestamp)
		})
	}
}
