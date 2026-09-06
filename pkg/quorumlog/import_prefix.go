package quorumlog

import (
	"crypto/sha256"
	"encoding/binary"
)

// ImportedPrefixVersion identifies a recordless migration boundary. It does
// not certify historical v2 proposals, messages or acknowledgements. Ordinary
// append APIs cannot create one; an empty-target importer must install it.
const ImportedPrefixVersion uint16 = 3

// NewImportedPrefix binds a retained source prefix to the exact source digest
// and Channel key. The new target starts with its own epoch/term/fence of one.
// The returned identity is a boundary, never a client-visible message record.
func NewImportedPrefix(channelKey string, sourceDigest [32]byte, through uint64) (ProposalManifest, EntryIdentity, bool) {
	if channelKey == "" || sourceDigest == ([32]byte{}) || through == 0 || through == ^uint64(0) {
		return ProposalManifest{}, EntryIdentity{}, false
	}
	h := sha256.New()
	_, _ = h.Write([]byte("wukongim/imported-prefix-source/v1\x00"))
	_, _ = h.Write(sourceDigest[:])
	_, _ = h.Write([]byte(channelKey))
	var command CommandID
	copy(command[:], h.Sum(nil))
	m := ProposalManifest{Version: ImportedPrefixVersion, ChannelEpoch: 1, LeaderTerm: 1, FenceVersion: 1, CommandID: command, LastOffset: through}
	m.Digest = importedPrefixDigest(m)
	e, ok := ImportedPrefixEntry(m)
	return m, e, ok
}

// ImportedPrefixEntry validates the complete recordless boundary identity.
func ImportedPrefixEntry(m ProposalManifest) (EntryIdentity, bool) {
	if m.Version != ImportedPrefixVersion || m.ChannelEpoch == 0 || m.LeaderTerm == 0 || m.FenceVersion == 0 ||
		m.CommandID == (CommandID{}) || m.BaseOffset != 0 || m.LastOffset == 0 || m.LastOffset == ^uint64(0) ||
		m.PreviousIndex != 0 || m.PreviousTerm != 0 || m.PreviousDigest != (EntryDigest{}) || m.Digest != importedPrefixDigest(m) {
		return EntryIdentity{}, false
	}
	return EntryIdentity{Version: ImportedPrefixVersion, ChannelEpoch: m.ChannelEpoch, LeaderTerm: m.LeaderTerm,
		FenceVersion: m.FenceVersion, Index: m.LastOffset, CommandID: m.CommandID, Digest: m.Digest}, true
}

func importedPrefixDigest(m ProposalManifest) EntryDigest {
	b := append([]byte("wukongim/imported-prefix-boundary/v1\x00"), m.CommandID[:]...)
	for _, n := range []uint64{m.ChannelEpoch, m.LeaderTerm, m.FenceVersion, m.LastOffset} {
		b = binary.BigEndian.AppendUint64(b, n)
	}
	return EntryDigest(sha256.Sum256(b))
}

// ValidEntryIdentity checks durable structure, including recordless imported
// boundaries. Verifying message content additionally requires VerifyEntry.
func ValidEntryIdentity(e EntryIdentity) bool {
	if e.Version == ImportedPrefixVersion {
		m := ProposalManifest{Version: e.Version, ChannelEpoch: e.ChannelEpoch, LeaderTerm: e.LeaderTerm,
			FenceVersion: e.FenceVersion, LastOffset: e.Index, CommandID: e.CommandID, Digest: e.Digest,
			PreviousTerm: e.PreviousTerm, PreviousIndex: e.PreviousIndex, PreviousDigest: e.PreviousDigest}
		_, ok := ImportedPrefixEntry(m)
		return ok
	}
	if !SupportedProposalVersion(e.Version) || e.ChannelEpoch == 0 || e.LeaderTerm == 0 || e.FenceVersion == 0 ||
		e.Index == 0 || e.PreviousIndex == ^uint64(0) || e.PreviousIndex+1 != e.Index || e.CommandID == (CommandID{}) || e.Digest == (EntryDigest{}) {
		return false
	}
	if e.PreviousIndex == 0 {
		return e.PreviousTerm == 0 && e.PreviousDigest == (EntryDigest{})
	}
	return e.PreviousTerm != 0 && e.PreviousDigest != (EntryDigest{})
}
