package cluster

import (
	"context"
	"errors"
	"testing"

	accessnode "github.com/WuKongIM/WuKongIM/internal/access/node"
	"github.com/WuKongIM/WuKongIM/internal/runtime/online"
	authority "github.com/WuKongIM/WuKongIM/internal/runtime/presence"
	"github.com/WuKongIM/WuKongIM/internal/usecase/presence"
	"github.com/WuKongIM/WuKongIM/pkg/cluster"
	"github.com/WuKongIM/WuKongIM/pkg/cluster/control"
)

type recoveryCluster struct {
	*fakePresenceCluster
	snapshot control.Snapshot
}

func (n *recoveryCluster) LocalControllerSnapshot(context.Context) (control.Snapshot, error) {
	return n.snapshot, nil
}

type recoveryReaderFunc func(context.Context, presence.RouteTarget, []string) (presence.OwnerRouteSnapshot, error)

func (f recoveryReaderFunc) ReadOwnerRoutes(ctx context.Context, t presence.RouteTarget, u []string) (presence.OwnerRouteSnapshot, error) {
	return f(ctx, t, u)
}

func TestRecoveryReadsSurvivingIngressAndRejectsStaleOwnerGeneration(t *testing.T) {
	target := presence.RouteTarget{HashSlot: 101, SlotID: 5, LeaderNodeID: 1, LeaderTerm: 4, ConfigEpoch: 1}
	node := &recoveryCluster{fakePresenceCluster: &fakePresenceCluster{nodeID: 1, route: cluster.Route{HashSlot: 101, SlotID: 5, Leader: 1, LeaderTerm: 4, ConfigEpoch: 1}}}
	for _, id := range []uint64{1, 2} {
		node.snapshot.Nodes = append(node.snapshot.Nodes, control.Node{NodeID: id, Roles: []control.Role{control.RoleData}, JoinState: control.NodeJoinStateActive, Health: control.NodeHealth{Status: control.NodeAlive, Freshness: control.NodeHealthFresh, RuntimeReady: true}})
	}
	registry := online.NewRegistry(online.RegistryOptions{})
	route := online.OwnerRoute{UID: "bob", HashSlot: 101, OwnerNodeID: 2, OwnerBootID: 22, OwnerSeq: 1, SessionID: 8}
	if err := registry.RegisterPending(online.LocalSession{Route: route}); err != nil {
		t.Fatal(err)
	}
	if err := registry.MarkActive(8); err != nil {
		t.Fatal(err)
	}
	owner := presence.NewOwnerRecovery(registry, 2, 22, func(got presence.RouteTarget) error {
		if got != target {
			return authority.ErrNotLeader
		}
		return nil
	})
	adapter := accessnode.New(accessnode.Options{OwnerRoutes: owner})
	node.rpcByNode = map[uint64]cluster.NodeRPCHandler{2: presenceOwnerRPCHandler{adapter: adapter}}
	local := recoveryReaderFunc(func(context.Context, presence.RouteTarget, []string) (presence.OwnerRouteSnapshot, error) {
		return presence.OwnerRouteSnapshot{OwnerNodeID: 1, OwnerBootID: 11}, nil
	})
	source := NewPresenceRecoveryOwners(node, local)
	directory := authority.NewDirectory(authority.DirectoryOptions{LocalNodeID: 1, RequireRecovery: true})
	directory.BecomeAuthority(target)
	authorityAdapter := NewPresenceDirectoryAuthority(directory)
	authorityAdapter.SetRecovery(presence.NewAuthorityRecovery(directory, source))
	results := authorityAdapter.EndpointsByTargets(context.Background(), []presence.EndpointLookupGroup{{Target: target, UIDs: []string{"bob", "offline"}}})
	if len(results) != 1 || results[0].Err != nil || len(results[0].Routes) != 1 || results[0].Routes[0].OwnerBootID != 22 {
		t.Fatalf("reconstructed=%+v", results)
	}
	// A response advertising the current boot may not smuggle a previous-boot route.
	forged := recoveryReaderFunc(func(context.Context, presence.RouteTarget, []string) (presence.OwnerRouteSnapshot, error) {
		return presence.OwnerRouteSnapshot{OwnerNodeID: 2, OwnerBootID: 23, Routes: results[0].Routes}, nil
	})
	node.rpcByNode[2] = presenceOwnerRPCHandler{adapter: accessnode.New(accessnode.Options{OwnerRoutes: forged})}
	if _, err := source.RecoverRoutes(context.Background(), target, []string{"bob"}); !errors.Is(err, authority.ErrStaleRoute) {
		t.Fatalf("old generation proof=%v", err)
	}
}
