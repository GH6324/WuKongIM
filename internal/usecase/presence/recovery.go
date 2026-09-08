package presence

import (
	"context"

	authority "github.com/WuKongIM/WuKongIM/internal/runtime/presence"
)

// RecoveryDirectory owns reconstructed UID proofs under exact authority fences.
type RecoveryDirectory interface {
	RecoveryUIDs(RouteTarget, []string) ([]string, error)
	InstallRecoveredRoutes(RouteTarget, []string, []Route) error
	EndpointsByUIDs(RouteTarget, []string) ([]Route, error)
}

// RecoveryOwners returns a complete bounded read from all current gateway owners.
// Partial/unavailable owner evidence must be returned as an error.
type RecoveryOwners interface {
	RecoverRoutes(context.Context, RouteTarget, []string) ([]Route, error)
}

// AuthorityRecovery reconstructs requested UIDs with bounded owner queries.
// Fixed admission lanes coalesce overlapping lookups without per-UID goroutines
// or an unbounded in-flight map. Directory ownership fences every completion.
type AuthorityRecovery struct {
	directory RecoveryDirectory
	owners    RecoveryOwners
	lanes     [32]chan struct{}
}

// NewAuthorityRecovery allocates fixed admission lanes for owner reconstruction.
func NewAuthorityRecovery(directory RecoveryDirectory, owners RecoveryOwners) *AuthorityRecovery {
	r := &AuthorityRecovery{directory: directory, owners: owners}
	for i := range r.lanes {
		r.lanes[i] = make(chan struct{}, 1)
	}
	return r
}

// Endpoints returns complete pages, retaining caller cancellation while waiting
// for another reconstruction of the same hash slot to publish its proof.
func (r *AuthorityRecovery) Endpoints(ctx context.Context, target RouteTarget, uids []string) ([]Route, error) {
	if r == nil || r.directory == nil || r.owners == nil {
		return nil, authority.ErrRouteNotReady
	}
	lane := r.lanes[int(target.HashSlot)%len(r.lanes)]
	select {
	case lane <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-lane }()
	var routes []Route
	for start := 0; start < len(uids); start += authority.RecoveryBatchSize {
		end := min(start+authority.RecoveryBatchSize, len(uids))
		page := uids[start:end]
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		missing, err := r.directory.RecoveryUIDs(target, page)
		if err != nil {
			return nil, err
		}
		if len(missing) != 0 {
			recovered, err := r.owners.RecoverRoutes(ctx, target, missing)
			if err != nil {
				return nil, err
			}
			if err := r.directory.InstallRecoveredRoutes(target, missing, recovered); err != nil {
				return nil, err
			}
		}
		got, err := r.directory.EndpointsByUIDs(target, page)
		if err != nil {
			return nil, err
		}
		routes = append(routes, got...)
	}
	return routes, nil
}
