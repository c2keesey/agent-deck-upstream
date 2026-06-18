package ui

import (
	"testing"
	"time"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// Reproduces Chris's live default-profile structure: ranked idle sessions spread
// across several groups (the conductor assigns ranks to sessions that live in
// maia/investigation, maia/active, maia/evals), built through the REAL groupTree
// + rebuildFlatItems path — so flatItems interleaves group headers, tree levels,
// and sessions exactly like the running TUI. Then it drives ^E and asserts the
// cursor actually advances through every ranked session in rank order.
//
// The earlier flat (group-less) repro passed; this one exercises the grouped
// flat-list the real TUI builds, which is the structural difference between the
// unit tests and what Chris sees.
func TestAttentionCycle_GroupedLikeLive_DrivesCursor(t *testing.T) {
	base := time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC)
	mk := func(id, group string, prio int) *session.Instance {
		in := session.NewInstanceWithTool(id, "/tmp/"+id, "claude")
		in.ID = id
		in.GroupPath = group
		in.Status = session.StatusIdle
		in.Priority = prio
		in.LastAccessedAt = base.Add(time.Duration(prio) * time.Minute)
		return in
	}
	instances := []*session.Instance{
		mk("p1", "maia/investigation", 1),
		mk("p2", "maia/active", 2),
		mk("p3", "maia/investigation", 3),
		mk("p4", "maia/evals", 4),
		mk("p5", "maia/evals", 5),
		mk("p7", "maia/active", 7),
		mk("u1", "c2k", 0),
		mk("u2", "agent-deck", 0),
	}

	h := NewHome()
	h.width = 120
	h.height = 40
	h.initialLoading = false
	h.instancesMu.Lock()
	h.instances = instances
	h.instanceByID = make(map[string]*session.Instance, len(instances))
	for _, in := range instances {
		h.instanceByID[in.ID] = in
	}
	h.instancesMu.Unlock()
	h.groupTree = session.NewGroupTree(instances)
	h.rebuildFlatItems()

	// Every ranked session must be present in the (all-expanded) flat list.
	inFlat := map[string]bool{}
	for _, it := range h.flatItems {
		if it.Type == session.ItemTypeSession && it.Session != nil {
			inFlat[it.Session.ID] = true
		}
	}
	for _, id := range []string{"p1", "p2", "p3", "p4", "p5", "p7", "u1", "u2"} {
		if !inFlat[id] {
			t.Fatalf("session %q missing from flatItems (group not expanded?) — flat=%v", id, inFlat)
		}
	}

	// ^E must walk the cursor through the ranked sessions in rank order, then
	// the unset ones, then wrap.
	want := []string{"p1", "p2", "p3", "p4", "p5", "p7", "u1", "u2", "p1"}
	for i, expID := range want {
		got := pressCtrlE(t, h)
		if got != expID {
			t.Fatalf("^E press #%d landed on %q, want %q (cursor=%d, flatItems=%d)",
				i+1, got, expID, h.cursor, len(h.flatItems))
		}
	}
}
