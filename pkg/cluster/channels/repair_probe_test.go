package channels

import (
	"context"
	"testing"

	ch "github.com/WuKongIM/WuKongIM/pkg/channel"
	"github.com/stretchr/testify/require"
)

type repairActivationRuntime struct {
	benchRuntimeFake
	activations   int
	activationErr error
}

func (r *repairActivationRuntime) ApplyMetaContext(ctx context.Context, _ ch.Meta) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.activations++
	return r.activationErr
}

func TestRepairActivationRejectsForeignReplicaAndInactiveMetadata(t *testing.T) {
	id := ch.ChannelID{ID: "repair", Type: 2}
	for _, tc := range []struct {
		name     string
		replicas []ch.NodeID
		status   ch.Status
	}{
		{"foreign", []ch.NodeID{1, 3}, ch.StatusActive},
		{"inactive", []ch.NodeID{1, 2}, ch.StatusCreating},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime := &repairActivationRuntime{}
			svc, err := NewService(Config{LocalNode: 2, Runtime: runtime, MetaSource: NewStaticMetaSource([]ch.Meta{{ID: id, Epoch: 1, LeaderEpoch: 1, Leader: 1, Replicas: tc.replicas, Status: tc.status}})})
			require.NoError(t, err)
			_, err = svc.ActivateReplicaForRepair(context.Background(), id)
			require.ErrorIs(t, err, ch.ErrStaleMeta)
			require.Zero(t, runtime.activations)
		})
	}
}

func TestRepairActivationDoesNotInventProofOnFailureOrMetadataRace(t *testing.T) {
	id := ch.ChannelID{ID: "repair", Type: 2}
	runtime := &repairActivationRuntime{}
	svc, err := NewService(Config{LocalNode: 2, Runtime: runtime, MetaSource: NewStaticMetaSource([]ch.Meta{{ID: id, Epoch: 1, LeaderEpoch: 1, Leader: 1, Replicas: []ch.NodeID{1, 2}, Status: ch.StatusActive}})})
	require.NoError(t, err)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = svc.ActivateReplicaForRepair(canceled, id)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, runtime.activations)
	runtime.activationErr = ch.ErrNotReady
	_, err = svc.ActivateReplicaForRepair(context.Background(), id)
	require.ErrorIs(t, err, ch.ErrNotReady)
	require.Zero(t, runtime.probeCalls)
	runtime.activationErr = nil
	_, err = svc.ActivateReplicaForRepair(context.Background(), id)
	require.ErrorIs(t, err, ch.ErrChannelNotFound)
	runtime.probe = ch.RuntimeProbeResult{Channels: []ch.RuntimeProbeChannel{{ChannelID: id, ChannelEpoch: 1, LeaderEpoch: 2, Status: ch.StatusActive}}}
	_, err = svc.ActivateReplicaForRepair(context.Background(), id)
	require.ErrorIs(t, err, ch.ErrStaleMeta)
}
