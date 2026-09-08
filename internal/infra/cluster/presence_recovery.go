package cluster

import (
	"context"
	"errors"
	accessnode "github.com/WuKongIM/WuKongIM/internal/access/node"
	authority "github.com/WuKongIM/WuKongIM/internal/runtime/presence"
	"github.com/WuKongIM/WuKongIM/internal/usecase/presence"
	"github.com/WuKongIM/WuKongIM/pkg/cluster/control"
	"github.com/WuKongIM/WuKongIM/pkg/transport"
	"slices"
	"sort"
	"time"
)

// PresenceRecoveryNode supplies exact routing and current membership evidence.
type PresenceRecoveryNode interface {
	PresenceNode
	LocalControllerSnapshot(context.Context) (control.Snapshot, error)
}

// PresenceRecoveryOwners reads indexed owner projections only during UID
// reconstruction. One node-wide gate bounds network reads across all hash slots.
type PresenceRecoveryOwners struct {
	node      PresenceRecoveryNode
	local     presence.OwnerRouteReader
	remote    *accessnode.Client
	admission chan struct{}
}

// NewPresenceRecoveryOwners shares bounded owner-read admission across UID authorities.
func NewPresenceRecoveryOwners(node PresenceRecoveryNode, local presence.OwnerRouteReader) *PresenceRecoveryOwners {
	return &PresenceRecoveryOwners{node: node, local: local, remote: accessnode.NewClient(node), admission: make(chan struct{}, 4)}
}

// ValidatePresenceTarget checks the distributed identity; local diagnostic
// revision/epoch counters are never compared across nodes.
func ValidatePresenceTarget(node PresenceNode, target presence.RouteTarget) error {
	route, err := node.RouteHashSlot(target.HashSlot)
	if err != nil {
		return mapPresenceRouteError(err)
	}
	current := routeTargetFromClusterRoute(route)
	if current.HashSlot != target.HashSlot || current.SlotID != target.SlotID || current.LeaderNodeID != target.LeaderNodeID || current.LeaderTerm != target.LeaderTerm || current.ConfigEpoch != target.ConfigEpoch {
		return authority.ErrNotLeader
	}
	return nil
}

// RecoverRoutes requires complete current-owner evidence before publishing a UID page.
func (r *PresenceRecoveryOwners) RecoverRoutes(ctx context.Context, target presence.RouteTarget, uids []string) ([]presence.Route, error) {
	if r == nil || r.node == nil || r.local == nil || len(uids) > authority.RecoveryBatchSize {
		return nil, authority.ErrRouteNotReady
	}
	select {
	case r.admission <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-r.admission }()
	if err := ValidatePresenceTarget(r.node, target); err != nil {
		return nil, err
	}
	before, err := r.node.LocalControllerSnapshot(ctx)
	if err != nil {
		return nil, authority.ErrRouteNotReady
	}
	nodes := recoveryOwnerNodes(before)
	if len(nodes) == 0 || len(nodes) > 256 {
		return nil, authority.ErrRouteNotReady
	}
	allowed := make(map[string]struct{}, len(uids))
	for _, uid := range uids {
		allowed[uid] = struct{}{}
	}
	var routes []presence.Route
	for _, node := range nodes {
		readCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		var snapshot presence.OwnerRouteSnapshot
		var readErr error
		if node.NodeID == r.node.NodeID() {
			snapshot, readErr = r.local.ReadOwnerRoutes(readCtx, target, uids)
		} else {
			snapshot, readErr = r.remote.ReadOwnerRoutes(readCtx, node.NodeID, target, uids)
		}
		cancel()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if readErr != nil {
			// Query stale members too: a reachable ingress may still own sessions.
			// Only unavailable members with expired/down Controller health may be omitted.
			if (node.Health.Freshness == control.NodeHealthStale || node.Health.Status == control.NodeDown) && (errors.Is(readErr, context.DeadlineExceeded) || errors.Is(readErr, transport.ErrTimeout) || errors.Is(readErr, transport.ErrDialFailed) || errors.Is(readErr, transport.ErrNodeNotFound)) {
				continue
			}
			return nil, authority.ErrRouteNotReady
		}
		if snapshot.OwnerNodeID != node.NodeID || snapshot.OwnerBootID == 0 {
			return nil, authority.ErrStaleRoute
		}
		for _, route := range snapshot.Routes {
			if _, ok := allowed[route.UID]; !ok || route.OwnerNodeID != node.NodeID || route.OwnerBootID != snapshot.OwnerBootID || route.SessionID == 0 {
				return nil, authority.ErrStaleRoute
			}
		}
		if len(routes)+len(snapshot.Routes) > 4096 {
			return nil, authority.ErrRouteNotReady
		}
		routes = append(routes, snapshot.Routes...)
	}
	after, err := r.node.LocalControllerSnapshot(ctx)
	if err != nil {
		return nil, authority.ErrRouteNotReady
	}
	// A membership change requires another complete owner round, even if the
	// queried hash slot retained its leader term.
	if !sameRecoveryOwners(nodes, recoveryOwnerNodes(after)) {
		return nil, authority.ErrRouteNotReady
	}
	if err := ValidatePresenceTarget(r.node, target); err != nil {
		return nil, err
	}
	return routes, nil
}

func recoveryOwnerNodes(snapshot control.Snapshot) []control.Node {
	nodes := make([]control.Node, 0, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		if node.NodeID == 0 || node.JoinState == control.NodeJoinStateRemoved || node.JoinState == control.NodeJoinStateJoining || !slices.Contains(node.Roles, control.RoleData) {
			continue
		}
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].NodeID < nodes[j].NodeID })
	return nodes
}

func sameRecoveryOwners(a, b []control.Node) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].NodeID != b[i].NodeID || a[i].Addr != b[i].Addr || a[i].JoinState != b[i].JoinState || a[i].Health.Freshness != b[i].Health.Freshness || a[i].Health.Status != b[i].Health.Status {
			return false
		}
	}
	return true
}
