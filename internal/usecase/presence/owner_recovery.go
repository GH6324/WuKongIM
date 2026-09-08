package presence

import (
	"context"

	authority "github.com/WuKongIM/WuKongIM/internal/runtime/presence"
)

// OwnerRouteSnapshot binds an active-route read, including an empty read, to
// the answering process generation. Concrete sessions never leave the owner.
type OwnerRouteSnapshot struct {
	// OwnerNodeID is the answering gateway owner.
	OwnerNodeID uint64
	// OwnerBootID is the process generation that owns every returned route.
	OwnerBootID uint64
	// Routes contains only active projections; an empty complete snapshot is valid.
	Routes []Route
}

// OwnerRouteReader reads a bounded exact-target page from one gateway owner.
type OwnerRouteReader interface {
	ReadOwnerRoutes(context.Context, RouteTarget, []string) (OwnerRouteSnapshot, error)
}

// ActiveRouteRegistry reads owner-indexed pages without exposing concrete sessions.
type ActiveRouteRegistry interface {
	ActiveRoutesByUIDs([]string, int) ([]OwnerRoute, bool)
}

// OwnerRecovery exposes active owner-local projections after checking the
// caller's authority against the owner's current route publication.
type OwnerRecovery struct {
	local          ActiveRouteRegistry
	nodeID, bootID uint64
	validate       func(RouteTarget) error
}

// NewOwnerRecovery binds an owner registry and its process identity to route validation.
func NewOwnerRecovery(local ActiveRouteRegistry, nodeID, bootID uint64, validate func(RouteTarget) error) *OwnerRecovery {
	return &OwnerRecovery{local: local, nodeID: nodeID, bootID: bootID, validate: validate}
}

// ReadOwnerRoutes rechecks the current authority around a bounded active-route read.
func (r *OwnerRecovery) ReadOwnerRoutes(ctx context.Context, target RouteTarget, uids []string) (OwnerRouteSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return OwnerRouteSnapshot{}, err
	}
	if r == nil || r.local == nil || r.validate == nil || r.nodeID == 0 || r.bootID == 0 || len(uids) > authority.RecoveryBatchSize {
		return OwnerRouteSnapshot{}, authority.ErrRouteNotReady
	}
	if err := r.validate(target); err != nil {
		return OwnerRouteSnapshot{}, err
	}
	rows, complete := r.local.ActiveRoutesByUIDs(uids, 4096)
	if !complete {
		return OwnerRouteSnapshot{}, authority.ErrRouteNotReady
	}
	result := OwnerRouteSnapshot{OwnerNodeID: r.nodeID, OwnerBootID: r.bootID, Routes: make([]Route, 0, len(rows))}
	for _, row := range rows {
		if row.OwnerNodeID != r.nodeID || row.OwnerBootID != r.bootID || row.HashSlot != target.HashSlot {
			return OwnerRouteSnapshot{}, authority.ErrStaleRoute
		}
		result.Routes = append(result.Routes, routeFromOwnerRoute(row))
	}
	if err := r.validate(target); err != nil {
		return OwnerRouteSnapshot{}, err
	}
	return result, nil
}
