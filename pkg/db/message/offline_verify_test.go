package message

import (
	"context"
	"testing"

	"github.com/WuKongIM/WuKongIM/pkg/db/internal/dberrors"
	channel "github.com/WuKongIM/WuKongIM/pkg/db/message/channelcompat"
	"github.com/WuKongIM/WuKongIM/pkg/quorumlog"
	"github.com/stretchr/testify/require"
)

func TestOfflineImportedLogVerificationRejectsCorruptInteriorIdentity(t *testing.T) {
	ctx := context.Background()
	engine := openCompatEngine(t)
	id := channel.ChannelID{ID: "offline-import", Type: 1}
	store := mustForChannel(t, engine, "offline-import", id)
	defer store.Close()
	records := []channel.Record{
		compatExactTestRecord(t, 1, 901, id.ID, "first"),
		compatExactTestRecord(t, 1, 902, id.ID, "second"),
	}
	manifest := sealCompatProposalManifest(t, DurableProposalManifest{
		Version: quorumlog.ProposalManifestVersion, ChannelEpoch: 1, LeaderTerm: 1, FenceVersion: 1,
		CommandID: quorumlog.CommandID{1}, LastOffset: 2,
	}, records)
	result := StoreAppendBatch(ctx, []AppendBatchItem{{Store: store, Records: records, ExactBaseOffset: true, Proposal: manifest, Committed: 2}})
	require.Len(t, result, 1)
	require.NoError(t, result[0].Err)
	require.NoError(t, store.log.VerifyOfflineImportedLog(ctx, 1<<20))

	// Counts, payloads and the final frontier remain unchanged. Only a complete
	// proposal walk can detect this non-tail identity's wrong channel epoch.
	entry, found, err := loadDurableEntryIdentityFrom(engine.engine, store.log.key, 1)
	require.NoError(t, err)
	require.True(t, found)
	entry.ChannelEpoch++
	setPhysicalTestValue(t, engine, encodeEntryIdentityKey(store.log.key, 1), encodeDurableEntryIdentity(entry))
	require.ErrorIs(t, store.log.VerifyOfflineImportedLog(ctx, 1<<20), dberrors.ErrCorruptState)
}
