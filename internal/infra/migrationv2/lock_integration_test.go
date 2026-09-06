//go:build integration

package migrationv2_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/WuKongIM/WuKongIM/internal/infra/migrationv2"
	"github.com/cockroachdb/pebble"
	"github.com/stretchr/testify/require"
)

func TestOriginalV2WriterCannotOpenSourceDuringScan(t *testing.T) {
	dir := unpackFixture(t)
	attempt := func() error {
		cmd := exec.Command(os.Args[0], "-test.run=^TestOriginalV2WriterProcess$")
		cmd.Env = append(os.Environ(), "WK_MIGRATION_TEST_WRITER_DIR="+filepath.Join(dir, "db", "wukongimdb", "shard000"))
		return cmd.Run()
	}
	checked := false
	require.NoError(t, migrationv2.Scan(context.Background(), migrationv2.Options{DataDir: dir, ShardCount: 2}, func(migrationv2.Row) error {
		if !checked {
			checked = true
			var exit *exec.ExitError
			require.ErrorAs(t, attempt(), &exit)
			require.Equal(t, 23, exit.ExitCode())
		}
		return nil
	}))
	require.True(t, checked)
	require.NoError(t, attempt(), "the original writer can open again only after the scan releases all locks")
}

// A separate process is essential: original Pebble uses process-owned fcntl
// locks, so a goroutine pretending to be the source writer would be misleading.
func TestOriginalV2WriterProcess(t *testing.T) {
	dir := os.Getenv("WK_MIGRATION_TEST_WRITER_DIR")
	if dir == "" {
		t.Skip("subprocess helper")
	}
	db, err := pebble.Open(dir, &pebble.Options{ErrorIfNotExists: true})
	if err != nil {
		os.Exit(23)
	}
	if err := db.Close(); err != nil {
		os.Exit(24)
	}
	os.Exit(0)
}
