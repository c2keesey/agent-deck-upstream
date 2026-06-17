package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

func TestAttentionCycleDefaultBinding(t *testing.T) {
	bindings := resolveHotkeys(nil)
	if got := bindings[hotkeyAttentionCycle]; got != "ctrl+e" {
		t.Fatalf("attention_cycle default binding = %q, want ctrl+e", got)
	}
}

func TestAttentionCycleOverride(t *testing.T) {
	bindings := resolveHotkeys(map[string]string{
		"attention_cycle": "ctrl+p",
	})
	if got := bindings[hotkeyAttentionCycle]; got != "ctrl+p" {
		t.Fatalf("attention_cycle override = %q, want ctrl+p", got)
	}
}

func TestAttentionCycleUnbind(t *testing.T) {
	bindings := resolveHotkeys(map[string]string{
		"attention_cycle": "",
	})
	if _, ok := bindings[hotkeyAttentionCycle]; ok {
		t.Fatalf("attention_cycle should be unbound")
	}
}

func TestAttentionSwitchByte(t *testing.T) {
	// Ctrl+E = byte 5 (5th letter, ctrl+a = 1).
	if got := DetachByteFromBinding("ctrl+e"); got != 5 {
		t.Fatalf("DetachByteFromBinding(ctrl+e) = %d, want 5", got)
	}
}

// readyInstance builds an instance in a ready (waiting/idle) state with a
// priority and a creation time controlling the longest-waiting tiebreak.
func readyInstance(id string, status session.Status, prio int, created time.Time) *session.Instance {
	inst := session.NewInstance(id, "/tmp/"+id)
	inst.ID = id
	inst.Status = status
	inst.Priority = prio
	inst.CreatedAt = created
	return inst
}

// pressCtrlE drives the live Ctrl+E handler the way the TUI does and returns
// the session the cursor lands on (or "" if none).
func pressCtrlE(t *testing.T, h *Home) string {
	t.Helper()
	model, _ := h.handleMainKey(tea.KeyMsg{Type: tea.KeyCtrlE})
	hh := model.(*Home)
	if sel := hh.getSelectedSession(); sel != nil {
		return sel.ID
	}
	return ""
}

// TestAttentionCycle_RecomputesWhenHigherBecomesReady pins the fix for "ctrl+e
// doesn't cycle to P2 — should go to the next most important when the higher one
// is busy". The ready set must be recomputed on EVERY press, not frozen on the
// first one: a session that just went busy must drop out, and one that just
// became ready must drop in, immediately. With the old frozen-snapshot handler
// the third press below stayed on the stale P2 instead of reaching the freshly
// ready P1.
func TestAttentionCycle_RecomputesWhenHigherBecomesReady(t *testing.T) {
	base := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)
	p1 := readyInstance("p1", session.StatusRunning, 1, base) // busy → excluded at first
	p2 := readyInstance("p2", session.StatusWaiting, 2, base)
	p3 := readyInstance("p3", session.StatusIdle, 3, base)
	items := []session.Item{
		{Type: session.ItemTypeSession, Session: p1, Level: 0},
		{Type: session.ItemTypeSession, Session: p2, Level: 0},
		{Type: session.ItemTypeSession, Session: p3, Level: 0},
	}
	h := newTestHomeWithItems(100, 30, items)
	h.instances = []*session.Instance{p1, p2, p3}

	// P1 is running (busy): first press lands on the top-priority READY session.
	if got := pressCtrlE(t, h); got != "p2" {
		t.Fatalf("first Ctrl+E with P1 busy landed on %q, want p2 (next most important ready)", got)
	}
	// Repeat press advances to the next ready session.
	if got := pressCtrlE(t, h); got != "p3" {
		t.Fatalf("second Ctrl+E landed on %q, want p3", got)
	}
	// P1 finishes and becomes ready — now the highest priority ready session.
	p1.Status = session.StatusWaiting
	if got := pressCtrlE(t, h); got != "p1" {
		t.Fatalf("third Ctrl+E after P1 became ready landed on %q, want p1 "+
			"(ready set must be recomputed, not frozen)", got)
	}
}

func TestAttentionSortedSessions_PriorityOrderAndReadyFilter(t *testing.T) {
	base := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)
	h := &Home{}
	h.instances = []*session.Instance{
		readyInstance("running-hi", session.StatusRunning, 1, base), // excluded: running
		readyInstance("idle-unset", session.StatusIdle, 0, base),    // unset → last
		readyInstance("wait-p2", session.StatusWaiting, 2, base),
		readyInstance("wait-p1", session.StatusWaiting, 1, base),
		readyInstance("idle-p1", session.StatusIdle, 1, base.Add(-time.Hour)), // older P1 → first
	}

	got := h.attentionSortedSessions()
	gotIDs := make([]string, len(got))
	for i, inst := range got {
		gotIDs[i] = inst.ID
	}

	want := []string{"idle-p1", "wait-p1", "wait-p2", "idle-unset"}
	if len(gotIDs) != len(want) {
		t.Fatalf("attentionSortedSessions returned %v, want %v", gotIDs, want)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Fatalf("attentionSortedSessions order = %v, want %v", gotIDs, want)
		}
	}
}

