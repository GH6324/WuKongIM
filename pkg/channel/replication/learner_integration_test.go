//go:build integration

package replication

import (
	"context"
	"testing"
	"time"

	ch "github.com/WuKongIM/WuKongIM/pkg/channel"
	channelstore "github.com/WuKongIM/WuKongIM/pkg/channel/store"
	"github.com/stretchr/testify/require"
)

// A replacement learner must receive existing exact proposals even while the
// task fences business writes; its durable HW must include the recovery barrier.
func TestNativeLearnerCatchesUpUnderWriteFence(t *testing.T) {
	router := &runtimeTestRouter{servers: make(map[ch.NodeID]*ExchangeServer)}
	runtimes := make(map[ch.NodeID]*Runtime)
	stores := make(map[ch.NodeID]ReplicaStore)
	for _, node := range []ch.NodeID{1, 2, 3, 4} {
		store, err := NewStoreAdapter(StoreAdapterConfig{Factory: channelstore.NewMemoryFactory(), MaxBatchItems: MaxExchangeBatchItems, MaxBatchBytes: MaxExchangeBatchBytes})
		require.NoError(t, err)
		runtime, err := NewRuntime(RuntimeConfig{LocalNode: node, Store: store, Link: runtimeTestLink{from: node, router: router}})
		require.NoError(t, err)
		runtimes[node], stores[node] = runtime, store
		router.register(node, runtime.ExchangeServer())
		t.Cleanup(func() { require.NoError(t, runtime.Close(context.Background())) })
	}
	a := Authority{Key: "2:native-learner", ChannelID: ch.ChannelID{ID: "native-learner", Type: 2}, ID: AuthorityID{ChannelEpoch: 1, LeaderTerm: 1, FenceVersion: 1}, Leader: 1, Voters: []ch.NodeID{1, 2, 3}, WriteQuorum: 2}
	ctx := context.Background()
	_, err := runtimes[1].Log().Install(ctx, a)
	require.NoError(t, err)
	_, err = runtimes[1].Log().Commit(ctx, Proposal{Key: a.Key, Expected: a.ID, CommandID: ch.CommandID{31: 1}, Records: []ch.Record{{ID: 101, Epoch: 1, FromUID: "sender", ClientMsgNo: "native", Payload: []byte("native"), SizeBytes: 6, ServerTimestampMS: 111}}})
	require.NoError(t, err)
	a.ID.ChannelEpoch++
	a.ID.FenceVersion++
	a.Learners = []ch.NodeID{4}
	a.WriteFence = ch.WriteFence{Token: "replacement", Version: 2, Reason: ch.WriteFenceReasonReplicaReplace}
	router.register(4, nil)
	installed, err := runtimes[1].Log().Install(ctx, a)
	require.NoError(t, err)
	require.Greater(t, installed.HW, uint64(1))
	router.register(4, runtimes[4].ExchangeServer())
	require.Eventually(t, func() bool {
		loaded, err := stores[4].Load(ctx, LoadBatch{Items: []LoadRequest{{ChannelKey: a.Key, ChannelID: a.ChannelID}}})
		return err == nil && len(loaded.Items) == 1 && loaded.Items[0].Err == nil && loaded.Items[0].State.LEO == installed.HW && loaded.Items[0].State.Committed == installed.HW
	}, 2*time.Second, time.Millisecond, "non-voter learner never received the native committed prefix")
	// With the learner up but both remote voters down, no business success is
	// possible. Restoring voters permits an exact retry of the pending proposal.
	a.WriteFence = ch.WriteFence{}
	a.ID.FenceVersion++
	_, err = runtimes[1].Log().Install(ctx, a)
	require.NoError(t, err)
	router.register(2, nil)
	router.register(3, nil)
	proposal := Proposal{Key: a.Key, Expected: a.ID, CommandID: ch.CommandID{31: 2}, Records: []ch.Record{{ID: 102, Epoch: a.ID.ChannelEpoch, FromUID: "sender", ClientMsgNo: "native-next", Payload: []byte("next"), SizeBytes: 4, ServerTimestampMS: 112}}}
	bounded, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	_, err = runtimes[1].Log().Commit(bounded, proposal)
	cancel()
	require.Error(t, err, "a learner must never replace a missing voter")
	router.register(2, runtimes[2].ExchangeServer())
	router.register(3, runtimes[3].ExchangeServer())
	receipt, err := runtimes[1].Log().Commit(ctx, proposal)
	require.NoError(t, err)
	require.Greater(t, receipt.HW, installed.HW)
	require.Eventually(t, func() bool {
		loaded, err := stores[4].Load(ctx, LoadBatch{Items: []LoadRequest{{ChannelKey: a.Key, ChannelID: a.ChannelID}}})
		return err == nil && len(loaded.Items) == 1 && loaded.Items[0].Err == nil && loaded.Items[0].State.Committed == receipt.HW
	}, 2*time.Second, time.Millisecond, "learner must follow new commits after its initial copy")
}
