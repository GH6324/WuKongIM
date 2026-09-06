//go:build e2e

package original_v2

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	wkclient "github.com/WuKongIM/WuKongIM/pkg/client"
	"github.com/WuKongIM/WuKongIM/pkg/protocol/frame"
	"github.com/WuKongIM/WuKongIM/test/e2e/suite"
	"github.com/stretchr/testify/require"
)

func TestOriginalV2ArchiveBootsWithOriginalCredentialsHistoryAndSequence(t *testing.T) {
	runOriginalV2(t, 1, 1, false)
}
func TestOriginalV2ArchiveChangesToThreeNodeCluster(t *testing.T) { runOriginalV2(t, 1, 3, false) }

func TestOriginalThreeNodeV2ArchiveChangesToSingleNodeCluster(t *testing.T) {
	runOriginalV2(t, 3, 1, false)
}
func TestOriginalThreeNodeV2ArchiveKeepsThreeNodeCluster(t *testing.T) { runOriginalV2(t, 3, 3, false) }

func TestOriginalThreeNodeV2ArchiveChangesToFiveNodeCluster(t *testing.T) {
	runOriginalV2(t, 3, 5, false)
}

func TestOriginalEmptyGroupRemainsInvisibleUntilFirstMessage(t *testing.T) {
	runOriginalV2(t, 1, 1, true)
}

