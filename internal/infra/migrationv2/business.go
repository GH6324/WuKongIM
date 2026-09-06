package migrationv2

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/WuKongIM/WuKongIM/internal/usecase/migration"
	"github.com/WuKongIM/WuKongIM/pkg/db/meta"
)

// DecodeBusiness preserves source field values without invoking target
// reducers, assigning target owners, or inventing missing source timestamps.
func (Reader) DecodeBusiness(row Row, id RecordIdentity) (facts migration.BusinessFacts, err error) {
	created, err := optionalTime(row, "CreatedAt")
	if err != nil {
		return facts, err
	}
	updated, err := optionalTime(row, "UpdatedAt")
	if err != nil {
		return facts, err
	}
	switch row.Table {
	case "Subscriber", "Allowlist", "Denylist":
		if id.UID == "" || id.Channel.ID == "" || channelHash(id.Channel.ID, id.Channel.Type) != row.Owner {
			return facts, errors.New("unresolved original member channel")
		}
		facts.Member = &migration.SourceMember{SourceID: row.ID, UID: id.UID, Channel: id.Channel, CreatedAtNS: created, UpdatedAtNS: updated}
	case "Message":
		if row.Kind == Other {
			if len(row.Key) != 12 || len(row.Value) != 16 || id.Channel.ID == "" || channelHash(id.Channel.ID, id.Channel.Type) != binary.BigEndian.Uint64(row.Key[4:12]) {
				return facts, errors.New("invalid or unresolved original message tail")
			}
			facts.Tail = &migration.SourceMessageTail{Channel: id.Channel, LastSeq: binary.BigEndian.Uint64(row.Value[:8]), LastAppendNS: int64(binary.BigEndian.Uint64(row.Value[8:]))}
			break
		}
		message, _, err := DecodeMessage(row)
		if err != nil {
			return facts, err
		}
		facts.Message = &message
	case "User":
		if id.UID == "" {
			return facts, errors.New("unresolved original account identity")
		}
		facts.User = &migration.SourceUser{SourceID: row.ID, UID: id.UID, PluginNo: string(row.Fields["PluginNo"]), CreatedAtNS: created, UpdatedAtNS: updated}
	case "ChannelInfo":
		if id.Channel.ID == "" {
			return facts, errors.New("unresolved original channel identity")
		}
		var decodeErr error
		flag := func(name string) bool {
			v := row.Fields[name]
			if len(v) == 0 {
				return false
			}
			if len(v) != 1 || v[0] > 1 {
				decodeErr = fmt.Errorf("invalid original channel flag %s", name)
				return false
			}
			return v[0] == 1
		}
		count := func(name string) uint32 {
			v := row.Fields[name]
			if len(v) == 0 {
				return 0
			}
			if len(v) != 4 {
				decodeErr = fmt.Errorf("invalid original channel count %s", name)
				return 0
			}
			return binary.BigEndian.Uint32(v)
		}
		facts.Channel = &migration.SourceChannel{ChannelIdentity: id.Channel, SourceID: row.ID, Ban: flag("Ban"), Disband: flag("Disband"), SendBan: flag("SendBan"), AllowStranger: flag("AllowStranger"), Large: flag("Large"), SubscriberCount: count("SubscriberCount"), AllowlistCount: count("AllowlistCount"), DenylistCount: count("DenylistCount"), CreatedAtNS: created, UpdatedAtNS: updated}
		if decodeErr != nil {
			return facts, decodeErr
		}
	case "Conversation", "PendingConversation":
		typ, err := optionalNumber(row, "Type", 1)
		if err != nil {
			return facts, err
		}
		read, err := optionalNumber(row, "ReadedToMsgSeq", 8)
		if err != nil {
			return facts, err
		}
		deleted, err := optionalNumber(row, "DeletedAtMsgSeq", 8)
		if err != nil {
			return facts, err
		}
		unread, err := optionalNumber(row, "UnreadCount", 4)
		if err != nil {
			return facts, err
		}
		pending := row.Table == "PendingConversation"
		if pending && strings.HasSuffix(id.Channel.ID, "____cmd") {
			typ = 1
		}
		if typ > 1 || id.UID == "" || id.Channel.ID == "" {
			return facts, errors.New("unsupported or incomplete original conversation")
		}
		if (typ == 1) != strings.HasSuffix(id.Channel.ID, "____cmd") {
			return facts, errors.New("original conversation type differs from command-channel identity")
		}
		facts.Conversation = &migration.SourceConversation{SourceID: row.ID, UID: id.UID, Channel: id.Channel, Type: uint8(typ), ReadSeq: read, DeletedToSeq: deleted, UnreadCount: uint32(unread), CreatedAtNS: created, UpdatedAtNS: updated, Pending: pending}
	case "Device":
		flag, err := scalar64(row, "DeviceFlag")
		if err != nil {
			return facts, err
		}
		level := row.Fields["DeviceLevel"]
		if len(level) != 1 || id.UID == "" || !utf8.Valid(row.Fields["Token"]) {
			return facts, errors.New("invalid original device credentials")
		}
		facts.Device = &migration.SourceDevice{SourceID: row.ID, UID: id.UID, Token: string(row.Fields["Token"]), Flag: flag, Level: level[0], CreatedAtNS: created, UpdatedAtNS: updated}
	case "MessageEventSeq":
		if row.Kind != Other || len(row.Key) != 20 || len(row.Value) != 8 || id.Channel.ID == "" || id.ClientMsgNo == "" || eventChannelHash(id.Channel.ID, id.Channel.Type) != binary.BigEndian.Uint64(row.Key[4:12]) || stringHash(id.ClientMsgNo) != binary.BigEndian.Uint64(row.Key[12:20]) {
			return facts, errors.New("invalid or unresolved original event cursor")
		}
		facts.EventCursor = &meta.MessageEventCursor{ChannelID: id.Channel.ID, ChannelType: int64(id.Channel.Type), ClientMsgNo: id.ClientMsgNo, LastMsgEventSeq: binary.BigEndian.Uint64(row.Value)}
	case "MessageEventState":
		var source originalEventState
		decoder := json.NewDecoder(bytes.NewReader(row.Value))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&source); err != nil {
			return facts, err
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return facts, errors.New("trailing original event state data")
		}
		if source.ChannelID != id.Channel.ID || source.ChannelType != id.Channel.Type || source.ClientMsgNo != id.ClientMsgNo || source.EventKey == "" || source.LastMsgEventSeq == 0 || source.LastEventID == "" || source.LastEventType == "" {
			return facts, errors.New("incomplete or conflicting original event state")
		}
		switch source.Status {
		case "", "open", "closed", "error", "cancelled":
		default:
			return facts, errors.New("unsupported original event status")
		}
		facts.EventState = &meta.MessageEventState{
			ChannelID: source.ChannelID, ChannelType: int64(source.ChannelType), ClientMsgNo: source.ClientMsgNo, EventKey: source.EventKey,
			Status: source.Status, LastMsgEventSeq: source.LastMsgEventSeq, LastEventID: source.LastEventID, LastEventType: source.LastEventType,
			LastVisibility: source.LastVisibility, LastOccurredAt: source.LastOccurredAt, SnapshotPayload: source.SnapshotPayload, EndReason: source.EndReason, Error: source.Error,
		}
	default:
		return facts, fmt.Errorf("business decoding is unavailable for %s", row.Table)
	}
	return facts, nil
}