func TestAttentionSortedSessions_LongestWaitingTiebreak(t *testing.T) {
	base := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)
	h := &Home{}
	// Same priority tier; the one ready longest (older CreatedAt) ranks first.
	h.instances = []*session.Instance{
		readyInstance("newer", session.StatusWaiting, 1, base),
		readyInstance("older", session.StatusWaiting, 1, base.Add(-2*time.Hour)),
	}
	got := h.attentionSortedSessions()
	if len(got) != 2 || got[0].ID != "older" {
		t.Fatalf("expected older P1 first, got %+v", got)
	}
}

func TestAttentionNudge_HigherPriorityReadyWhileAttached(t *testing.T) {
	base := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)
	h := &Home{}
	attached := readyInstance("attached-p2", session.StatusRunning, 2, base)
	p1ready := readyInstance("urgent-p1", session.StatusWaiting, 1, base)
	insts := []*session.Instance{attached, p1ready}

	got := h.attentionNudgeText("attached-p2", insts)
	want := "⚡ P1 urgent-p1 ready — ^E"
	if got != want {
		t.Fatalf("nudge = %q, want %q", got, want)
	}
}

func TestAttentionNudge_NoNudgeWhenAttachedIsTopPriority(t *testing.T) {
	base := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)
	h := &Home{}
	insts := []*session.Instance{
		readyInstance("attached-p1", session.StatusRunning, 1, base),
		readyInstance("other-p2", session.StatusWaiting, 2, base),
	}
	if got := h.attentionNudgeText("attached-p1", insts); got != "" {
		t.Fatalf("expected no nudge when attached is top priority, got %q", got)
	}
}

func TestAttentionNudge_UnsetAttachedNudgedByAnyPrioritized(t *testing.T) {
	base := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)
	h := &Home{}
	insts := []*session.Instance{
		readyInstance("attached-unset", session.StatusRunning, 0, base),
		readyInstance("p3-ready", session.StatusIdle, 3, base),
	}
	got := h.attentionNudgeText("attached-unset", insts)
	if got != "⚡ P3 p3-ready ready — ^E" {
		t.Fatalf("unset-attached should be nudged by any prioritized ready session, got %q", got)
	}
}

func TestAttentionNudge_NoNudgeWhenNotAttached(t *testing.T) {
	base := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)
	h := &Home{}
	insts := []*session.Instance{
		readyInstance("p1-ready", session.StatusWaiting, 1, base),
	}
	if got := h.attentionNudgeText("", insts); got != "" {
		t.Fatalf("expected no nudge in list view (attachedID empty), got %q", got)
	}
}

func TestAttentionNudge_IgnoresRunningHigherPriority(t *testing.T) {
	base := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)
	h := &Home{}
	insts := []*session.Instance{
		readyInstance("attached-p2", session.StatusRunning, 2, base),
		readyInstance("p1-running", session.StatusRunning, 1, base), // not ready
	}
	if got := h.attentionNudgeText("attached-p2", insts); got != "" {
		t.Fatalf("a running higher-priority session is not ready — no nudge, got %q", got)
	}
}

func TestAttentionSortedSessions_EmptyWhenNothingReady(t *testing.T) {
	base := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)
	h := &Home{}
	h.instances = []*session.Instance{
		readyInstance("r1", session.StatusRunning, 1, base),
		readyInstance("r2", session.StatusRunning, 2, base),
	}
	if got := h.attentionSortedSessions(); len(got) != 0 {
		t.Fatalf("expected no ready sessions, got %d", len(got))
	}
}

func TestStyledNudgeBar_EmptyPassesThrough(t *testing.T) {
	if got := styledNudgeBar(""); got != "" {
		t.Fatalf("empty nudge should stay empty, got %q", got)
	}
}

func TestStyledNudgeBar_TierColorsAndReset(t *testing.T) {
	cases := []struct {
		plain  string
		wantBg string
	}{
		{"⚡ P1 urgent ready — ^E", "bg=#f7768e"}, // red
		{"⚡ P2 soon ready — ^E", "bg=#ff9e64"},   // orange
		{"⚡ P3 later ready — ^E", "bg=#e0af68"},  // yellow
	}
	for _, tc := range cases {
		got := styledNudgeBar(tc.plain)
		if !strings.Contains(got, tc.wantBg) {
			t.Errorf("styledNudgeBar(%q) = %q, want bg %q", tc.plain, got, tc.wantBg)
		}
		if !strings.HasPrefix(got, "#[") {
			t.Errorf("styledNudgeBar(%q) should start with a tmux style code, got %q", tc.plain, got)
		}
		if !strings.HasSuffix(got, "#[default]") {
			t.Errorf("styledNudgeBar(%q) should reset styling with #[default], got %q", tc.plain, got)
		}
		if !strings.Contains(got, tc.plain) {
			t.Errorf("styledNudgeBar(%q) should preserve the plain text, got %q", tc.plain, got)
		}
	}
}

func TestPriorityBadge_UnsetIsEmpty(t *testing.T) {
	if got := priorityBadge(0, false); got != "" {
		t.Fatalf("unset priority should render no badge, got %q", got)
	}
}

func TestPriorityBadge_RendersTier(t *testing.T) {
	for _, prio := range []int{1, 2, 3} {
		got := priorityBadge(prio, false)
		if got == "" {
			t.Fatalf("priority %d should render a badge", prio)
		}
		if !strings.Contains(got, fmt.Sprintf("P%d", prio)) {
			t.Errorf("priorityBadge(%d) = %q, want it to contain P%d", prio, got, prio)
		}
	}
}
