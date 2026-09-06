package migrationv2

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/WuKongIM/WuKongIM/internal/usecase/migration"
)

// Describe decodes source format only. Slot and Channel authority decisions
// belong to the migration use case, after all hash references are resolved.
func (Reader) Describe(row Row, id RecordIdentity) (d migration.RecordDescription, err error) {
	var key any
	value := row.Fields
	switch row.Table {
	case "Plugin":
		key = []any{string(row.Fields["No"])}
		plugin := &migration.PluginDescription{}
		if v := row.Fields["Methods"]; len(v) > 0 {
			if err := json.Unmarshal(v, &plugin.Methods); err != nil {
				return d, errors.New("invalid original plugin methods")
			}
		}
		if v := row.Fields["Config"]; len(v) > 0 {
			var config map[string]json.RawMessage
			if err := json.Unmarshal(v, &config); err != nil {
				return d, errors.New("invalid original plugin config")
			}
			plugin.HasConfig = len(config) != 0
		}
		d.Plugin = plugin
	case "User", "SystemUid":
		key = []any{id.UID}
	case "Device":
		flag, err := scalar64(row, "DeviceFlag")
		if err != nil {
			return d, err
		}
		key = []any{id.UID, flag}
		value = cloneFields(row.Fields)
		value["SourceID"] = uint64Bytes(row.ID)
	case "ChannelInfo", "ChannelClusterConfig":
		key = []any{id.Channel.ID, id.Channel.Type}
	case "Subscriber", "Allowlist", "Denylist":
		key = []any{id.Channel.ID, id.Channel.Type, id.UID}
	case "Conversation", "PendingConversation":
		key = []any{id.UID, id.Channel.ID, id.Channel.Type}
	case "PluginUser":
		key = []any{id.UID, string(row.Fields["PluginNo"])}
	case "Message":
		if row.Kind == Other {
			key = []any{id.Channel.ID, id.Channel.Type, "tail"}
			// Last append wall time differs between replicas. Preserve its
			// original bytes, but compare the actual persisted sequence.
			d.Comparable = append([]byte(nil), row.Value[:8]...)
		} else {
			key = []any{id.Channel.ID, id.Channel.Type, row.ID}
		}
	case "MessageEventState":
		var v map[string]json.RawMessage
		if err := json.Unmarshal(row.Value, &v); err != nil {
			return d, err
		}
		var eventKey string
		if err := json.Unmarshal(v["event_key"], &eventKey); err != nil {
			return d, err
		}
		key = []any{id.Channel.ID, id.Channel.Type, id.ClientMsgNo, eventKey}
		d.Comparable, err = json.Marshal(v)
		if err != nil {
			return d, err
		}
	case "MessageEventSeq":
		key = []any{id.Channel.ID, id.Channel.Type, id.ClientMsgNo}
		d.Comparable = append([]byte(nil), row.Value...)
	default:
		return d, fmt.Errorf("unsupported v2 business record %s", row.Table)
	}
	if row.Table == "ChannelClusterConfig" {
		authority, err := decodeChannelAuthority(row, id)
		if err != nil {
			return d, err
		}
		d.Authority = &authority
		value = cloneFields(row.Fields)
		var replicas []byte
		for _, node := range authority.Replicas {
			replicas = append(replicas, uint64Bytes(node)...)
		}
		value["Replicas"] = replicas
	}
	encoded, err := json.Marshal(key)
	if err != nil {
		return d, err
	}
	d.Key = base64.RawURLEncoding.EncodeToString(encoded)
	if d.Comparable == nil {
		d.Comparable, err = json.Marshal(value)
	}
	return d, err
}

func decodeChannelAuthority(row Row, id RecordIdentity) (a migration.ChannelAuthority, err error) {
	a.Channel = id.Channel
	if a.Leader, err = scalar64(row, "LeaderId"); err != nil {
		return a, err
	}
	if a.Version, err = scalar64(row, "ConfVersion"); err != nil {
		return a, err
	}
	term := row.Fields["Term"]
	if len(term) != 4 {
		return a, errors.New("invalid v2 channel term")
	}
	a.Term = binary.BigEndian.Uint32(term)
	for _, name := range []string{"MigrateFrom", "MigrateTo"} {
		v, err := scalar64(row, name)
		if err != nil {
			return a, err
		}
		if v != 0 {
			return a, errors.New("v2 channel has an unfinished migration")
		}
	}
	if status := row.Fields["Status"]; len(status) != 1 || status[0] != 0 || len(row.Fields["Learners"]) != 0 {
		return a, errors.New("v2 channel has an unfinished election or learners")
	}
	replicas := row.Fields["Replicas"]
	if len(replicas) == 0 || len(replicas)%8 != 0 || len(replicas) > 8*1024 {
		return a, errors.New("invalid v2 channel replicas")
	}
	found := false
	seen := map[uint64]bool{}
	for offset := 0; offset < len(replicas); offset += 8 {
		node := binary.BigEndian.Uint64(replicas[offset:])
		if node == 0 || seen[node] {
			return a, errors.New("invalid v2 channel replica identity")
		}
		seen[node] = true
		found = found || node == a.Leader
		a.Replicas = append(a.Replicas, node)
	}
	if !found || a.Term == 0 || a.Version == 0 {
		return a, errors.New("v2 channel authority is not initialized")
	}
	sort.Slice(a.Replicas, func(i, j int) bool { return a.Replicas[i] < a.Replicas[j] })
	return a, nil
}
func scalar64(row Row, name string) (uint64, error) {
	v := row.Fields[name]
	if len(v) != 8 {
		return 0, fmt.Errorf("%s.%s has no complete scalar", row.Table, name)
	}
	return binary.BigEndian.Uint64(v), nil
}
func uint64Bytes(n uint64) []byte { b := make([]byte, 8); binary.BigEndian.PutUint64(b, n); return b }
func cloneFields(fields map[string][]byte) map[string][]byte {
	result := make(map[string][]byte, len(fields)+1)
	for k, v := range fields {
		result[k] = v
	}
	return result
}
