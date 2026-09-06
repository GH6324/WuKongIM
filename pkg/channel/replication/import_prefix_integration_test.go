//go:build integration

package replication_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	ch "github.com/WuKongIM/WuKongIM/pkg/channel"
	"github.com/WuKongIM/WuKongIM/pkg/channel/replication"
	"github.com/WuKongIM/WuKongIM/pkg/channel/store"
	"github.com/WuKongIM/WuKongIM/pkg/quorumlog"
	"github.com/stretchr/testify/require"
)

// Real disk stores and replication runtimes communicate through the production
// wire codec; only the transport is in-process. Process/API cutover is covered
// separately by the migration end-to-end suite.
func TestImportedHistoryRecoversThroughPublicQuorumRuntime(t *testing.T) {
	for _, emptyNode := range []ch.NodeID{1, 3} {
		t.Run(fmt.Sprintf("empty-node-%d", emptyNode), func(t *testing.T) { testImportedHistoryRuntime(t, emptyNode) })
	}
}

func testImportedHistoryRuntime(t *testing.T, emptyNode ch.NodeID) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id := ch.ChannelID{ID: "imported-cluster", Type: 2}
	key := ch.ChannelKeyForID(id)
	prefix, boundary, ok := quorumlog.NewImportedPrefix(string(key), [32]byte{1}, 100000)
	require.True(t, ok)
	records := []ch.Record{{ID: 9007199254740999, Index: 100001, Epoch: 1, FromUID: "original-user", ClientMsgNo: "original-client", ServerTimestampMS: 1700000001000, Payload: []byte{0, 255}, Protocol: ch.ProtocolFields{Expire: 3600, FramerFlags: 2, Topic: "topic", StreamNo: "stream", Timestamp: 1700000001}}}
	manifest, _, ok := ch.SealProposalManifest(ch.ProposalManifest{Version: ch.FullMessageProposalVersion, ChannelEpoch: 1, LeaderTerm: 1, FenceVersion: 1,
		CommandID: ch.CommandID{2}, BaseOffset: 100000, LastOffset: 100001, PreviousIndex: boundary.Index, PreviousTerm: boundary.LeaderTerm, PreviousDigest: boundary.Digest}, records)
	require.True(t, ok)
	router := &importRouter{servers: make(map[ch.NodeID]*replication.ExchangeServer)}
	runtimes := make(map[ch.NodeID]*replication.Runtime)
	adapters := make(map[ch.NodeID]replication.ReplicaStore)
	factories := make(map[ch.NodeID]*store.MessageDBFactory)
	for _, node := range []ch.NodeID{1, 2, 3} {
		f := store.NewMessageDBFactory(t.TempDir())
		t.Cleanup(func() { require.NoError(t, f.Close()) })
		factories[node] = f
		a, err := replication.NewStoreAdapter(replication.StoreAdapterConfig{Factory: f, MaxBatchItems: 256, MaxBatchBytes: 4 << 20})
		require.NoError(t, err)
		a = observedImportStore{ReplicaStore: a, node: node, router: router}
		adapters[node] = a
		if node != emptyNode {
			result := a.Replace(ctx, []replication.RecoveryReplacement{{ChannelKey: key, ChannelID: id, Proposals: []replication.RecoveryProposal{{Manifest: prefix}}, Committed: 100000}})
			require.NoError(t, result[0].Err)
			appended := a.Sync(ctx, []replication.Mutation{{ChannelKey: key, ChannelID: id, Manifest: manifest, Records: records, Committed: 100001}})
			require.NoError(t, appended[0].Err)
		}
		runtime, err := replication.NewRuntime(replication.RuntimeConfig{LocalNode: node, Store: a, Link: importLink{from: node, router: router}, MaxChannels: 4, LocalWorkers: 2, PeerWorkers: 2, PeerTargetFlight: 2, RepairWorkers: 1})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, runtime.Close(context.Background())) })
		runtimes[node] = runtime
		router.mu.Lock()
		router.servers[node] = runtime.ExchangeServer()
		router.mu.Unlock()
	}
	defer func() {
		if t.Failed() {
			router.mu.Lock()
			defer router.mu.Unlock()
			t.Logf("repair events: %v", router.events)
		}
	}()
	authority := replication.Authority{Key: key, ChannelID: id, ID: replication.AuthorityID{ChannelEpoch: 1, LeaderTerm: 2, FenceVersion: 2}, Leader: 1, Voters: []ch.NodeID{1, 2, 3}, WriteQuorum: 2}
	installed, err := runtimes[1].Log().Install(ctx, authority)
	require.NoError(t, err)
	require.GreaterOrEqual(t, installed.HW, uint64(100001))
	log, err := factories[1].ChannelStore(key, id)
	require.NoError(t, err)
	read, err := log.ReadCommitted(ctx, store.ReadCommittedRequest{FromSeq: 100001, Limit: 1, MaxBytes: 64 << 10})
	require.NoError(t, err)
	require.Len(t, read.Messages, 1)
	require.Equal(t, records[0].ID, read.Messages[0].MessageID)
	require.Equal(t, records[0].Protocol, read.Messages[0].Protocol)
	require.Equal(t, records[0].Payload, read.Messages[0].Payload)
	require.NoError(t, log.Close())
	proposal := replication.Proposal{Key: key, Expected: authority.ID, CommandID: ch.CommandID{3}, Records: []ch.Record{{ID: 9007199254741000, Epoch: 1, ServerTimestampMS: 1700000002000, FromUID: "new-user", ClientMsgNo: "new-client", Payload: []byte("new"), Protocol: ch.ProtocolFields{Topic: "new-topic"}, SizeBytes: 3 + ch.ProtocolFields{Topic: "new-topic"}.SizeBytes()}}}
	receipt, err := runtimes[1].Log().Commit(ctx, proposal)
	require.NoError(t, err)
	require.Greater(t, receipt.First, uint64(100001))
	require.True(t, runtimes[1].Log().Release(key, authority.ID))
	_, err = runtimes[1].Log().Install(ctx, authority)
	require.NoError(t, err)
	retry, err := runtimes[1].Log().Commit(ctx, proposal)
	require.NoError(t, err)
	require.Equal(t, receipt, retry)
	changed := proposal
	changed.Records = append([]ch.Record(nil), proposal.Records...)
	changed.Records[0].Protocol.Topic = "bad-topic"
	_, err = runtimes[1].Log().Commit(ctx, changed)
	require.ErrorIs(t, err, ch.ErrLogConflict, "idempotency must authenticate protocol fields too")
	require.Eventually(t, func() bool {
		loaded, e := adapters[3].Load(ctx, replication.LoadBatch{Items: []replication.LoadRequest{{ChannelKey: key, ChannelID: id}}})
		return e == nil && len(loaded.Items) == 1 && loaded.Items[0].Err == nil && loaded.Items[0].State.Prefix == prefix && loaded.Items[0].State.LEO >= receipt.Last
	}, 3*time.Second, 10*time.Millisecond, "trailing replica must recover imported boundary and original messages")
	loaded, err := adapters[1].Load(ctx, replication.LoadBatch{Items: []replication.LoadRequest{{ChannelKey: key, ChannelID: id}}})
	require.NoError(t, err)
	require.NoError(t, loaded.Items[0].Err)
	require.Equal(t, prefix, loaded.Items[0].State.Prefix)
}

