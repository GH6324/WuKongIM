package cluster

import (
	"context"
	"errors"
	"testing"

	ch "github.com/WuKongIM/WuKongIM/pkg/channel"
	channelstore "github.com/WuKongIM/WuKongIM/pkg/channel/store"
	metadb "github.com/WuKongIM/WuKongIM/pkg/db/meta"
)

// coldRepairRuntime models a durable replica whose reactor is not loaded yet.
type coldRepairRuntime struct {
	migrationRuntimeChannelService
	calls int
}

func (r *coldRepairRuntime) RuntimeProbe(context.Context, ch.RuntimeSelector) (ch.RuntimeProbeResult, error) {
	r.calls++
	if len(r.applied) == 0 {
		return ch.RuntimeProbeResult{}, nil
	}
	m := r.applied[len(r.applied)-1]
	return ch.RuntimeProbeResult{Channels: []ch.RuntimeProbeChannel{{ChannelID: m.ID, ChannelEpoch: m.Epoch, LeaderEpoch: m.LeaderEpoch, Role: ch.RoleFollower, Status: m.Status, LEO: 17, HW: 12, CheckpointHW: 12}}}, nil
}

func TestChannelRepairProbeActivatesColdAuthoritativeReplica(t *testing.T) {
	n, db := newLocalMetadataScanNode(t)
	n.defaultSlotProposer = fixedUnitChannelMigrationSlotRuntime{localLeader: true}
	id := ch.ChannelID{ID: keyForNodeHashSlot(t, 4, 0), Type: 2}
	meta := metadb.NormalizeChannelRuntimeMeta(metadb.ChannelRuntimeMeta{ChannelID: id.ID, ChannelType: 2, ChannelEpoch: 4, LeaderEpoch: 5, Leader: 2, Replicas: []uint64{1, 2, 3}, ISR: []uint64{1, 2, 3}, MinISR: 2, Status: uint8(ch.StatusActive)})
	if err := db.ForHashSlot(0).UpsertChannelRuntimeMeta(context.Background(), meta); err != nil {
		t.Fatal(err)
	}
	runtime := &coldRepairRuntime{}
	n.channels = runtime
	got, err := n.ProbeChannel(context.Background(), 1, id.ID, id.Type)
	if err != nil {
		t.Fatal(err)
	}
	if got.ChannelID != id || got.HW != 12 || got.LeaderEpoch != 5 || len(runtime.applied) != 1 || runtime.calls != 2 {
		t.Fatalf("probe=%+v applied=%+v calls=%d", got, runtime.applied, runtime.calls)
	}
}

func TestChannelRepairProbeDoesNotActivateNonReplica(t *testing.T) {
	n, db := newLocalMetadataScanNode(t)
	n.defaultSlotProposer = fixedUnitChannelMigrationSlotRuntime{localLeader: true}
	id := ch.ChannelID{ID: keyForNodeHashSlot(t, 4, 0), Type: 2}
	meta := metadb.NormalizeChannelRuntimeMeta(metadb.ChannelRuntimeMeta{ChannelID: id.ID, ChannelType: 2, ChannelEpoch: 4, LeaderEpoch: 5, Leader: 2, Replicas: []uint64{2, 3}, ISR: []uint64{2, 3}, MinISR: 2, Status: uint8(ch.StatusActive)})
	if err := db.ForHashSlot(0).UpsertChannelRuntimeMeta(context.Background(), meta); err != nil {
		t.Fatal(err)
	}
	runtime := &coldRepairRuntime{}
	n.channels = runtime
	_, err := n.ProbeChannel(context.Background(), 1, id.ID, id.Type)
	if !errors.Is(err, ch.ErrChannelNotFound) || len(runtime.applied) != 0 {
		t.Fatalf("err=%v applied=%+v", err, runtime.applied)
	}
}

// migrationProbeStore exposes a controlled exact read at the store boundary.
type migrationProbeStore struct {
	channelstore.ChannelStore
	state   channelstore.ExactState
	loadErr error
	onLoad  func()
	closed  bool
}

func (s *migrationProbeStore) LoadExactState(context.Context) (channelstore.ExactState, error) {
	if s.onLoad != nil {
		s.onLoad()
	}
	return s.state, s.loadErr
}
func (s *migrationProbeStore) Close() error { s.closed = true; return nil }

type migrationProbeFactory struct{ store *migrationProbeStore }

func (f migrationProbeFactory) ChannelStore(ch.ChannelKey, ch.ChannelID) (channelstore.ChannelStore, error) {
	return f.store, nil
}

func TestMigrationFollowerProgressRejectsChangedAuthorityAndClosesHandle(t *testing.T) {
	for _, mode := range []string{"fresh", "epoch_changed", "load_failed"} {
		t.Run(mode, func(t *testing.T) {
			n, _ := newLocalMetadataScanNode(t)
			meta := ch.Meta{ID: ch.ChannelID{ID: "probe-fence", Type: 2}, Epoch: 4, LeaderEpoch: 5, Status: ch.StatusActive}
			runtime := &coldRepairRuntime{}
			runtime.applied = []ch.Meta{meta}
			n.channels = runtime
			handle := &migrationProbeStore{state: channelstore.ExactState{InitialState: channelstore.InitialState{LEO: 20, HW: 19, CheckpointHW: 19}}}
			if mode == "epoch_changed" {
				handle.onLoad = func() { runtime.applied[0].Epoch++ }
			}
			if mode == "load_failed" {
				handle.loadErr = context.DeadlineExceeded
			}
			n.channelStoreFactory = migrationProbeFactory{store: handle}
			probe := ch.RuntimeProbeChannel{ChannelID: meta.ID, ChannelEpoch: 4, LeaderEpoch: 5, Role: ch.RoleFollower, Status: ch.StatusActive}
			got, err := n.refreshMigrationFollowerProgress(context.Background(), probe)
			switch mode {
			case "fresh":
				if err != nil || got.LEO != 20 || got.HW != 19 {
					t.Fatalf("fresh probe=%+v err=%v", got, err)
				}
			case "epoch_changed":
				if !errors.Is(err, ch.ErrStaleMeta) {
					t.Fatalf("changed epoch err=%v", err)
				}
			case "load_failed":
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("load failure err=%v", err)
				}
			}
			if !handle.closed {
				t.Fatal("probe leaked its store handle")
			}
		})
	}
}
