package migration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/WuKongIM/WuKongIM/internal/contracts/channelmembers"
	"github.com/WuKongIM/WuKongIM/internal/contracts/protocolmeta"
	"github.com/WuKongIM/WuKongIM/pkg/db/message"
	"github.com/WuKongIM/WuKongIM/pkg/db/transfer"
)

// TargetInspector opens only stopped, read-only native storage.
type TargetInspector interface {
	Open(context.Context, TargetPlan, TargetNode) (TargetView, error)
}

// TargetView keeps ownership and native read mechanics outside business
// verification. Message reads must also validate ID and idempotency indexes.
type TargetView interface {
	OwnsMetadata(string) (bool, error)
	OwnsMessages(ChannelIdentity) (bool, error)
	SourceDigest() string
	Metadata(context.Context, string, string, map[string]any) (map[string]any, bool, error)
	Message(context.Context, ChannelIdentity, uint64) (message.Message, bool, error)
	Progress(context.Context, ChannelIdentity) (uint64, uint64, error)
	CheckRuntime(context.Context, ChannelIdentity, uint64) error
	CheckBootstrap(context.Context, uint64) error
	Counts(context.Context) (map[string]uint64, uint64, uint64, error)
	Close() error
}

type VerificationReport struct {
	Status          string            `json:"status"`
	CutoverReady    bool              `json:"cutover_ready"`
	SelectionDigest string            `json:"selection_digest"`
	Digest          string            `json:"digest"`
	Nodes           int               `json:"nodes"`
	Messages        uint64            `json:"verified_message_replicas"`
	Metadata        map[string]uint64 `json:"verified_metadata_replicas"`
}

// VerifyTargets derives expected values directly from selected original rows.
// It never reads the converter's target records or its success counters. Full
// row comparisons, native indexes, replica counts and commit boundaries must
// agree before an offline verification report is returned.
func VerifyTargets(ctx context.Context, plan TargetPlan, selection SourceSelection, w Workspace, decoder BusinessDecoder, inspector TargetInspector) (report VerificationReport, err error) {
	if ctx == nil || selection.Digest == "" || w == nil || decoder == nil || inspector == nil {
		return report, errors.New("verification requires selected original data and native target inspector")
	}
	report.SelectionDigest = selection.Digest
	report.Metadata = map[string]uint64{}
	// Build an independent, bounded directory/count index from original rows.
	b := &captureBatch{ctx: ctx, workspace: w}
	put := func(key string, value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return b.add(transfer.SpoolRow{Key: []byte(key), Value: data})
	}
	err = WalkSelectedSources(ctx, w, func(record SelectedRecord) error {
		id := record.Identity
		if record.Row.Table == "ChannelClusterConfig" || record.Row.Table == "PluginUser" {
			return nil
		}
		if record.Row.Table == "SystemUid" {
			id.Channel = ChannelIdentity{ID: "__wk_internal_system_uids__", Type: uint8(protocolmeta.ChannelTypeSystemUIDRegistry)}
			if err := put("verification/channels/"+channelTuple(id.Channel), id.Channel); err != nil {
				return err
			}
			return put("verification/members/"+channelTuple(id.Channel)+"/"+tuple(id.UID), id.UID)
		}
		facts, err := decoder.DecodeBusiness(record.Row, id)
		if err != nil {
			return err
		}
		if c := facts.Channel; c != nil {
			if err := put("verification/channel-flags/"+channelTuple(c.ChannelIdentity), c); err != nil {
				return err
			}
			return put("verification/channels/"+channelTuple(c.ChannelIdentity), c.ChannelIdentity)
		}
		if member := facts.Member; member != nil {
			channel := member.Channel
			if record.Row.Table == "Allowlist" {
				channel.ID = channelmembers.AllowlistChannelID(channelmembers.ChannelKey{ChannelID: channel.ID, ChannelType: channel.Type})
			}
			if record.Row.Table == "Denylist" {
				channel.ID = channelmembers.DenylistChannelID(channelmembers.ChannelKey{ChannelID: channel.ID, ChannelType: channel.Type})
			}
			if err := put("verification/channels/"+channelTuple(channel), channel); err != nil {
				return err
			}
			return put("verification/members/"+channelTuple(channel)+"/"+tuple(member.UID), member.UID)
		}
		if c := facts.Conversation; c != nil && c.Type == 0 {
			return put("verification/conversations/"+tuple(c.UID, c.Channel.ID, c.Channel.Type), true)
		}
		if m := facts.Message; m != nil {
			return put(fmt.Sprintf("verification/sequences/%s/%020d", channelTuple(id.Channel), m.MessageSeq), m.MessageSeq)
		}
		return nil
	})
	if err != nil {
		return report, err
	}
	if err := b.flush(); err != nil {
		return report, err
	}
	h := sha256.New()
	evidence := json.NewEncoder(h)
	for _, node := range plan.Nodes {
		view, err := inspector.Open(ctx, plan, node)
		if err != nil {
			return report, err
		}
		verified, verifyErr := verifyNode(ctx, node.NodeID, selection, w, decoder, view, evidence)
		closeErr := view.Close()
		if err := errors.Join(verifyErr, closeErr); err != nil {
			return report, fmt.Errorf("verify target node %d: %w", node.NodeID, err)
		}
		report.Nodes++
		report.Messages += verified.Messages
		for table, count := range verified.Metadata {
			report.Metadata[table] += count
		}
	}
	report.Status = "offline_verified"
	report.Digest = hex.EncodeToString(h.Sum(nil))
	return report, nil
}

