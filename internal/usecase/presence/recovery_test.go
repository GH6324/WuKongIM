package presence

import (
	"context"
	"errors"
	"testing"

	authority "github.com/WuKongIM/WuKongIM/internal/runtime/presence"
)

type recoveryOwnersFunc func(context.Context, RouteTarget, []string) ([]Route, error)

func (f recoveryOwnersFunc) RecoverRoutes(ctx context.Context, target RouteTarget, uids []string) ([]Route, error) {
	return f(ctx, target, uids)
}

func TestAuthorityRecoveryRequiresCompleteOwnersAndRejectsOldCompletion(t *testing.T) {
	directory := authority.NewDirectory(authority.DirectoryOptions{LocalNodeID: 1, RequireRecovery: true})
	target := RouteTarget{HashSlot: 1, SlotID: 1, LeaderNodeID: 1, LeaderTerm: 1}
	directory.BecomeAuthority(target)
	unavailable := true
	advance := false
	calls := 0
	recovery := NewAuthorityRecovery(directory, recoveryOwnersFunc(func(ctx context.Context, tgt RouteTarget, uids []string) ([]Route, error) {
		calls++
		if len(uids) > authority.RecoveryBatchSize {
			t.Fatal("unbounded recovery page")
		}
		if unavailable {
			return nil, authority.ErrRouteNotReady
		}
		if advance {
			next := tgt
			next.LeaderTerm++
			directory.BecomeAuthority(next)
		}
		return nil, nil
	}))
	if _, err := recovery.Endpoints(context.Background(), target, []string{"offline"}); !errors.Is(err, authority.ErrRouteNotReady) {
		t.Fatalf("partial owners=%v", err)
	}
	if _, err := directory.EndpointsByUID(target, "offline"); !errors.Is(err, authority.ErrRouteNotReady) {
		t.Fatalf("partial proof became offline: %v", err)
	}
	unavailable = false
	for i := 0; i < 2; i++ {
		if rows, err := recovery.Endpoints(context.Background(), target, []string{"offline"}); err != nil || len(rows) != 0 {
			t.Fatalf("confirmed offline=%+v err=%v", rows, err)
		}
	}
	if calls != 2 {
		t.Fatalf("cached offline was requeried: calls=%d", calls)
	}
	advance = true
	if _, err := recovery.Endpoints(context.Background(), target, []string{"next"}); !errors.Is(err, authority.ErrNotLeader) {
		t.Fatalf("old completion=%v", err)
	}
	next := target
	next.LeaderTerm++
	if _, err := directory.EndpointsByUID(next, "next"); !errors.Is(err, authority.ErrRouteNotReady) {
		t.Fatalf("old completion made new authority ready: %v", err)
	}
}

func TestAuthorityRecoveryCancellationDoesNotWaitForBusyLane(t *testing.T) {
	directory := authority.NewDirectory(authority.DirectoryOptions{LocalNodeID: 1, RequireRecovery: true})
	target := RouteTarget{HashSlot: 1, SlotID: 1, LeaderNodeID: 1, LeaderTerm: 1}
	directory.BecomeAuthority(target)
	recovery := NewAuthorityRecovery(directory, recoveryOwnersFunc(func(context.Context, RouteTarget, []string) ([]Route, error) {
		t.Fatal("canceled recovery called owner")
		return nil, nil
	}))
	recovery.lanes[1] <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := recovery.Endpoints(ctx, target, []string{"bob"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("busy lane cancel=%v", err)
	}
	<-recovery.lanes[1]
}
