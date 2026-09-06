package message

import (
	"context"

	"github.com/WuKongIM/WuKongIM/pkg/db/internal/dberrors"
	"github.com/WuKongIM/WuKongIM/pkg/db/internal/engine"
	"github.com/WuKongIM/WuKongIM/pkg/db/internal/keycodec"
	channel "github.com/WuKongIM/WuKongIM/pkg/db/message/channelcompat"
	"github.com/WuKongIM/WuKongIM/pkg/quorumlog"
)

// InstallImportedPrefix initializes an empty Channel's original retained
// sequence boundary without adding message rows or indexes. Same-proof retries
// are idempotent, including after subsequent imported appends. The importer
// owns source authority checks and target admission fencing above this seam.
func (s *ChannelStore) InstallImportedPrefix(ctx context.Context, prefix quorumlog.ProposalManifest) (quorumlog.AppendOutcome, error) {
	entry, ok := quorumlog.ImportedPrefixEntry(prefix)
	if ctx == nil || !ok {
		return quorumlog.AppendOutcomeDefinitelyNotWritten, channel.ErrInvalidArgument
	}
	if err := s.beginUse(); err != nil {
		return quorumlog.AppendOutcomeDefinitelyNotWritten, err
	}
	defer s.endUse()
	if err := ctx.Err(); err != nil {
		return quorumlog.AppendOutcomeDefinitelyNotWritten, err
	}
	s.log.appendMu.Lock()
	defer s.log.appendMu.Unlock()
	s.log.checkpointMu.Lock()
	defer s.log.checkpointMu.Unlock()
	current, _, err := s.loadDurableFrontierLocked(ctx)
	if err != nil {
		return quorumlog.AppendOutcomeConflict, toChannelError(err)
	}
	if current.LEO != 0 {
		stored, present, err := loadDurableProposalPairByLast(s.log.db.engine, s.log.key, prefix.LastOffset)
		if err != nil {
			return quorumlog.AppendOutcomeConflict, toChannelError(err)
		}
		boundary, found, err := loadDurableEntryIdentityFrom(s.log.db.engine, s.log.key, prefix.LastOffset)
		if err != nil {
			return quorumlog.AppendOutcomeConflict, toChannelError(err)
		}
		retention, hasRetention, err := s.log.loadRetentionState(ctx)
		if err != nil {
			return quorumlog.AppendOutcomeConflict, toChannelError(err)
		}
		if present && found && stored.manifest == prefix && boundary == entry && hasRetention &&
			retention.LocalRetentionThroughSeq >= prefix.LastOffset && current.Committed >= prefix.LastOffset {
			return quorumlog.AppendOutcomeAlreadyDurable, nil
		}
		return quorumlog.AppendOutcomeConflict, channel.ErrCorruptState
	}
	batch := s.log.db.engine.NewBatch()
	defer batch.Close()
	if err := s.stageImportedPrefix(batch, prefix, entry); err != nil {
		return quorumlog.AppendOutcomeDefinitelyNotWritten, toChannelError(err)
	}
	if err := batch.Commit(true); err != nil {
		return quorumlog.AppendOutcomeUnknown, toChannelError(err)
	}
	s.log.leo.Store(prefix.LastOffset)
	s.log.loaded.Store(true)
	s.log.clearDurableProposalTailLocked()
	return quorumlog.AppendOutcomeDurable, nil
}

// firstDurableProposal reads one ordered manifest instead of materializing the
// proposal catalog. The caller holds the append/checkpoint frontier locks.
func (s *ChannelStore) firstDurableProposal(ctx context.Context) (durableProposalRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return durableProposalRecord{}, false, err
	}
	span := keycodec.NewPrefixSpan(encodeProposalByLastPrefix(s.log.key))
	iter, err := s.log.db.engine.NewIter(engine.Span{Start: span.Start, End: span.End}, engine.IterOptions{})
	if err != nil {
		return durableProposalRecord{}, false, err
	}
	defer iter.Close()
	if !iter.First() {
		return durableProposalRecord{}, false, iter.Error()
	}
	index, ok := decodeProposalByLastKey(s.log.key, iter.Key())
	if !ok {
		return durableProposalRecord{}, false, dberrors.ErrCorruptState
	}
	return loadDurableProposalPairByLast(s.log.db.engine, s.log.key, index)
}

func (s *ChannelStore) stageImportedPrefix(batch *engine.Batch, prefix quorumlog.ProposalManifest, entry quorumlog.EntryIdentity) error {
	checkpoint := Checkpoint{Epoch: prefix.ChannelEpoch, LogStartOffset: prefix.LastOffset, HW: prefix.LastOffset}
	if err := s.log.stageCommitRows(batch, nil, &checkpoint, nil, []durableProposalRecord{{manifest: prefix}}, nil); err != nil {
		return err
	}
	if err := batch.Set(encodeEntryIdentityKey(s.log.key, entry.Index), encodeDurableEntryIdentity(entry)); err != nil {
		return err
	}
	retention := RetentionState{LocalRetentionThroughSeq: prefix.LastOffset, PhysicalRetentionThroughSeq: prefix.LastOffset, RetainedMaxSeq: prefix.LastOffset}
	return batch.Set(encodeRetentionStateKey(s.log.key), encodeRetentionState(retention))
}

// replaceImportedPrefixLocked admits a recordless boundary through the normal
// fenced recovery seam only on an empty durable frontier. The caller owns both
// frontier locks and has already compared Expected against physical state.
func (s *ChannelStore) replaceImportedPrefixLocked(req ReplaceRecoverySuffixRequest) (ReplaceRecoverySuffixResult, error) {
	if len(req.Proposals) != 1 || req.Expected != (DurableFrontier{}) || req.KeepThrough != 0 || len(req.Proposals[0].Records) != 0 {
		return recoveryReplaceError(channel.ErrInvalidArgument), channel.ErrInvalidArgument
	}
	prefix := req.Proposals[0].Manifest
	entry, ok := quorumlog.ImportedPrefixEntry(prefix)
	if !ok || req.Committed != prefix.LastOffset {
		return recoveryReplaceError(channel.ErrInvalidArgument), channel.ErrInvalidArgument
	}
	batch := s.log.db.engine.NewBatch()
	defer batch.Close()
	if err := s.stageImportedPrefix(batch, prefix, entry); err != nil {
		return recoveryReplaceError(err), toChannelError(err)
	}
	if err := batch.Commit(true); err != nil {
		return ReplaceRecoverySuffixResult{Outcome: quorumlog.AppendOutcomeUnknown}, toChannelError(err)
	}
	s.log.leo.Store(prefix.LastOffset)
	s.log.loaded.Store(true)
	s.log.clearDurableProposalTailLocked()
	return ReplaceRecoverySuffixResult{LastOffset: prefix.LastOffset, Outcome: quorumlog.AppendOutcomeDurable}, nil
}
