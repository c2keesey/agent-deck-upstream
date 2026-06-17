package ui

import (
	"strings"
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// Session row pane-title suffix. PERSONAL-FORK OVERRIDE of upstream #1343:
// upstream gates the dim task-description suffix (the tmux pane title) behind
// [display] show_pane_titles (off = selected-row-only). This fork instead
// ALWAYS broadcasts the suffix on every row — the user wants live activity
// visible at a glance — and falls back to the session status word when the
// pane title is empty so the slot is never blank. The show_pane_titles flag
// is therefore a no-op for visibility here (always-on supersedes it).
//
// renderRowWithPaneTitle renders one session row with the given selection and
// show_pane_titles flag and returns the rendered string for assertions.
func renderRowWithPaneTitle(t *testing.T, selected, showAll bool, paneTitle string) string {
	t.Helper()
	forceTrueColorProfile()

	h := &Home{width: 140, showPaneTitles: showAll}
	inst := &session.Instance{
		ID:    "sess-pane-title",
		Title: "with-pane-title",
	}
	item := session.Item{
		Type:          session.ItemTypeSession,
		Session:       inst,
		Level:         1,
		Path:          "test",
		IsLastInGroup: true,
	}
	snapshot := map[string]sessionRenderState{
		inst.ID: {
			status:    session.StatusRunning,
			tool:      "claude",
			paneTitle: paneTitle,
		},
	}

	var b strings.Builder
	h.renderSessionItem(&b, item, selected, snapshot, h.width)
	return b.String()
}

const sampleTaskTitle = "Explore messaging support features"

// TestPaneTitle_ShowAllRendersOnUnselectedRow verifies show_pane_titles=true
// renders the pane-title suffix on an unselected row.
func TestPaneTitle_ShowAllRendersOnUnselectedRow(t *testing.T) {
	row := renderRowWithPaneTitle(t, false, true, sampleTaskTitle)
	if !strings.Contains(row, sampleTaskTitle) {
		t.Fatalf("show_pane_titles=true must render the pane-title suffix on an unselected row, "+
			"but %q was not found. Got: %q", sampleTaskTitle, row)
	}
}

// TestPaneTitle_AlwaysOnEvenWhenToggleOff pins the personal-fork override of
// upstream #1343: the pane-title suffix renders on an UNSELECTED row even when
// show_pane_titles is off. Upstream omits it in that case; this fork always
// broadcasts. (Resolved during the upstream sync that brought in #1343.)
func TestPaneTitle_AlwaysOnEvenWhenToggleOff(t *testing.T) {
	row := renderRowWithPaneTitle(t, false, false, sampleTaskTitle)
	if !strings.Contains(row, sampleTaskTitle) {
		t.Fatalf("personal-fork always-on: the pane-title suffix must render on an "+
			"unselected row even with show_pane_titles off, but %q was not found. Got: %q",
			sampleTaskTitle, row)
	}
}

// TestPaneTitle_NoEchoWhenBroadcastIsPrimary pins the de-dup for the MAIA-worker
// fix: when the live pane title wins the PRIMARY label (an active worker with an
// auto title and no distinguishing branch), the always-on trailing slot must not
// repeat it — the row should read "<task> · running", not "<task> · <task>".
func TestPaneTitle_NoEchoWhenBroadcastIsPrimary(t *testing.T) {
	forceTrueColorProfile()
	const task = "Exploring messaging support"

	h := &Home{width: 140}
	inst := &session.Instance{
		ID:          "worker-sess",
		Title:       "light-thorn", // auto-generated → not a sticky title
		ProjectPath: "/x/MAIA.worker-3",
	}
	item := session.Item{Type: session.ItemTypeSession, Session: inst, Level: 1, Path: "test", IsLastInGroup: true}
	snapshot := map[string]sessionRenderState{
		inst.ID: {status: session.StatusRunning, tool: "claude", paneTitle: task},
	}

	var b strings.Builder
	h.renderSessionItem(&b, item, false, snapshot, h.width)
	row := b.String()

	if n := strings.Count(row, task); n != 1 {
		t.Fatalf("active worker should show the pane title exactly once (primary, not echoed in the trailing slot), got %d occurrences.\nRow: %q", n, row)
	}
}

// TestPaneTitle_StatusFallbackWhenEmpty pins the other half of the personal-fork
// override: when the pane title is empty the slot falls back to the session
// status word so it is never blank. Upstream renders nothing in this case.
func TestPaneTitle_StatusFallbackWhenEmpty(t *testing.T) {
	row := renderRowWithPaneTitle(t, false, false, "")
	if !strings.Contains(row, string(session.StatusRunning)) {
		t.Fatalf("personal-fork status fallback: an empty pane title must fall back to the "+
			"status word %q, but it was not found. Got: %q", string(session.StatusRunning), row)
	}
}

// TestPaneTitle_SelectedRowAlwaysRenders verifies the selected row renders its
// pane-title suffix even when show_pane_titles is off.
func TestPaneTitle_SelectedRowAlwaysRenders(t *testing.T) {
	// Selected-only behavior must be preserved when the toggle is off.
	row := renderRowWithPaneTitle(t, true, false, sampleTaskTitle)
	if !strings.Contains(row, sampleTaskTitle) {
		t.Fatalf("the selected row must always render its pane-title suffix regardless of "+
			"show_pane_titles, but %q was not found. Got: %q", sampleTaskTitle, row)
	}
}
