package channels

import (
	"context"

	ch "github.com/WuKongIM/WuKongIM/pkg/channel"
	"github.com/WuKongIM/WuKongIM/pkg/channel/replication"
)

// ActivateReplicaForRepair resolves authoritative membership, activates a cold
// replica if needed, and reads native follower durability independently of stale
// reactor progress. Leaders retain their recovered reactor proof. This leaves
// the observational RuntimeProbe contract used by diagnostics unchanged.
func (s *Service) ActivateReplicaForRepair(ctx context.Context, id ch.ChannelID) (ch.RuntimeProbeChannel, error) {
	if err := ctxErr(ctx); err != nil {
		return ch.RuntimeProbeChannel{}, err
	}
	if s == nil || s.metaSource == nil || s.runtime == nil {
		return ch.RuntimeProbeChannel{}, ch.ErrNotReady
	}
	meta, err := s.metaSource.ResolveChannelMeta(ctx, id)
	if err != nil {
		return ch.RuntimeProbeChannel{}, err
	}
	if meta.ID != id || meta.Status != ch.StatusActive || !localNodeIsReplica(meta, s.localNode) {
		return ch.RuntimeProbeChannel{}, ch.ErrStaleMeta
	}
	applier, ok := s.runtime.(runtimeMetaContextApplier)
	if !ok {
		return ch.RuntimeProbeChannel{}, ch.ErrInvalidConfig
	}
	if err := applier.ApplyMetaContext(ctx, meta); err != nil {
		return ch.RuntimeProbeChannel{}, err
	}
	result, err := s.RuntimeProbe(ctx, ch.RuntimeSelector{ChannelIDs: []ch.ChannelID{id}})
	if err != nil {
		return ch.RuntimeProbeChannel{}, err
	}
	for _, proof := range result.Channels {
		if proof.ChannelID != id {
			continue
		}
		if proof.ChannelEpoch != meta.Epoch || proof.LeaderEpoch != meta.LeaderEpoch || proof.Status != ch.StatusActive {
			return ch.RuntimeProbeChannel{}, ch.ErrStaleMeta
		}

		if proof.Role == ch.RoleFollower && s.replicaStore != nil {
			loaded, err := s.replicaStore.Load(ctx, replication.LoadBatch{Items: []replication.LoadRequest{{ChannelKey: ch.ChannelKeyForID(id), ChannelID: id}}})
			if err != nil {
				return ch.RuntimeProbeChannel{}, err
			}
			if len(loaded.Items) != 1 {
				return ch.RuntimeProbeChannel{}, ch.ErrLogConflict
			}
			if err := loaded.Items[0].Err; err != nil {
				return ch.RuntimeProbeChannel{}, err
			}
			// A concurrent authority change invalidates the proof even though
			// the underlying exact state read itself was consistent.
			latest, err := s.metaSource.ResolveChannelMeta(ctx, id)
			if err != nil {
				return ch.RuntimeProbeChannel{}, err
			}
			if latest.Epoch != meta.Epoch || latest.LeaderEpoch != meta.LeaderEpoch || latest.RouteGeneration != meta.RouteGeneration || latest.Leader != meta.Leader || latest.WriteFence != meta.WriteFence || latest.Status != meta.Status || !localNodeIsReplica(latest, s.localNode) {
				return ch.RuntimeProbeChannel{}, ch.ErrStaleMeta
			}
			state := loaded.Items[0].State
			proof.LEO, proof.HW, proof.CheckpointHW = state.LEO, state.Committed, state.Committed
		}
		return proof, nil
	}
	return ch.RuntimeProbeChannel{}, ch.ErrChannelNotFound
}
