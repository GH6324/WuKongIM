package migration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"slices"

	"github.com/WuKongIM/WuKongIM/pkg/db/transfer"
)

type sourceGroup struct {
	Leader   uint64   `json:"leader"`
	Replicas []uint64 `json:"replicas"`
}

// sourceCandidate references the immutable capture instead of duplicating
// message payloads for every join, comparison and selected record.
type sourceCandidate struct {
	NodeID     uint64         `json:"node_id"`
	SourceKey  []byte         `json:"source_key"`
	Table      string         `json:"table"`
	Kind       Kind           `json:"kind"`
	Identity   RecordIdentity `json:"identity"`
	LogicalKey string         `json:"logical_key"`
	Digest     string         `json:"digest"`
	Group      sourceGroup    `json:"group"`
}

// SelectSources compares every configured replica to the persisted authority.
// No union or maximum sequence can create a business record. Source indexes,
// runtime metadata and obsolete management rows remain in the bound capture.
func SelectSources(ctx context.Context, capture SourceCapture, catalog SourceCatalog, workspace Workspace, decoder RecordDecoder) (selection SourceSelection, err error) {
	if ctx == nil || workspace == nil || decoder == nil || capture.Digest == "" || catalog.Digest == "" {
		return selection, errors.New("source selection requires completed capture and catalog")
	}
	if err := validateCapturedAuthority(capture.Nodes); err != nil {
		return selection, err
	}
	selection.Tables = map[string]uint64{}
	selection.Preserved = map[string]uint64{}
	slots := make(map[uint32]SourceSlot, len(capture.Nodes[0].Config.Slots))
	knownNodes := make(map[uint64]bool, len(capture.Nodes))
	for _, slot := range capture.Nodes[0].Config.Slots {
		slots[slot.ID] = slot
	}
	for _, node := range capture.Nodes {
		knownNodes[node.NodeID] = true
	}
	batch := &captureBatch{ctx: ctx, workspace: workspace}
	for _, phase := range []string{"metadata", "messages"} {
		for _, node := range capture.Nodes {
			err := walkSourceRows(ctx, workspace, node.NodeID, func(row Row) error {
				category, reason, err := sourceCategory(row)
				if err != nil {
					return err
				}
				if reason != "" {
					if phase == "metadata" {
						if row.Table == "Plugin" && row.Kind == Primary {
							description, err := decoder.Describe(row, RecordIdentity{})
							if err != nil {
								return err
							}
							// Persisted status does not prove that an original global
							// hook was inactive. No v3 plugin mapping is implemented.
							if description.Plugin == nil || len(description.Plugin.Methods) != 0 || description.Plugin.HasConfig {
								return errors.New("source plugin business methods/config require a verified v3 compatibility mapping")
							}
						}
						selection.Preserved[reason]++
					}
					return nil
				}
				if category != phase {
					return nil
				}
				id, err := decoder.Identify(row)
				if err != nil {
					return err
				}
				id, err = resolveIdentity(ctx, workspace, id)
				if err != nil {
					return fmt.Errorf("%s identity: %w", row.Table, err)
				}
				description, err := decoder.Describe(row, id)
				if err != nil {
					return err
				}
				var group sourceGroup
				if phase == "metadata" {
					owner := id.Channel.ID
					switch row.Table {
					case "User", "Device", "Conversation":
						owner = id.UID
					}
					slotID := crc32.ChecksumIEEE([]byte(owner)) % capture.Nodes[0].Config.SlotCount
					if row.Table == "SystemUid" || row.Table == "PluginUser" {
						slotID = 0
					}
					slot, ok := slots[slotID]
					if !ok {
						return errors.New("missing source owner Slot")
					}
					group = sourceGroup{Leader: slot.Leader, Replicas: slot.Replicas}
				} else {
					group, err = messageGroup(ctx, workspace, decoder, id.Channel, knownNodes)
					if err != nil {
						return err
					}
				}
				if !slices.Contains(group.Replicas, node.NodeID) {
					selection.Preserved["outside_current_replica_group"]++
					return nil
				}
				sum := sha256.Sum256(description.Comparable)
				candidate := sourceCandidate{
					NodeID: node.NodeID, SourceKey: sourceRowKey(node.NodeID, row), Table: row.Table, Kind: row.Kind,
					Identity: id, LogicalKey: description.Key, Digest: hex.EncodeToString(sum[:]), Group: group,
				}
				value, err := json.Marshal(candidate)
				if err != nil {
					return err
				}
				return batch.add(transfer.SpoolRow{Key: candidateKey(phase, node.NodeID, row.Table, description.Key), Value: value})
			})
			if err != nil {
				return selection, fmt.Errorf("source node %d %s selection: %w", node.NodeID, phase, err)
			}
		}
		if err := batch.flush(); err != nil {
			return selection, err
		}
		if err := compareCandidates(ctx, workspace, phase, batch); err != nil {
			return selection, err
		}
		if err := batch.flush(); err != nil {
			return selection, err
		}
	}
	if err := recoverSourceConversations(ctx, capture, workspace, decoder, batch, &selection); err != nil {
		return selection, err
	}
	if err := batch.flush(); err != nil {
		return selection, err
	}
	h := sha256.New()
	enc := json.NewEncoder(h)
	err = WalkSelectedSources(ctx, workspace, func(record SelectedRecord) error {
		if record.Row.Kind == Primary {
			selection.Tables[record.Row.Table]++
		}
		return enc.Encode(record)
	})
	if err != nil {
		return selection, err
	}
	selection.Digest = hex.EncodeToString(h.Sum(nil))
	return selection, nil
}

