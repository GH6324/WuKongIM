package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/WuKongIM/WuKongIM/pkg/db/transfer"
)

// recoverSourceConversations mirrors original AddConversationsIfNotExist at
// the UID authority. Conflicting absent-row intents are ambiguous after restart
// and fail; no node order or maximum read position is allowed to choose one.
func recoverSourceConversations(ctx context.Context, capture SourceCapture, workspace Workspace, decoder RecordDecoder, batch *captureBatch, selection *SourceSelection) error {
	for _, node := range capture.Nodes {
		err := walkSourceRows(ctx, workspace, node.NodeID, func(row Row) error {
			if row.Table != "PendingConversation" {
				return nil
			}
			id, err := decoder.Identify(row)
			if err != nil {
				return err
			}
			description, err := decoder.Describe(row, id)
			if err != nil {
				return err
			}
			_, exists, err := workspace.Get(ctx, selectedKey("Conversation", description.Key))
			if err != nil {
				return err
			}
			if exists {
				selection.Preserved["pending_conversation_already_exists"]++
				return nil
			}
			sum := sha256.Sum256(description.Comparable)
			candidate := sourceCandidate{NodeID: node.NodeID, SourceKey: sourceRowKey(node.NodeID, row), Table: row.Table, Kind: row.Kind, Identity: id, LogicalKey: description.Key, Digest: hex.EncodeToString(sum[:])}
			data, err := json.Marshal(candidate)
			if err != nil {
				return err
			}
			return batch.add(transfer.SpoolRow{Key: candidateKey("pending", node.NodeID, row.Table, description.Key), Value: data})
		})
		if err != nil {
			return err
		}
	}
	if err := batch.flush(); err != nil {
		return err
	}
	// Flush each selected intent so subsequent nodes observe the existing
	// choice. Tail files are small relative to message history; this path is
	// deliberately serialized rather than retaining an unbounded UID map.
	return workspace.Walk(ctx, []byte("candidate/pending/"), func(row transfer.SpoolRow) error {
		var candidate sourceCandidate
		if err := json.Unmarshal(row.Value, &candidate); err != nil {
			return err
		}
		key := selectedKey(candidate.Table, candidate.LogicalKey)
		data, exists, err := workspace.Get(ctx, key)
		if err != nil {
			return err
		}
		if exists {
			var previous sourceCandidate
			if err := json.Unmarshal(data, &previous); err != nil {
				return err
			}
			if previous.Digest != candidate.Digest {
				return errors.New("conflicting pending conversations cannot be recovered deterministically")
			}
			return nil
		}
		return workspace.Put(ctx, []transfer.SpoolRow{{Key: key, Value: row.Value}})
	})
}