// originalEventState is exactly the persisted JSON shape at SourceCommit.
// Unknown fields must block conversion instead of disappearing on re-encoding.
type originalEventState struct {
	ChannelID       string `json:"channel_id"`
	ChannelType     uint8  `json:"channel_type"`
	ClientMsgNo     string `json:"client_msg_no"`
	EventKey        string `json:"event_key"`
	Status          string `json:"status"`
	LastMsgEventSeq uint64 `json:"last_msg_event_seq"`
	LastEventID     string `json:"last_event_id,omitempty"`
	LastEventType   string `json:"last_event_type,omitempty"`
	LastVisibility  string `json:"last_visibility,omitempty"`
	LastOccurredAt  int64  `json:"last_occurred_at,omitempty"`
	SnapshotPayload []byte `json:"snapshot_payload,omitempty"`
	EndReason       uint8  `json:"end_reason,omitempty"`
	Error           string `json:"error,omitempty"`
}

func optionalTime(row Row, name string) (int64, error) {
	value, exists := row.Fields[name]
	if !exists {
		return 0, nil
	}
	if len(value) != 8 {
		return 0, fmt.Errorf("%s.%s has invalid time bytes", row.Table, name)
	}
	return int64(binary.BigEndian.Uint64(value)), nil
}

func optionalNumber(row Row, name string, width int) (uint64, error) {
	value, exists := row.Fields[name]
	if !exists {
		return 0, nil
	}
	if len(value) != width {
		return 0, fmt.Errorf("%s.%s has invalid scalar bytes", row.Table, name)
	}
	var n uint64
	for _, b := range value {
		n = n<<8 | uint64(b)
	}
	return n, nil
}
