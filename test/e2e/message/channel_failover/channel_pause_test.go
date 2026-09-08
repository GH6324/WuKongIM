//go:build e2e && (darwin || linux)

package channel_failover

import (
	"context"
	"encoding/base64"
	"fmt"
	"syscall"
	"testing"
	"time"

	"github.com/WuKongIM/WuKongIM/pkg/protocol/frame"
	"github.com/WuKongIM/WuKongIM/test/e2e/suite"
	"github.com/stretchr/testify/require"
)

// TestChannelThreeNodeLeaderFailoverWhileProcessPaused checks availability before
// the paused process resumes. History safety alone does not satisfy this gate.
func TestChannelThreeNodeLeaderFailoverWhileProcessPaused(t *testing.T) {
	runPausedLeaderScenario(t, true)
}

// TestChannelAcknowledgedHistorySurvivesProcessPause permits bounded write
// unavailability but never loss or sequence reuse after incomplete recovery.
func TestChannelAcknowledgedHistorySurvivesProcessPause(t *testing.T) {
	runPausedLeaderScenario(t, false)
}

func runPausedLeaderScenario(t *testing.T, requireAvailable bool) {
	t.Helper()
	cluster := suite.New(t).StartThreeNodeCluster(fastRecoveryOptionsForNodes(3, map[string]string{
		"WK_CLUSTER_INITIAL_SLOT_COUNT": "10", "WK_CHANNEL_MIGRATION_MAX_PAGES_PER_TICK": "1",
	})...)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	require.NoError(t, cluster.WaitHTTPReady(ctx), cluster.DumpDiagnostics())
	survivor := cluster.MustNode(1)
	slots := cluster.ManagerClient(t, 1).MustSlots(t)
	var affected failoverChannel
	var bootstrap sentMessage
	for _, slot := range slots {
		if slot.Assignment.PreferredLeaderID != 3 {
			continue
		}
		id, ok := channelIDForPhysicalSlot(slots, slot, fmt.Sprintf("e2e-pause-%d", time.Now().UnixNano()))
		require.True(t, ok)
		createGroupChannel(t, survivor, id, frame.ChannelTypeGroup)
		bootstrap = sendGroupMessage(t, survivor, id, frame.ChannelTypeGroup, "bootstrap")
		meta := suite.RequireChannelRuntimeMetaEventually(t, cluster, survivor, id, frame.ChannelTypeGroup, 20*time.Second)
		require.Equal(t, uint64(3), meta.Leader)
		require.ElementsMatch(t, []uint64{1, 2, 3}, meta.Replicas)
		require.ElementsMatch(t, []uint64{1, 2, 3}, meta.ISR)
		require.Equal(t, int64(2), meta.MinISR)
		affected = failoverChannel{ChannelID: id, ChannelType: frame.ChannelTypeGroup, Leader: 3}
		break
	}
	require.NotEmpty(t, affected.ChannelID)
	process := cluster.MustNode(3).Process.Cmd.Process
	require.NotNil(t, process)
	t.Cleanup(func() { _ = process.Signal(syscall.SIGCONT) })
	// Pause immediately after ACK, before a history read can give the trailing
	// replica extra time to catch up. The observed metadata already proves RF3.
	affected.Pre = sendGroupMessage(t, survivor, affected.ChannelID, affected.ChannelType, "before-pause")
	require.NoError(t, process.Signal(syscall.SIGSTOP))
	time.Sleep(fastRecoveryHealthReportTTL + fastRecoveryHealthReportInterval)
	timeout := 20 * time.Second
	if requireAvailable {
		timeout = 60 * time.Second
	}
	sendCtx, sendCancel := context.WithTimeout(context.Background(), timeout)
	key := fmt.Sprintf("while-paused-%d", time.Now().UnixNano())
	response, sendErr := suite.PostMessageSendEventually(sendCtx, survivor.APIAddr(), map[string]any{
		"from_uid": "channel-ha-sender", "channel_id": affected.ChannelID, "channel_type": affected.ChannelType,
		"client_msg_no": key, "payload": base64.StdEncoding.EncodeToString([]byte("while-paused")),
	})
	sendCancel()
	if requireAvailable {
		require.NoError(t, sendErr, cluster.DumpDiagnostics())
	}
	if sendErr == nil {
		require.Equal(t, uint8(frame.ReasonSuccess), response.Reason)
		require.Greater(t, response.MessageSeq, affected.Pre.Seq)
		t.Log("send committed while node 3 remained paused")
	} else {
		t.Logf("write remained unavailable during pause: %v", sendErr)
	}
	require.NoError(t, process.Signal(syscall.SIGCONT))
	readyCtx, readyCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer readyCancel()
	require.NoError(t, cluster.WaitHTTPReady(readyCtx), cluster.DumpDiagnostics())
	recovered := sendGroupMessageWithin(t, survivor, affected.ChannelID, affected.ChannelType, "after-pause", 30*time.Second)
	require.Greater(t, recovered.Seq, affected.Pre.Seq)
	if sendErr == nil {
		require.Greater(t, recovered.Seq, response.MessageSeq)
		requireMessageOnceEventually(t, cluster, survivor, affected.ChannelID, affected.ChannelType, sentMessage{ID: uint64(response.MessageID), Seq: response.MessageSeq, ClientMsgNo: key})
	}
	requireMessageOnceEventually(t, cluster, survivor, affected.ChannelID, affected.ChannelType, bootstrap)
	requireMessageOnceEventually(t, cluster, survivor, affected.ChannelID, affected.ChannelType, affected.Pre)
	requireMessageOnceEventually(t, cluster, survivor, affected.ChannelID, affected.ChannelType, recovered)
}