func runOriginalV2(t *testing.T, sourceCount, nodeCount int, emptyGroup bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	root := t.TempDir()
	var sources []any
	for i := 1; i <= sourceCount; i++ {
		name := "original-v2-server.tar.gz"
		if emptyGroup {
			name = "original-v2-empty.tar.gz"
		}
		if sourceCount > 1 {
			name = fmt.Sprintf("original-v2-three-%d.tar.gz", i)
		}
		source := suite.UnpackMigrationFixture(t, name)
		sources = append(sources, map[string]any{"node_id": i, "data_dir": source, "shard_count": 2})
	}
	s := suite.New(t)
	var specs []suite.NodeSpec
	var targets []any
	for i := 1; i <= nodeCount; i++ {
		ports := suite.ReserveLoopbackPorts(t)
		nodeRoot := filepath.Join(root, fmt.Sprintf("node%d", i))
		require.NoError(t, os.Mkdir(nodeRoot, 0700))
		node := suite.NodeSpec{ID: uint64(i), Name: fmt.Sprintf("migrated-%d", i), RootDir: nodeRoot, DataDir: filepath.Join(nodeRoot, "data"), ConfigPath: filepath.Join(nodeRoot, "wukongim.toml"), StdoutPath: filepath.Join(nodeRoot, "stdout.log"), StderrPath: filepath.Join(nodeRoot, "stderr.log"), ClusterAddr: ports.ClusterAddr, GatewayAddr: ports.GatewayAddr, APIAddr: ports.APIAddr, ManagerAddr: ports.ManagerAddr}
		node.ConfigOverrides = map[string]string{"WK_CLUSTER_ID": "migration-e2e", "WK_CLUSTER_INITIAL_SLOT_COUNT": "4", "WK_CLUSTER_HASH_SLOT_COUNT": "256", "WK_CLUSTER_SLOT_REPLICA_N": fmt.Sprint(nodeCount), "WK_CLUSTER_CHANNEL_REPLICA_N": fmt.Sprint(nodeCount), "WK_GATEWAY_TOKEN_AUTH_ON": "true"}
		if nodeCount == 1 {
			node.ConfigOverrides["WK_CLUSTER_NODES"] = fmt.Sprintf(`[{"id":1,"addr":%q}]`, ports.ClusterAddr)
		}
		specs = append(specs, node)
		targets = append(targets, map[string]any{"node_id": i, "addr": ports.ClusterAddr, "data_dir": node.DataDir})
	}
	plan := map[string]any{"version": 1, "source_commit": "a888f89533d0e7d1b2030e06504ca97f1ad891d4", "sources": sources, "target": map[string]any{"cluster_id": "migration-e2e", "created_at": "2026-09-06T06:56:42Z", "slot_count": 4, "hash_slot_count": 256, "replicas": nodeCount, "channel_replicas": nodeCount, "nodes": targets}}

	data, err := json.Marshal(plan)
	require.NoError(t, err)
	planPath := filepath.Join(root, "plan.json")
	require.NoError(t, os.WriteFile(planPath, data, 0600))
	cli := suite.BuildMigrationCLI(t)
	args := []string{"--plan", planPath, "--workspace", filepath.Join(root, "scratch")}
	suite.RunMigrationCLI(t, ctx, cli, append([]string{"prepare"}, args...)...)
	args = append(args, "--archive", filepath.Join(root, "archive"))
	for _, verb := range []string{"export", "import", "verify"} {
		suite.RunMigrationCLI(t, ctx, cli, append([]string{verb}, args...)...)
	}
	cluster := s.StartPreparedCluster(specs)
	require.NoError(t, cluster.WaitHTTPReady(ctx), cluster.DumpDiagnostics())
	started := cluster.MustNode(1)
	if emptyGroup {
		var conversations []json.RawMessage
		_, err = suite.PostJSON(ctx, "http://"+started.APIAddr()+"/conversation/sync", map[string]any{"uid": "emptyalice", "msg_count": 10}, &conversations)
		require.NoError(t, err)
		require.Empty(t, conversations, "an original stored update timestamp must not activate an empty group")
		client, err := wkclient.New(wkclient.Config{Addr: started.GatewayAddr(), OperationTimeout: 5 * time.Second})
		require.NoError(t, err)
		defer client.Close()
		_, err = client.Connect(ctx, wkclient.ConnectOptions{UID: "emptyalice", DeviceID: "empty-group-check", DeviceFlag: frame.DeviceFlag(1), Token: "synthetic-emptyalice"})
		require.NoError(t, err)
		sent, err := client.Send(ctx, wkclient.Message{ClientSeq: 1, ClientMsgNo: "first-after-migration", ChannelID: "emptygroup", ChannelType: 2, Payload: []byte("first message")})
		require.NoError(t, err)
		require.Equal(t, frame.ReasonSuccess, sent.ReasonCode, "the original subscriber must retain send permission")
		require.Equal(t, uint64(1), sent.MessageSeq)
		_, err = suite.PostJSON(ctx, "http://"+started.APIAddr()+"/conversation/sync", map[string]any{"uid": "emptyalice", "msg_count": 10}, &conversations)
		require.NoError(t, err)
		require.Len(t, conversations, 1)
		return
	}
	// Historical API rows and original event projections are read before any write.
	var page struct {
		Messages []struct {
			MessageID   int64  `json:"message_id"`
			MessageSeq  uint64 `json:"message_seq"`
			ClientMsgNo string `json:"client_msg_no"`
			Payload     string `json:"payload"`
			Expire      uint32 `json:"expire"`
			Timestamp   int64  `json:"timestamp"`
			Header      struct {
				RedDot   int `json:"red_dot"`
				SyncOnce int `json:"sync_once"`
			} `json:"header"`
			EventMeta *struct {
				HasEvents       bool   `json:"has_events"`
				Completed       bool   `json:"completed"`
				LastMsgEventSeq uint64 `json:"last_msg_event_seq"`
			} `json:"event_meta"`
		} `json:"messages"`
	}
	body := map[string]any{"login_uid": "migrationalice", "channel_id": "migrationgroup", "channel_type": 2, "start_message_seq": 0, "end_message_seq": 0, "pull_mode": 1, "limit": 10, "include_event_meta": 1}
	_, err = suite.PostJSON(ctx, "http://"+started.APIAddr()+"/channel/messagesync", body, &page)
	require.NoError(t, err, started.DumpDiagnostics())
	require.Len(t, page.Messages, 3)
	expected := []int64{2096462572973723648, 2096462572977917952, 2096462573007278080}
	maxID := int64(2096462782110109696)
	if sourceCount > 1 {
		expected = []int64{2096502314880733184, 2096502314893316096, 2096502314910093312}
		maxID = 2096502314910093312
	}
	for i, m := range page.Messages {
		require.Equal(t, expected[i], m.MessageID)
		require.Equal(t, uint64(i+1), m.MessageSeq)
		if sourceCount > 1 {
			require.Equal(t, fmt.Sprintf("multi-source-%d", i), m.ClientMsgNo)
			require.Equal(t, "aGVsbG8=", m.Payload)
			require.Zero(t, m.Expire)
			require.Equal(t, int64(1788680077), m.Timestamp)
		} else {
			require.Equal(t, fmt.Sprintf("original-v2-%d", i), m.ClientMsgNo)
			require.Equal(t, base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("消息%d", i))), m.Payload)
			require.Equal(t, uint32(3600), m.Expire)
			require.Equal(t, int64(1788670602), m.Timestamp)
		}
		require.Equal(t, 1, m.Header.RedDot)
	}
	if sourceCount == 1 {
		for _, denied := range []struct {
			uid    string
			reason frame.ReasonCode
		}{{"migrationdenied", frame.ReasonInBlacklist}, {"migrationoutsider", frame.ReasonSubscriberNotExist}} {
			response, err := suite.PostMessageSend(ctx, started.APIAddr(), map[string]any{"from_uid": denied.uid, "channel_id": "migrationgroup", "channel_type": 2, "client_msg_no": "permission-" + denied.uid, "payload": "cGVybWlzc2lvbg=="})
			require.NoError(t, err)
			require.Equal(t, uint8(denied.reason), response.Reason)
			require.Zero(t, response.MessageID)
			require.Zero(t, response.MessageSeq)
		}
		// The original closed stream and CMD cursors remain visible without replaying
		// any historical events or invoking token/membership mutation endpoints.
		require.Nil(t, page.Messages[0].EventMeta, "original base message has no stream setting")
		var eventPage struct {
			Data struct {
				Next   uint64 `json:"next_msg_event_seq"`
				More   int    `json:"more"`
				Events []struct {
					ID      string `json:"event_id"`
					Type    string `json:"event_type"`
					Seq     uint64 `json:"msg_event_seq"`
					Payload struct {
						Text string `json:"text"`
					} `json:"payload"`
				} `json:"events"`
			} `json:"data"`
		}
		_, err = suite.PostJSON(ctx, "http://"+started.APIAddr()+"/message/eventsync", map[string]any{"channel_id": "migrationgroup", "channel_type": 2, "from_uid": "migrationbob", "client_msg_no": "original-v2-0", "limit": 100}, &eventPage)
		require.NoError(t, err)
		require.Len(t, eventPage.Data.Events, 1)
		require.Equal(t, "event-close", eventPage.Data.Events[0].ID)
		require.Equal(t, "stream.close", eventPage.Data.Events[0].Type)
		require.Equal(t, uint64(2), eventPage.Data.Events[0].Seq)
		require.Equal(t, "持久快照", eventPage.Data.Events[0].Payload.Text)
		require.Equal(t, uint64(2), eventPage.Data.Next)
		require.Zero(t, eventPage.Data.More)

		var commands []struct {
			MessageID  int64  `json:"message_id"`
			MessageSeq uint64 `json:"message_seq"`
			Header     struct {
				SyncOnce int `json:"sync_once"`
			} `json:"header"`
		}
		_, err = suite.PostJSON(ctx, "http://"+started.APIAddr()+"/message/sync", map[string]any{"uid": "migrationbob", "limit": 10}, &commands)
		require.NoError(t, err)
		require.Len(t, commands, 1)
		require.Equal(t, int64(2096462782110109696), commands[0].MessageID)
		require.Equal(t, uint64(1), commands[0].MessageSeq)
		require.Equal(t, 1, commands[0].Header.SyncOnce)
		commands = nil
		_, err = suite.PostJSON(ctx, "http://"+started.APIAddr()+"/message/sync", map[string]any{"uid": "migrationalice", "limit": 10}, &commands)
		require.NoError(t, err)
		require.Empty(t, commands)
		var conversations []json.RawMessage
		_, err = suite.PostJSON(ctx, "http://"+started.APIAddr()+"/conversation/sync", map[string]any{"uid": "migrationbob", "msg_count": 10}, &conversations)
		require.NoError(t, err)
		require.Empty(t, conversations)
	}
	bad, err := wkclient.New(wkclient.Config{Addr: started.GatewayAddr(), OperationTimeout: 5 * time.Second})
	require.NoError(t, err)
	_, err = bad.Connect(ctx, wkclient.ConnectOptions{UID: "migrationalice", DeviceID: "wrong-token", DeviceFlag: frame.WEB, Token: "wrong-token"})
	require.Error(t, err)
	require.NoError(t, bad.Close())
	client, err := wkclient.New(wkclient.Config{Addr: started.GatewayAddr(), OperationTimeout: 5 * time.Second})
	require.NoError(t, err)
	defer client.Close()
	_, err = client.Connect(ctx, wkclient.ConnectOptions{UID: "migrationalice", DeviceID: "migration-check", DeviceFlag: frame.DeviceFlag(1), Token: "synthetic-migrationalice"})
	require.NoError(t, err, started.DumpDiagnostics())
	sent, err := client.Send(ctx, wkclient.Message{ClientSeq: 1, ClientMsgNo: "after-migration", ChannelID: "migrationgroup", ChannelType: 2, Payload: []byte("迁移后的消息")})
	require.NoError(t, err, started.DumpDiagnostics())
	require.Equal(t, frame.ReasonSuccess, sent.ReasonCode)
	require.Equal(t, uint64(4), sent.MessageSeq)
	require.Greater(t, sent.MessageID, maxID)
	retried, err := client.Send(ctx, wkclient.Message{ClientSeq: 2, ClientMsgNo: "after-migration", ChannelID: "migrationgroup", ChannelType: 2, Payload: []byte("迁移后的消息")})
	require.NoError(t, err)
	require.Equal(t, sent.MessageID, retried.MessageID)
	require.Equal(t, sent.MessageSeq, retried.MessageSeq)
	require.NoError(t, client.Close())
	require.NoError(t, cluster.RestartNode(1))
	require.NoError(t, cluster.WaitHTTPReady(ctx), cluster.DumpDiagnostics())
	page.Messages = nil
	_, err = suite.PostJSON(ctx, "http://"+started.APIAddr()+"/channel/messagesync", body, &page)
	require.NoError(t, err)
	require.Len(t, page.Messages, 4)
	require.Equal(t, sent.MessageID, page.Messages[3].MessageID)
	if nodeCount >= 3 {
		before := suite.RequireChannelRuntimeMetaEventually(t, cluster, started, "migrationgroup", 2, 10*time.Second)
		require.NoError(t, cluster.MustNode(before.Leader).Stop())
		var survivor *suite.StartedNode
		for i := range cluster.Nodes {
			if cluster.Nodes[i].Spec.ID != before.Leader {
				survivor = &cluster.Nodes[i]
				break
			}
		}
		failoverCtx, stopFailover := context.WithTimeout(ctx, 60*time.Second)
		defer stopFailover()
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		var after suite.ChannelRuntimeMeta
		for {
			current, readErr := suite.GetChannelRuntimeMeta(failoverCtx, survivor, "migrationgroup", 2)
			if readErr == nil && current.Leader != 0 && current.Leader != before.Leader && current.Status == "active" && current.WriteFenceToken == "" {
				after = current
				break
			}
			select {
			case <-failoverCtx.Done():
				t.Fatalf("migrated leader did not fail over: old=%d last=%+v err=%v\n%s", before.Leader, current, readErr, cluster.DumpDiagnostics())
			case <-ticker.C:
			}
		}
		require.NotEqual(t, before.Leader, after.Leader)
		page.Messages = nil
		_, err = suite.PostJSON(ctx, "http://"+survivor.APIAddr()+"/channel/messagesync", body, &page)
		require.NoError(t, err)
		require.Len(t, page.Messages, 4)
		require.Equal(t, sent.MessageID, page.Messages[3].MessageID)
		next, err := suite.PostMessageSendEventually(ctx, survivor.APIAddr(), map[string]any{"from_uid": "migrationalice", "channel_id": "migrationgroup", "channel_type": 2, "client_msg_no": "after-leader-loss", "payload": base64.StdEncoding.EncodeToString([]byte("new leader"))})
		require.NoError(t, err, cluster.DumpDiagnostics())
		require.Greater(t, next.MessageSeq, sent.MessageSeq, "native term barriers may consume internal sequence positions")
		require.NoError(t, cluster.StartStoppedNode(before.Leader))
		require.NoError(t, cluster.WaitHTTPReady(ctx))
	}

}
