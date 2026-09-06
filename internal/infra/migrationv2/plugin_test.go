package migrationv2_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/WuKongIM/WuKongIM/internal/infra/migrationv2"
	"github.com/WuKongIM/WuKongIM/internal/usecase/migration"
	"github.com/WuKongIM/WuKongIM/pkg/db/transfer"
	"github.com/cockroachdb/pebble"
	"github.com/stretchr/testify/require"
)

func TestPrepareRejectsOriginalGlobalPluginBusinessWithoutUserBindings(t *testing.T) {
	var fixtures map[string][]struct{ Key, Value []byte }
	data, err := os.ReadFile("testdata/original-v2-plugin-kv.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &fixtures))
	require.Len(t, fixtures, 5)
	for name, rows := range fixtures {
		t.Run(name, func(t *testing.T) {
			dir := unpackNamedFixture(t, "original-v2-server.tar.gz")
			// Replay exact raw columns produced and read through original v2 public
			// AddOrUpdatePlugin/GetPlugin, only into this private fixture copy.
			db, err := pebble.Open(filepath.Join(dir, "db", "wukongimdb", "shard000"), &pebble.Options{})
			require.NoError(t, err)
			batch := db.NewBatch()
			for _, row := range rows {
				require.NoError(t, batch.Set(row.Key, row.Value, nil))
			}
			require.NoError(t, batch.Commit(pebble.Sync))
			require.NoError(t, batch.Close())
			require.NoError(t, db.Close())
			before := fileDigests(t, dir)
			w, err := transfer.OpenSpool(filepath.Join(t.TempDir(), "spool"), "plugin-check", 128<<20)
			require.NoError(t, err)
			defer w.Close()
			reader := migrationv2.Reader{}
			plan := migration.Plan{Version: 1, SourceCommit: migrationv2.SourceCommit, Sources: []migration.NodeOptions{{NodeID: 1, Options: migration.Options{DataDir: dir, ShardCount: 2}}}, Target: migration.TargetPlan{ClusterID: "plugin-check", CreatedAt: time.Unix(1788670602, 0).UTC(), SlotCount: 4, HashSlotCount: 256, Replicas: 1, ChannelReplicas: 1, Nodes: []migration.TargetNode{{NodeID: 1, Addr: "127.0.0.1:57882", DataDir: filepath.Join(t.TempDir(), "node")}}}}
			result, err := migration.Prepare(context.Background(), plan, w, reader, reader, nil)
			if name == "descriptor" {
				require.NoError(t, err)
				require.Equal(t, "prepared", result.Status)
				require.Positive(t, result.Selection.Preserved["old_management_data"])
			} else {
				require.ErrorContains(t, err, "plugin business methods/config")
				require.Empty(t, result.Status)
				require.NotContains(t, err.Error(), "synthetic-do-not-log")
			}
			require.Equal(t, before, fileDigests(t, dir))
		})
	}
}

func TestOriginalPluginDescriptionRejectsMalformedBusinessFieldsWithoutLoggingContent(t *testing.T) {
	for _, field := range []string{"Methods", "Config"} {
		t.Run(field, func(t *testing.T) {
			_, err := (migrationv2.Reader{}).Describe(migration.Row{Table: "Plugin", Kind: migration.Primary, Fields: map[string][]byte{"No": []byte("legacy"), field: []byte(`"synthetic-private-value"`)}}, migration.RecordIdentity{})
			require.ErrorContains(t, err, "invalid original plugin")
			require.NotContains(t, err.Error(), "synthetic-private-value")
		})
	}
}
