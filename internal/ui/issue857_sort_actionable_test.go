package ui

// Coverage for the in-group session sort.
//
// Upstream #857 sorts sessions by "most recently actionable" within a group so
// active sessions don't get buried among parked ones. The FORK changes the
// group tree's DEFAULT in-group order to a STABLE persisted-Order sort
// (SortInstancesByOrder): the tree is rebuilt on every reload, so an actionable
// sort re-runs constantly and makes rows jump out from under the cursor right
// before a click. The actionable surfacing is still available on demand via the
// active-on-top view mode ('t'), which floats active sessions at render time
// without disturbing the stable base order.
//
// So these tests now cover two things:
//   - SortInstancesByActionable (the function) still produces the #857 order —
//     it backs the active-on-top view mode.
//   - NewGroupTree (the fork default) produces stable persisted-Order order.

import (
	"testing"
	"time"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// TestSortByActionable_RegressionFor857 verifies the actionable sort FUNCTION
// still surfaces error/waiting/running ahead of idle/stopped (#857). The Order
// values are reversed vs. the desired actionable order so an Order-only sort
// would flip at least four positions.
func TestSortByActionable_RegressionFor857(t *testing.T) {
	now := time.Now()
	instances := []*session.Instance{
		{ID: "run", Title: "running-sess", GroupPath: "g", Order: 1, Status: session.StatusRunning, LastAccessedAt: now.Add(-1 * time.Minute)},
		{ID: "wait", Title: "waiting-sess", GroupPath: "g", Order: 2, Status: session.StatusWaiting, LastAccessedAt: now.Add(-2 * time.Minute)},
		{ID: "err", Title: "error-sess", GroupPath: "g", Order: 3, Status: session.StatusError, LastAccessedAt: now.Add(-3 * time.Minute)},
		{ID: "idle", Title: "idle-sess", GroupPath: "g", Order: 4, Status: session.StatusIdle, LastAccessedAt: now.Add(-4 * time.Minute)},
		{ID: "stop", Title: "stopped-sess", GroupPath: "g", Order: 5, Status: session.StatusStopped, LastAccessedAt: now.Add(-5 * time.Minute)},
	}

	session.SortInstancesByActionable(instances)

	wantIDs := []string{"err", "wait", "run", "idle", "stop"}
	gotIDs := make([]string, len(instances))
	for i, s := range instances {
		gotIDs[i] = s.ID
	}
	for i, want := range wantIDs {
		if gotIDs[i] != want {
			t.Errorf("position %d: want %q (status %s), got %q\n  full order: got=%v want=%v",
				i, want, statusForID(instances, want), gotIDs[i], gotIDs, wantIDs)
		}
	}
}

// TestSortByActionable_TimestampTieBreak verifies the function's secondary sort:
// within one status the recently-accessed session surfaces first.
func TestSortByActionable_TimestampTieBreak(t *testing.T) {
	now := time.Now()
	instances := []*session.Instance{
		{ID: "old-wait", Title: "old", GroupPath: "g", Order: 1, Status: session.StatusWaiting, LastAccessedAt: now.Add(-3 * time.Hour)},
		{ID: "new-wait", Title: "new", GroupPath: "g", Order: 2, Status: session.StatusWaiting, LastAccessedAt: now.Add(-5 * time.Minute)},
		{ID: "old-run", Title: "old-r", GroupPath: "g", Order: 3, Status: session.StatusRunning, LastAccessedAt: now.Add(-1 * time.Hour)},
		{ID: "new-run", Title: "new-r", GroupPath: "g", Order: 4, Status: session.StatusRunning, LastAccessedAt: now.Add(-1 * time.Minute)},
	}

	session.SortInstancesByActionable(instances)

	want := []string{"new-wait", "old-wait", "new-run", "old-run"}
	got := make([]string, len(instances))
	for i, s := range instances {
		got[i] = s.ID
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: want %q, got %q (full: got=%v want=%v)", i, want[i], got[i], got, want)
		}
	}
}

// TestNewGroupTree_StableOrder_ForkDefault pins the fork's default: the group
// tree orders sessions by persisted Order REGARDLESS of status, so rows do not
// jump as statuses change between the frequent tree rebuilds. Status values are
// arranged so an actionable sort would reorder them; a stable Order sort must
// keep the persisted 1..5 order.
func TestNewGroupTree_StableOrder_ForkDefault(t *testing.T) {
	now := time.Now()
	instances := []*session.Instance{
		{ID: "a", Title: "a", GroupPath: "g", Order: 1, Status: session.StatusIdle, LastAccessedAt: now.Add(-5 * time.Minute)},
		{ID: "b", Title: "b", GroupPath: "g", Order: 2, Status: session.StatusError, LastAccessedAt: now.Add(-1 * time.Minute)},
		{ID: "c", Title: "c", GroupPath: "g", Order: 3, Status: session.StatusWaiting, LastAccessedAt: now.Add(-2 * time.Minute)},
		{ID: "d", Title: "d", GroupPath: "g", Order: 4, Status: session.StatusRunning, LastAccessedAt: now},
		{ID: "e", Title: "e", GroupPath: "g", Order: 5, Status: session.StatusStopped, LastAccessedAt: now.Add(-9 * time.Minute)},
	}

	tree := session.NewGroupTree(instances)
	group := tree.Groups["g"]
	if group == nil {
		t.Fatalf("group %q not in tree", "g")
	}

	want := []string{"a", "b", "c", "d", "e"} // persisted Order, untouched by status
	got := make([]string, len(group.Sessions))
	for i, s := range group.Sessions {
		got[i] = s.ID
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: want %q, got %q (full: got=%v want=%v) — in-group order must be stable", i, want[i], got[i], got, want)
		}
	}
}

func statusForID(insts []*session.Instance, id string) session.Status {
	for _, i := range insts {
		if i.ID == id {
			return i.Status
		}
	}
	return ""
}