func sourceCategory(row Row) (category, reason string, err error) {
	if row.Kind == Index || row.Kind == SecondaryIndex {
		return "", "original_indexes", nil
	}
	switch row.Table {
	case "MessageNotifyQueue":
		return "", "", errors.New("source retains message notification queue records; do not replay automatically")
	case "Total", "Tester", "Plugin":
		return "", "old_management_data", nil
	case "LeaderTermSequence", "ChannelCommon":
		return "", "source_replication_metadata", nil
	case "ConversationLocalUser":
		return "", "derived_conversation_directory", nil
	case "SubscriberChannelRelation":
		return "", "obsolete_subscriber_directory", nil
	case "PendingConversation":
		return "", "", nil
	case "Message":
		if row.Kind == Primary || row.Kind == Other {
			return "messages", "", nil
		}
	case "User", "Device", "ChannelInfo", "Subscriber", "Denylist", "Allowlist", "SystemUid", "PluginUser", "Conversation", "ChannelClusterConfig", "MessageEventState":
		if row.Kind == Primary {
			return "metadata", "", nil
		}
	case "MessageEventSeq":
		if row.Kind == Other {
			return "metadata", "", nil
		}
	}
	return "", "", fmt.Errorf("source business table %s kind %d has no supported interpretation", row.Table, row.Kind)
}

func sourceRowKey(nodeID uint64, row Row) []byte {
	return []byte(fmt.Sprintf("source/%020d/rows/%04d/%x", nodeID, row.Shard, row.Key))
}
func candidateKey(phase string, nodeID uint64, table, key string) []byte {
	return []byte(fmt.Sprintf("candidate/%s/%020d/%s/%s", phase, nodeID, table, key))
}
func selectedKey(table, key string) []byte { return []byte("selected/" + table + "/" + key) }

func compareCandidates(ctx context.Context, workspace Workspace, phase string, batch *captureBatch) error {
	return workspace.Walk(ctx, []byte("candidate/"+phase+"/"), func(record transfer.SpoolRow) error {
		var row sourceCandidate
		if err := json.Unmarshal(record.Value, &row); err != nil {
			return err
		}
		if !bytes.Equal(record.Key, candidateKey(phase, row.NodeID, row.Table, row.LogicalKey)) {
			return errors.New("source comparison key mismatch")
		}
		ids := []uint64{row.Group.Leader}
		if row.NodeID == row.Group.Leader {
			ids = row.Group.Replicas
		}
		for _, id := range ids {
			data, ok, err := workspace.Get(ctx, candidateKey(phase, id, row.Table, row.LogicalKey))
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("source %s record is missing on configured replica node %d", row.Table, id)
			}
			var other sourceCandidate
			if err := json.Unmarshal(data, &other); err != nil {
				return err
			}
			if other.Digest != row.Digest {
				return fmt.Errorf("source %s record conflicts between replica nodes %d and %d", row.Table, row.NodeID, id)
			}
		}
		if row.NodeID != row.Group.Leader {
			return nil
		}
		return batch.add(transfer.SpoolRow{Key: selectedKey(row.Table, row.LogicalKey), Value: record.Value})
	})
}

