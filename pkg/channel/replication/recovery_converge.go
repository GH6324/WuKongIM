package replication

import (
	"context"
	"sort"

	ch "github.com/WuKongIM/WuKongIM/pkg/channel"
)

// convergeRecoveryPrefix copies only missing exact proposals between compatible
// voter logs. A donor is not a commit certificate: Install must subsequently
// obtain a fresh identical-prefix quorum and its current-authority barrier.
// No existing entry, proposal identity, or committed checkpoint is replaced.
func (l *quorumLog) convergeRecoveryPrefix(ctx context.Context, authority Authority) error {
	ctx, cancel := context.WithTimeout(ctx, l.cfg.RecoveryTimeout)
	defer cancel()
	request := recoveryProbeRequest{ChannelKey: authority.Key, ChannelID: authority.ChannelID, Leader: authority.Leader, Voters: authority.Voters, Quorum: authority.WriteQuorum, Timeout: l.cfg.RecoveryTimeout}
	reports, err := collectRecoveryProbeRound(ctx, request, nil, l.cfg.Recovery)
	if err != nil {
		return err
	}
	if len(reports) < authority.WriteQuorum {
		return errRecoveryQuorumUnavailable
	}
	donor := reports[0]
	localPresent := false
	indexes := make([]uint64, 0, len(reports))
	seen := make(map[uint64]bool, len(reports))
	for _, report := range reports {
		localPresent = localPresent || report.Voter == authority.Leader
		state := report.Result.State
		if state.LEO > donor.Result.State.LEO {
			donor = report
		}
		if state.LEO > 0 {
			tail := state.TailIdentity
			if compareAuthorityID(AuthorityID{ChannelEpoch: tail.ChannelEpoch, LeaderTerm: tail.LeaderTerm, FenceVersion: tail.FenceVersion}, authority.ID) > 0 {
				return ch.ErrStaleMeta
			}
			if !seen[state.LEO] {
				indexes = append(indexes, state.LEO)
				seen[state.LEO] = true
			}
		}
	}
	if !localPresent {
		return errRecoveryQuorumUnavailable
	}
	if donor.Result.State.LEO == 0 {
		return errRecoveryProbeIncomplete
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i] < indexes[j] })
	// Every observed tail must be an exact ancestor of the candidate, including
	// non-quorum tails. A higher LEO alone never authorizes choosing a branch.
	request.Voters = []ch.NodeID{donor.Voter}
	proof, err := collectRecoveryProbeRound(ctx, request, indexes, l.cfg.Recovery)
	if err != nil {
		return err
	}
	if len(proof) != 1 || proof[0].Result.State != donor.Result.State {
		return errRecoveryProbeIncomplete
	}
	identities := make(map[uint64]ch.EntryIdentity, len(indexes))
	for _, entry := range proof[0].Result.Entries {
		if !entry.Present {
			return ch.ErrLogConflict
		}
		identities[entry.Index] = entry.Identity
	}
	for _, report := range reports {
		if report.Result.State.LEO > 0 && identities[report.Result.State.LEO] != report.Result.State.TailIdentity {
			return ch.ErrLogConflict
		}
	}
	for _, report := range reports {
		if report.Result.State.LEO == donor.Result.State.LEO {
			continue
		}
		if err := l.copyRecoverySuffix(ctx, authority, donor, report); err != nil {
			return err
		}
	}
	return nil
}

// copyRecoverySuffix reuses the bounded exact append path with Committed=0.
// A concurrent append can be an exact replay or a conflict, never an overwrite.
// Successfully copied pages remain durable progress after timeout or restart.
func (l *quorumLog) copyRecoverySuffix(ctx context.Context, authority Authority, donor, target recoveryProbeReport) error {
	previous := target.Result.State.TailIdentity
	for from := target.Result.State.LEO + 1; previous.Index < donor.Result.State.LEO; {
		through := donor.Result.State.LEO
		if through-from >= maxRecoveryProbeIndexes {
			through = from + maxRecoveryProbeIndexes - 1
		}
		query := recoveryFetchQuery{ChannelKey: authority.Key, ChannelID: authority.ChannelID, Leader: authority.Leader, Donor: donor.Voter, Expected: donor.Result.State, From: from, Through: through, Previous: previous, MaxBytes: l.cfg.RecoveryPageBytes}
		completion := make(chan recoveryFetchCompletion, 1)
		if err := l.cfg.Recovery.submitRecoveryFetch(ctx, query, func(result FetchResult, err error) { completion <- recoveryFetchCompletion{result: result, err: err} }); err != nil {
			return err
		}
		var page FetchResult
		select {
		case <-ctx.Done():
			return ctx.Err()
		case done := <-completion:
			if done.err != nil {
				return done.err
			}
			page = done.result
		}
		fetch := FetchRequest{ChannelKey: query.ChannelKey, ChannelID: query.ChannelID, Leader: query.Leader, Follower: query.Donor, Expected: query.Expected, From: from, Through: through, Previous: previous, MaxBytes: query.MaxBytes}
		if !validPeerFetchResult(fetch, page) {
			return ch.ErrLogConflict
		}
		for _, proposal := range page.Proposals {
			durable := durableProposal{first: proposal.Manifest.BaseOffset + 1, last: proposal.Manifest.LastOffset, channelKey: authority.Key, channelID: authority.ChannelID, leader: authority.Leader, manifest: proposal.Manifest, records: proposal.Records}
			done := make(chan durabilityCompletion, 1)
			finish := func(result durabilityCompletion) { done <- result }
			var err error
			if target.Voter == authority.Leader {
				err = l.cfg.Durability.submitLocal(ctx, durable, finish)
			} else {
				err = l.cfg.Durability.submitReplica(ctx, target.Voter, durable, finish)
			}
			if err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case result := <-done:
				if result.err != nil {
					return result.err
				}
				if !result.outcome.Durable() {
					return ch.ErrLogConflict
				}
			}
			_, entries, ok := ch.SealProposalManifest(proposal.Manifest, proposal.Records)
			if !ok || len(entries) == 0 {
				return ch.ErrLogConflict
			}
			previous = entries[len(entries)-1]
			from = proposal.Manifest.LastOffset + 1
		}
	}
	return nil
}