type importRouter struct {
	mu      sync.Mutex
	servers map[ch.NodeID]*replication.ExchangeServer
	events  []string
}
type importLink struct {
	from   ch.NodeID
	router *importRouter
}

func (l importLink) Exchange(ctx context.Context, target ch.NodeID, batch replication.ExchangeBatch) (replication.ExchangeBatchResult, error) {
	b, err := replication.EncodeExchangeBatch(batch)
	if err != nil {
		return replication.ExchangeBatchResult{}, err
	}
	decoded, err := replication.DecodeExchangeBatch(b)
	if err != nil {
		return replication.ExchangeBatchResult{}, err
	}
	l.router.mu.Lock()
	server := l.router.servers[target]
	l.router.mu.Unlock()
	if server == nil {
		return replication.ExchangeBatchResult{}, ch.ErrNotReady
	}
	result, err := server.Handle(ctx, l.from, decoded)
	if err != nil {
		return replication.ExchangeBatchResult{}, err
	}
	b, err = replication.EncodeExchangeBatchResult(result)
	if err != nil {
		return replication.ExchangeBatchResult{}, err
	}
	return replication.DecodeExchangeBatchResult(b)
}

type observedImportStore struct {
	replication.ReplicaStore
	node   ch.NodeID
	router *importRouter
}

func (s observedImportStore) Fetch(ctx context.Context, q []replication.FetchRange) []replication.FetchRangeResult {
	r := s.ReplicaStore.Fetch(ctx, q)
	for i, result := range r {
		if result.Err != nil {
			s.log(fmt.Sprintf("fetch node=%d range=%d-%d err=%v", s.node, q[i].From, q[i].Through, result.Err))
		}
	}
	return r
}
func (s observedImportStore) Sync(ctx context.Context, q []replication.Mutation) []replication.MutationResult {
	r := s.ReplicaStore.Sync(ctx, q)
	for i, result := range r {
		s.log(fmt.Sprintf("sync node=%d range=%d-%d outcome=%d need=%d err=%v", s.node, q[i].Manifest.BaseOffset, q[i].Manifest.LastOffset, result.Outcome, result.NeedFrom, result.Err))
	}
	return r
}
func (s observedImportStore) LookupCommands(ctx context.Context, q []replication.CommandLookup) []replication.CommandLookupResult {
	r := s.ReplicaStore.(interface {
		LookupCommands(context.Context, []replication.CommandLookup) []replication.CommandLookupResult
	}).LookupCommands(ctx, q)
	for _, result := range r {
		s.log(fmt.Sprintf("lookup node=%d found=%v last=%d err=%v", s.node, result.Found, result.Manifest.LastOffset, result.Err))
	}
	return r
}
func (s observedImportStore) log(v string) {
	s.router.mu.Lock()
	defer s.router.mu.Unlock()
	if len(s.router.events) < 60 {
		s.router.events = append(s.router.events, v)
	}
}
