package cluster

import (
	"context"
	"sort"
	"sync"

	metadb "github.com/WuKongIM/WuKongIM/pkg/db/meta"
)

type migrationTaskPageReader func(context.Context, uint16, metadb.ChannelMigrationTaskCursor, int) ([]metadb.ChannelMigrationTask, metadb.ChannelMigrationTaskCursor, bool, error)

// migrationTaskScan retains only one cursor per owned hash slot. Every returned
// stalled page yields to the next shard; durable progress gets one short quantum.
type migrationTaskScan struct {
	mu      sync.Mutex
	cursors map[uint16]metadb.ChannelMigrationTaskCursor
	next    uint16
	focus   *migrationTaskFocus
}

// A short quantum amortizes claim/fence leases while still yielding stalled work.
const migrationTaskProgressQuantum = 8

type migrationTaskFocus struct {
	hashSlot  uint16
	before    metadb.ChannelMigrationTaskCursor
	task      metadb.ChannelMigrationTask
	remaining int
}

func (s *migrationTaskScan) list(ctx context.Context, owned []uint16, limit int, read migrationTaskPageReader) ([]metadb.ChannelMigrationTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cursors == nil {
		s.cursors = make(map[uint16]metadb.ChannelMigrationTaskCursor)
	}
	for hashSlot := range s.cursors {
		i := sort.Search(len(owned), func(i int) bool { return owned[i] >= hashSlot })
		if i == len(owned) || owned[i] != hashSlot {
			delete(s.cursors, hashSlot)
		}
	}
	if len(owned) == 0 || limit <= 0 {
		s.focus = nil
		return nil, nil
	}
	// Follow only observed durable progress. A timeout, unchanged row, terminal
	// state, blocked task, ownership loss, or exhausted quantum releases focus.
	if f := s.focus; f != nil {
		s.focus = nil
		ownedIndex := sort.Search(len(owned), func(i int) bool { return owned[i] >= f.hashSlot })
		if f.remaining > 0 && ownedIndex < len(owned) && owned[ownedIndex] == f.hashSlot {
			page, _, _, err := read(ctx, f.hashSlot, f.before, 1)
			if err != nil {
				return nil, err
			}
			if len(page) == 1 {
				task := page[0]
				if task.TaskID == f.task.TaskID && task.ChannelID == f.task.ChannelID && task.ChannelType == f.task.ChannelType && task.UpdatedAtMS > f.task.UpdatedAtMS && !task.IsTerminal() && task.Status != metadb.ChannelMigrationStatusBlocked {
					f.task = task
					f.remaining--
					s.focus = f
					return page, nil
				}
			}
		}
	}
	start := sort.Search(len(owned), func(i int) bool { return owned[i] >= s.next })
	// At most one page per owned shard and one task candidate per tick. A sparse
	// shard is cheap; no task count or unavailable peer can grow the scan's memory.
	out := make([]metadb.ChannelMigrationTask, 0, 1)
	for i := 0; i < len(owned) && len(out) < 1; i++ {
		if err := ctxErr(ctx); err != nil {
			return nil, err
		}
		index := (start + i) % len(owned)
		hashSlot := owned[index]
		before := s.cursors[hashSlot]
		tasks, cursor, done, err := read(ctx, hashSlot, before, 1)
		if err != nil {
			return nil, err
		}
		if done {
			delete(s.cursors, hashSlot)
		} else {
			s.cursors[hashSlot] = cursor
		}
		s.next = owned[(index+1)%len(owned)]
		if len(tasks) == 1 && !tasks[0].IsTerminal() && tasks[0].Status != metadb.ChannelMigrationStatusBlocked {
			s.focus = &migrationTaskFocus{hashSlot: hashSlot, before: before, task: tasks[0], remaining: migrationTaskProgressQuantum - 1}
		}
		out = append(out, tasks...)
	}
	return out, nil
}