func verifyNode(ctx context.Context, node uint64, selection SourceSelection, w Workspace, decoder BusinessDecoder, view TargetView, evidence *json.Encoder) (report VerificationReport, err error) {
	if view.SourceDigest() != selection.Digest {
		return report, errors.New("target generation belongs to a different selected source")
	}
	report.Metadata = map[string]uint64{}
	prefix := fmt.Sprintf("verification/expected/%020d/", node)
	b := &captureBatch{ctx: ctx, workspace: w}
	check := func(table, owner string, key, want map[string]any) error {
		owned, err := view.OwnsMetadata(owner)
		if err != nil || !owned {
			return err
		}
		got, found, err := view.Metadata(ctx, table, owner, key)
		if err != nil {
			return err
		}
		wantBytes, err := json.Marshal(want)
		if err != nil {
			return err
		}
		gotBytes, err := json.Marshal(got)
		if err != nil {
			return err
		}
		if !found || !bytes.Equal(wantBytes, gotBytes) {
			return fmt.Errorf("%s row is missing or its business fields differ", table)
		}
		if err := evidence.Encode(struct {
			Node  uint64
			Table string
			Row   map[string]any
		}{node, table, got}); err != nil {
			return err
		}
		return b.add(transfer.SpoolRow{Key: []byte(prefix + table + "/" + tuple(key)), Value: []byte("1")})
	}
	var expectedChannels, sourceMaxMessageID uint64
	err = WalkSelectedSources(ctx, w, func(record SelectedRecord) error {
		id := record.Identity
		channelKey := func() map[string]any {
			return map[string]any{"channel_id": id.Channel.ID, "channel_type": id.Channel.Type}
		}
		switch record.Row.Table {
		case "ChannelClusterConfig", "ChannelInfo":
			return nil
		case "PluginUser":
			return errors.New("plugin compatibility verification is required")
		case "SystemUid":
			key := map[string]any{"channel_id": "__wk_internal_system_uids__", "channel_type": uint8(protocolmeta.ChannelTypeSystemUIDRegistry), "uid": id.UID}
			return check("subscriber", "__wk_internal_system_uids__", key, key)
		}
		facts, err := decoder.DecodeBusiness(record.Row, id)
		if err != nil {
			return err
		}
		switch {
		case facts.User != nil:
			return check("user", id.UID, map[string]any{"uid": id.UID}, map[string]any{"uid": id.UID, "token": "", "device_flag": 0, "device_level": 0})
		case facts.Device != nil:
			d := facts.Device
			return check("device", d.UID, map[string]any{"uid": d.UID, "device_flag": d.Flag}, map[string]any{"uid": d.UID, "device_flag": d.Flag, "device_level": d.Level, "token": d.Token})
		case facts.Member != nil:
			m := facts.Member
			channel := m.Channel.ID
			if record.Row.Table == "Allowlist" {
				channel = channelmembers.AllowlistChannelID(channelmembers.ChannelKey{ChannelID: channel, ChannelType: m.Channel.Type})
			}
			if record.Row.Table == "Denylist" {
				channel = channelmembers.DenylistChannelID(channelmembers.ChannelKey{ChannelID: channel, ChannelType: m.Channel.Type})
			}
			key := map[string]any{"channel_id": channel, "channel_type": m.Channel.Type, "uid": m.UID}
			if err := check("subscriber", channel, key, key); err != nil {
				return err
			}
			if record.Row.Table == "Subscriber" {
				_, hasConversation, err := w.Get(ctx, []byte("verification/conversations/"+tuple(m.UID, m.Channel.ID, m.Channel.Type)))
				if err != nil {
					return err
				}
				if !hasConversation {
					return check("user_channel_membership", m.UID, map[string]any{"uid": m.UID, "channel_id": m.Channel.ID, "channel_type": m.Channel.Type}, map[string]any{"uid": m.UID, "channel_id": m.Channel.ID, "channel_type": m.Channel.Type, "join_seq": 1, "read_seq": 0, "deleted_to_seq": 0, "activated_at": 0, "tombstone": false, "tombstone_at": 0, "source_version": 1, "updated_at": 0})
				}
			}
			return nil
		case facts.Conversation != nil:
			c := facts.Conversation
			if c.Type == 1 {
				return check("user_cmd_channel_membership", c.UID, map[string]any{"uid": c.UID, "command_channel_id": c.Channel.ID, "channel_type": c.Channel.Type}, map[string]any{"uid": c.UID, "command_channel_id": c.Channel.ID, "channel_type": c.Channel.Type, "start_seq": 1, "ack_seq": c.ReadSeq, "tombstone": false, "tombstone_at": 0, "updated_at": c.UpdatedAtNS})
			}
			return check("user_channel_membership", c.UID, map[string]any{"uid": c.UID, "channel_id": c.Channel.ID, "channel_type": c.Channel.Type}, map[string]any{"uid": c.UID, "channel_id": c.Channel.ID, "channel_type": c.Channel.Type, "join_seq": 1, "read_seq": c.ReadSeq, "deleted_to_seq": c.DeletedToSeq, "activated_at": 0, "tombstone": false, "tombstone_at": 0, "source_version": 1, "updated_at": c.UpdatedAtNS})
		case facts.Message != nil:
			sourceMaxMessageID = max(sourceMaxMessageID, facts.Message.MessageID)
			owned, err := view.OwnsMessages(id.Channel)
			if err != nil || !owned {
				return err
			}
			m := facts.Message
			got, found, err := view.Message(ctx, id.Channel, m.MessageSeq)
			if err != nil {
				return err
			}
			p := got.Protocol
			if !found || got.MessageID != m.MessageID || got.MessageSeq != m.MessageSeq || got.ChannelID != m.ChannelID || got.ChannelType != m.ChannelType || got.ClientMsgNo != m.ClientMsgNo || got.FromUID != m.FromUID || got.ServerTimestampMS != m.ServerTimestampMS || !bytes.Equal(got.Payload, m.Payload) || got.Setting != uint8(m.Setting) || got.SyncOnce != m.Framer.SyncOnce || p.Timestamp != m.Timestamp || p.Expire != m.Expire || p.ClientSeq != m.ClientSeq || p.MsgKey != m.MsgKey || p.StreamNo != m.StreamNo || p.StreamID != m.StreamID || p.StreamFlag != uint8(m.StreamFlag) || p.Topic != m.Topic || ((p.FramerFlags&1) != 0) != m.Framer.NoPersist || ((p.FramerFlags&2) != 0) != m.Framer.RedDot || ((p.FramerFlags&8) != 0) != m.Framer.DUP || ((p.FramerFlags&16) != 0) != m.Framer.HasServerVersion || ((p.FramerFlags&32) != 0) != m.Framer.End {
				return fmt.Errorf("message %q/%d/%d differs from original source", id.Channel.ID, id.Channel.Type, m.MessageSeq)
			}
			report.Messages++
			return evidence.Encode(struct {
				Node    uint64
				Message message.Message
			}{node, got})
		case facts.Tail != nil:
			var first uint64
			if err := w.Walk(ctx, []byte("verification/sequences/"+channelTuple(id.Channel)+"/"), func(row transfer.SpoolRow) error {
				if first != 0 {
					return nil
				}
				return json.Unmarshal(row.Value, &first)
			}); err != nil {
				return err
			}
			prefixThrough := facts.Tail.LastSeq
			if first != 0 {
				prefixThrough = first - 1
			}
			owned, err := view.OwnsMetadata(id.Channel.ID)
			if err != nil {
				return err
			}
			if owned {
				if err := view.CheckRuntime(ctx, id.Channel, prefixThrough); err != nil {
					return err
				}
				if err := b.add(transfer.SpoolRow{Key: []byte(prefix + "channel_runtime_meta/" + tuple(channelKey())), Value: []byte("1")}); err != nil {
					return err
				}
			}
			owned, err = view.OwnsMessages(id.Channel)
			if err != nil || !owned {
				return err
			}
			leo, hw, err := view.Progress(ctx, id.Channel)
			if err != nil {
				return err
			}
			if leo != facts.Tail.LastSeq || hw != facts.Tail.LastSeq {
				return errors.New("message log tail or committed boundary differs from original source")
			}
			if leo > 0 {
				expectedChannels++
			}
			return nil
		case facts.EventCursor != nil:
			c := facts.EventCursor
			key := channelKey()
			key["client_msg_no"] = c.ClientMsgNo
			return check("message_event_cursor", id.Channel.ID, key, map[string]any{"channel_id": c.ChannelID, "channel_type": c.ChannelType, "client_msg_no": c.ClientMsgNo, "last_msg_event_seq": c.LastMsgEventSeq, "updated_at": 0})
		case facts.EventState != nil:
			s := facts.EventState
			key := channelKey()
			key["client_msg_no"] = s.ClientMsgNo
			key["event_key"] = s.EventKey
			if err := check("message_event_state", id.Channel.ID, key, map[string]any{"channel_id": s.ChannelID, "channel_type": s.ChannelType, "client_msg_no": s.ClientMsgNo, "event_key": s.EventKey, "status": s.Status, "last_msg_event_seq": s.LastMsgEventSeq, "last_event_id": s.LastEventID, "last_event_type": s.LastEventType, "last_visibility": s.LastVisibility, "last_occurred_at": s.LastOccurredAt, "snapshot_payload": s.SnapshotPayload, "end_reason": s.EndReason, "error": s.Error, "updated_at": 0}); err != nil {
				return err
			}
			return check("message_event_applied", id.Channel.ID, map[string]any{"channel_id": s.ChannelID, "channel_type": s.ChannelType, "client_msg_no": s.ClientMsgNo, "event_id": s.LastEventID}, map[string]any{"channel_id": s.ChannelID, "channel_type": s.ChannelType, "client_msg_no": s.ClientMsgNo, "event_id": s.LastEventID, "event_key": s.EventKey, "msg_event_seq": s.LastMsgEventSeq, "status": s.Status, "updated_at": 0})
		default:
			return fmt.Errorf("verification is unavailable for selected %s", record.Row.Table)
		}
	})
	if err != nil {
		return report, err
	}
	err = w.Walk(ctx, []byte("verification/channels/"), func(row transfer.SpoolRow) error {
		var id ChannelIdentity
		if err := json.Unmarshal(row.Value, &id); err != nil {
			return err
		}
		flags := SourceChannel{ChannelIdentity: id}
		data, exists, err := w.Get(ctx, []byte("verification/channel-flags/"+channelTuple(id)))
		if err != nil {
			return err
		}
		if exists {
			if err := json.Unmarshal(data, &flags); err != nil {
				return err
			}
		}
		var count uint64
		if err := w.Walk(ctx, []byte("verification/members/"+channelTuple(id)+"/"), func(transfer.SpoolRow) error { count++; return nil }); err != nil {
			return err
		}
		key := map[string]any{"channel_id": id.ID, "channel_type": id.Type}
		return check("channel", id.ID, key, map[string]any{"channel_id": id.ID, "channel_type": id.Type, "ban": boolInt(flags.Ban), "disband": boolInt(flags.Disband), "send_ban": boolInt(flags.SendBan), "allow_stranger": boolInt(flags.AllowStranger), "large": boolInt(flags.Large), "subscriber_mutation_version": 1, "subscriber_count": count, "directory_projection_state": 0, "directory_projection_generation": 0})
	})
	if err != nil {
		return report, err
	}
	if err := b.flush(); err != nil {
		return report, err
	}
	if err := w.Walk(ctx, []byte(prefix), func(row transfer.SpoolRow) error {
		relative := string(row.Key[len(prefix):])
		for i := range relative {
			if relative[i] == '/' {
				report.Metadata[relative[:i]]++
				return nil
			}
		}
		return errors.New("invalid verification ledger key")
	}); err != nil {
		return report, err
	}
	actual, messages, channels, err := view.Counts(ctx)
	if err != nil {
		return report, err
	}
	if messages != report.Messages || channels != expectedChannels {
		return report, errors.New("unexpected or missing target message rows or channels")
	}
	for table, count := range actual {
		if report.Metadata[table] != count {
			return report, fmt.Errorf("%s target row count differs from independent source count", table)
		}
	}
	for table, count := range report.Metadata {
		if actual[table] != count {
			return report, fmt.Errorf("%s target row count differs from independent source count", table)
		}
	}
	if err := view.CheckBootstrap(ctx, sourceMaxMessageID); err != nil {
		return report, err
	}
	return report, nil
}
