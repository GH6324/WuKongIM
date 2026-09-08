package cluster

import (
	"context"
	"testing"

	metadb "github.com/WuKongIM/WuKongIM/pkg/db/meta"
	"github.com/stretchr/testify/require"
)

func TestMigrationTaskScanYieldsPastBlockedTasksAndAcrossShards(t *testing.T) {
	var scan migrationTaskScan
	rows := map[uint16][]metadb.ChannelMigrationTask{
		1: {{ChannelID: "a", TaskID: "blocked", Status: metadb.ChannelMigrationStatusBlocked}, {ChannelID: "b", TaskID: "runnable"}},
		2: {{ChannelID: "c", TaskID: "other-shard"}},
	}
	reads := 0
	read := func(_ context.Context, slot uint16, cursor metadb.ChannelMigrationTaskCursor, limit int) ([]metadb.ChannelMigrationTask, metadb.ChannelMigrationTaskCursor, bool, error) {
		reads++
		require.Equal(t, 1, limit, "one durable phase must not discard other candidates in its page")
		for i, task := range rows[slot] {
			if task.ChannelID > cursor.ChannelID {
				return []metadb.ChannelMigrationTask{task}, metadb.ChannelMigrationTaskCursor{ChannelID: task.ChannelID}, i == len(rows[slot])-1, nil
			}
		}
		return nil, cursor, true, nil
	}
	// A permanently blocked first row must not starve a second row in the same
	// shard or another physical owner's shard, even with TaskLimit greater than one.
	for _, want := range []string{"blocked", "other-shard", "runnable", "other-shard", "blocked"} {
		got, err := scan.list(context.Background(), []uint16{1, 2}, 8, read)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, want, got[0].TaskID)
	}
	require.LessOrEqual(t, reads, 10, "at most one progress recheck plus one nonempty task page per call")
	_, err := scan.list(context.Background(), []uint16{2}, 1, read)
	require.NoError(t, err)
	require.NotContains(t, scan.cursors, uint16(1), "lost leadership must discard cursor")
	got, err := scan.list(context.Background(), []uint16{1}, 1, read)
	require.NoError(t, err)
	require.Equal(t, "blocked", got[0].TaskID)
}

func TestMigrationTaskScanKeepsBoundedDurableProgressBeforeRotating(t *testing.T) {
	var scan migrationTaskScan
	rows := []metadb.ChannelMigrationTask{{ChannelID: "a", TaskID: "progress", UpdatedAtMS: 1}, {ChannelID: "b", TaskID: "next", UpdatedAtMS: 1}}
	read := func(_ context.Context, _ uint16, cursor metadb.ChannelMigrationTaskCursor, _ int) ([]metadb.ChannelMigrationTask, metadb.ChannelMigrationTaskCursor, bool, error) {
		for i, task := range rows {
			if task.ChannelID > cursor.ChannelID {
				return []metadb.ChannelMigrationTask{task}, metadb.ChannelMigrationTaskCursor{ChannelID: task.ChannelID}, i == len(rows)-1, nil
			}
		}
		return nil, cursor, true, nil
	}
	got, err := scan.list(context.Background(), []uint16{1}, 1, read)
	require.NoError(t, err)
	require.Equal(t, "progress", got[0].TaskID)
	// A successful claim/phase changes the durable version. Keep a short quantum
	// so a busy queue does not expire every newly claimed lease before its next phase.
	for step := 0; step < 7; step++ {
		rows[0].UpdatedAtMS++
		got, err = scan.list(context.Background(), []uint16{1}, 1, read)
		require.NoError(t, err)
		require.Equal(t, "progress", got[0].TaskID)
	}
	rows[0].UpdatedAtMS++
	got, err = scan.list(context.Background(), []uint16{1}, 1, read)
	require.NoError(t, err)
	require.Equal(t, "next", got[0].TaskID, "even progressing work must yield its bounded quantum")
	// No durable progress (including a timed-out target) yields immediately.
	got, err = scan.list(context.Background(), []uint16{1}, 1, read)
	require.NoError(t, err)
	require.Equal(t, "progress", got[0].TaskID)
	got, err = scan.list(context.Background(), []uint16{1}, 1, read)
	require.NoError(t, err)
	require.Equal(t, "next", got[0].TaskID)
}

func TestMigrationTaskScanDropsProgressFocusWhenOwnershipIsLost(t *testing.T) {
	scan := migrationTaskScan{focus: &migrationTaskFocus{hashSlot: 1, remaining: 7}, cursors: map[uint16]metadb.ChannelMigrationTaskCursor{1: {ChannelID: "old"}}}
	got, err := scan.list(context.Background(), nil, 1, nil)
	require.NoError(t, err)
	require.Empty(t, got)
	require.Nil(t, scan.focus)
	require.Empty(t, scan.cursors)
}
