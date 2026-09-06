package message_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/WuKongIM/WuKongIM/pkg/db/message"
	"github.com/WuKongIM/WuKongIM/pkg/db/message/channelcompat"
	"github.com/WuKongIM/WuKongIM/pkg/quorumlog"
	"github.com/stretchr/testify/require"
)

func TestImportedRetainedPrefixPreservesSequenceWithoutInventingMessages(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	key := channelcompat.ChannelKey("2:retained-import")
	id := channelcompat.ChannelID{ID: "retained-import", Type: 2}
	engine, err := message.OpenWithLogger(dir, nil)
	require.NoError(t, err)
	log, err := engine.ForChannel(key, id)
	require.NoError(t, err)
	prefix, boundary, ok := quorumlog.NewImportedPrefix(string(key), [32]byte{1}, 256)
	require.True(t, ok)
	outcome, err := log.InstallImportedPrefix(ctx, prefix)
	require.NoError(t, err)
	require.True(t, outcome.Durable())
	initial, err := log.LoadDurableFrontier(ctx)
	require.NoError(t, err)
	require.Equal(t, prefix, initial.Prefix)
	rows, err := log.Read(0, 64<<10)
	require.NoError(t, err)
	require.Empty(t, rows)
	outcome, err = log.InstallImportedPrefix(ctx, prefix)
	require.NoError(t, err)
	require.Equal(t, quorumlog.AppendOutcomeAlreadyDurable, outcome)

	msg := channelcompat.Message{MessageID: 9007199254740999, MessageSeq: 257, ChannelID: id.ID, ChannelType: id.Type, ServerTimestampMS: 1700000001000, Payload: []byte("retained")}
	record, err := message.EncodeMessageRecord(msg, 1)
	require.NoError(t, err)
	manifest, _, ok := quorumlog.SealProposalManifest(quorumlog.ProposalManifest{
		Version: quorumlog.FullMessageProposalVersion, ChannelEpoch: 1, LeaderTerm: 1, FenceVersion: 1,
		CommandID: quorumlog.CommandID{2}, BaseOffset: 256, LastOffset: 257,
		PreviousIndex: boundary.Index, PreviousTerm: boundary.LeaderTerm, PreviousDigest: boundary.Digest,
	}, []quorumlog.Record{{ID: msg.MessageID, Index: 257, Epoch: 1, ServerTimestampMS: msg.ServerTimestampMS, Payload: msg.Payload}})
	require.True(t, ok)
	results := message.StoreAppendBatch(ctx, []message.AppendBatchItem{{Store: log, Records: []channelcompat.Record{record}, ExactBaseOffset: true, ExpectedBaseOffset: 256, Proposal: manifest, Committed: 257}})
	require.Len(t, results, 1)
	require.NoError(t, results[0].Err)
	require.True(t, results[0].Outcome.Durable())
	require.NoError(t, log.Close())
	require.NoError(t, engine.Close())

	engine, err = message.OpenWithLogger(dir, nil)
	require.NoError(t, err)
	defer engine.Close()
	log, err = engine.ForChannel(key, id)
	require.NoError(t, err)
	defer log.Close()
	frontier, err := log.LoadDurableFrontier(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(257), frontier.LEO)
	require.Equal(t, uint64(257), frontier.Committed)
	require.Equal(t, prefix, frontier.Prefix)
	rows, err = log.Read(0, 64<<10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, uint64(257), rows[0].Index)
	require.Equal(t, msg.MessageID, rows[0].ID)
	stream, err := engine.OpenBackupSnapshot(ctx, message.BackupSnapshotRequest{HashSlot: 7, Channels: []message.BackupChannelCut{{
		Key: message.ChannelKey(key), ID: message.ChannelID{ID: id.ID, Type: id.Type}, Checkpoint: message.Checkpoint{Epoch: 1, LogStartOffset: 256, HW: 257},
	}}})
	require.NoError(t, err)
	body, err := io.ReadAll(stream)
	require.NoError(t, err)
	require.NoError(t, stream.Close())
	restored, err := message.OpenWithLogger(t.TempDir(), nil)
	require.NoError(t, err)
	defer restored.Close()
	stats, err := restored.ImportBackupSnapshotReader(ctx, bytes.NewReader(body), int64(len(body)))
	require.NoError(t, err)
	require.Equal(t, uint64(1), stats.MessageCount)
	restoredLog, err := restored.ForChannel(key, id)
	require.NoError(t, err)
	defer restoredLog.Close()
	restoredFrontier, err := restoredLog.LoadDurableFrontier(ctx)
	require.NoError(t, err)
	require.Equal(t, frontier, restoredFrontier)

}