func resolveIdentity(ctx context.Context, workspace Workspace, id RecordIdentity) (RecordIdentity, error) {
	get := func(key string) ([]byte, error) {
		data, ok, err := workspace.Get(ctx, []byte(key))
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errors.New("unresolved source hash reference")
		}
		return data, nil
	}
	if id.UID == "" && id.UIDHash != 0 {
		data, err := get(fmt.Sprintf("catalog/uid/%016x", id.UIDHash))
		if err != nil {
			return id, err
		}
		id.UID = string(data)
	}
	if id.Channel.ID == "" && (id.ChannelHash != 0 || id.EventChannelHash != 0) {
		key := fmt.Sprintf("catalog/channel/%016x", id.ChannelHash)
		if id.EventChannelHash != 0 {
			key = fmt.Sprintf("catalog/event-channel/%016x", id.EventChannelHash)
		}
		data, err := get(key)
		if err != nil {
			return id, err
		}
		if err := json.Unmarshal(data, &id.Channel); err != nil {
			return id, err
		}
	}
	if id.ClientMsgNo == "" && id.ClientMsgHash != 0 {
		data, err := get(fmt.Sprintf("catalog/event-message/%016x/%016x", id.EventChannelHash, id.ClientMsgHash))
		if err != nil {
			return id, err
		}
		id.ClientMsgNo = string(data)
	}
	return id, nil
}

func messageGroup(ctx context.Context, workspace Workspace, decoder RecordDecoder, channel ChannelIdentity, nodes map[uint64]bool) (group sourceGroup, err error) {
	data, err := json.Marshal([]any{channel.ID, channel.Type})
	if err != nil {
		return group, err
	}
	encoded, ok, err := workspace.Get(ctx, selectedKey("ChannelClusterConfig", base64.RawURLEncoding.EncodeToString(data)))
	if err != nil {
		return group, err
	}
	if !ok {
		return group, errors.New("message channel lacks authoritative source configuration")
	}
	record, err := loadSelectedRecord(ctx, workspace, encoded)
	if err != nil {
		return group, err
	}
	description, err := decoder.Describe(record.Row, record.Identity)
	if err != nil {
		return group, err
	}
	if description.Authority == nil {
		return group, errors.New("source channel lacks decoded authority")
	}
	for _, node := range description.Authority.Replicas {
		if !nodes[node] {
			return group, fmt.Errorf("message channel replica node %d is missing", node)
		}
	}
	return sourceGroup{Leader: description.Authority.Leader, Replicas: description.Authority.Replicas}, nil
}

func walkSelected(ctx context.Context, workspace Workspace, prefix []byte, visit func(SelectedRecord) error) error {
	if ctx == nil || workspace == nil || visit == nil {
		return errors.New("selected source walk requires context, workspace and visitor")
	}
	return workspace.Walk(ctx, prefix, func(row transfer.SpoolRow) error {
		record, err := loadSelectedRecord(ctx, workspace, row.Value)
		if err != nil {
			return err
		}
		if !bytes.Equal(row.Key, selectedKey(record.Row.Table, record.LogicalKey)) {
			return errors.New("selected source key mismatch")
		}
		return visit(record)
	})
}
func loadSelectedRecord(ctx context.Context, workspace Workspace, data []byte) (record SelectedRecord, err error) {
	var reference sourceCandidate
	if err := json.Unmarshal(data, &reference); err != nil {
		return record, err
	}
	value, ok, err := workspace.Get(ctx, reference.SourceKey)
	if err != nil {
		return record, err
	}
	if !ok {
		return record, errors.New("selected source record is absent from its capture")
	}
	if err := json.Unmarshal(value, &record.Row); err != nil {
		return record, err
	}
	if !bytes.Equal(reference.SourceKey, sourceRowKey(reference.NodeID, record.Row)) || record.Row.Table != reference.Table || record.Row.Kind != reference.Kind {
		return record, errors.New("selected source reference mismatch")
	}
	record.NodeID, record.Identity, record.LogicalKey = reference.NodeID, reference.Identity, reference.LogicalKey
	return record, nil
}
