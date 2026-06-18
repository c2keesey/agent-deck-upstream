package ui

import (
	"testing"
	"time"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// Reproduction of the live conductor state (2026-06-18): idle/ready sessions
// ranked 1,2,3,4,5,7 plus unset ones. Ranks 4/5/7 are past the OLD 1..3 cap and
// are exactly what tripped a stale binary, where the unset sentinel was
// MaxPriority+1 == 4: an unset session and a rank-4 session collided, and rank
// 5/7 sorted BEHIND unset. This test pins the NEW (math.MaxInt) behavior end to
// end: attentionSortedSessions order, the Ctrl+E cursor cycle, and the ⚡ nudge.
func TestAttentionCycle_LiveConductorRanks_FullCycle(t *testing.T) {
	base := time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC)
	mk := func(id string, prio int) *session.Instance {
		// Distinct CreatedAt so the longest-waiting tiebreak is deterministic
		// within an (unset) tier; higher prio => arbitrary, tiebreak only bites
		// among equal ranks.
		return readyInstance(id, session.StatusIdle, prio, base.Add(time.Duration(prio)*time.Minute))
	}
	// Mixed input order on purpose — sort must impose the rank order.
	insts := []*session.Instance{
		readyInstance("unset-a", session.StatusIdle, 0, base.Add(-2*time.Hour)), // oldest unset
		mk("p7", 7),
		mk("p3", 3),
		readyInstance("unset-b", session.StatusWaiting, 0, base.Add(-1*time.Hour)),
		mk("p1", 1),
		mk("p5", 5),
		mk("p2", 2),
		mk("p4", 4),
	}
	items := make([]session.Item, len(insts))
	for i, in := range insts {
		items[i] = session.Item{Type: session.ItemTypeSession, Session: in, Level: 0}
	}
	h := newTestHomeWithItems(120, 40, items)
	// newTestHomeWithItems starts NewHome's background livePipeReconciler, which
	// reads h.instances under instancesMu; assign under the same lock so the
	// race detector stays clean (the product reader is correctly synchronized).
	h.instancesMu.Lock()
	h.instances = insts
	h.instancesMu.Unlock()

	// 1) Sort order: strict ranks ascending, then unset (oldest-ready first).
	got := h.attentionSortedSessions()
	gotIDs := make([]string, len(got))
	for i, in := range got {
		gotIDs[i] = in.ID
	}
	want := []string{"p1", "p2", "p3", "p4", "p5", "p7", "unset-a", "unset-b"}
	if len(gotIDs) != len(want) {
		t.Fatalf("sorted ready set = %v, want %v", gotIDs, want)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Fatalf("sort order = %v, want %v (ranks 4/5/7 must sort ahead of unset)", gotIDs, want)
		}
	}

	// 2) Ctrl+E cycles the cursor through that exact order, then wraps.
	cycle := append(append([]string{}, want...), want[0]) // full lap + wrap to top
	for i, expID := range cycle {
		if got := pressCtrlE(t, h); got != expID {
			t.Fatalf("Ctrl+E press #%d landed on %q, want %q (full cycle: %v)", i+1, got, expID, cycle)
		}
	}

	// 3) ⚡ nudge: attached to an unset session, the highest-rank ready one (p1)
	// must be advertised.
	nudge := h.attentionNudgeText("unset-a", insts)
	if want := "⚡ P1 p1 ready — ^E"; nudge != want {
		t.Fatalf("nudge from unset-attached = %q, want %q", nudge, want)
	}
	// And attached to p2, the nudge must point at p1 (strictly higher), not a
	// higher-numbered (lower) rank.
	if nudge := h.attentionNudgeText("p2", insts); nudge != "⚡ P1 p1 ready — ^E" {
		t.Fatalf("nudge from p2 = %q, want the rank-1 session", nudge)
	}
	// Attached to p1 (top), no nudge — nothing is strictly higher.
	if nudge := h.attentionNudgeText("p1", insts); nudge != "" {
		t.Fatalf("attached to top rank should produce no nudge, got %q", nudge)
	}
}
