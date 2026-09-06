package migration

import "context"

// ChannelAuthority is decoded source ownership, never target cluster state.
type ChannelAuthority struct {
	Channel  ChannelIdentity `json:"channel"`
	Leader   uint64          `json:"leader"`
	Replicas []uint64        `json:"replicas"`
	Term     uint32          `json:"term"`
	Version  uint64          `json:"version"`
}

// RecordDescription gives a logical comparison key and complete source value.
// Comparable excludes physical row placement and proven replica-local metadata;
// the original row is always retained separately for lossless provenance.
type RecordDescription struct {
	Key        string
	Comparable []byte
	Authority  *ChannelAuthority
	Plugin     *PluginDescription
}

// PluginDescription separates obsolete registration text from methods and
// configuration that can affect business behavior without any user binding.
type PluginDescription struct {
	Methods   []string
	HasConfig bool
}

type RecordDecoder interface {
	IdentityDecoder
	Describe(Row, RecordIdentity) (RecordDescription, error)
}

// SelectedRecord preserves the exact authoritative source row. A pending
// conversation retains its recovery intent instead of inventing an old row ID.
type SelectedRecord struct {
	NodeID     uint64         `json:"node_id"`
	Row        Row            `json:"row"`
	Identity   RecordIdentity `json:"identity"`
	LogicalKey string         `json:"logical_key"`
}

type SourceSelection struct {
	Tables    map[string]uint64 `json:"selected_primary_rows_by_table"`
	Preserved map[string]uint64 `json:"preserved_physical_rows_by_reason"`
	Digest    string            `json:"digest"`
}

// WalkSelectedSources streams only the source records selected by the workflow.
// Callers must require a successful SelectSources result before using its data.
func WalkSelectedSources(ctx context.Context, workspace Workspace, visit func(SelectedRecord) error) error {
	return walkSelected(ctx, workspace, []byte("selected/"), visit)
}
