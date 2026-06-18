package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// Top-priority marker (local fork): the single strict-rank-1 session's row is
// prefixed with a ◆ (U+25C6) glyph in the left gutter so the one next thing to
// pick up stands out at a glance. ◆ is used (not ▶) because ▶ is already the
// selection/attached-row cursor (SessionSelectionPrefix); the diamond sits in
// the gutter so it coexists with the ▶ cursor on a selected rank-1 row. Only
// priority==1 rows get it; everything else renders the plain-space gutter. The
// marker drops into the reserved leftGutterWidth cells, so it must not shift any
// downstream column.
func renderPriorityRow(t *testing.T, prio int, selected bool) string {
	t.Helper()
	forceTrueColorProfile()

	inst := &session.Instance{ID: "prio-sess", Title: "worker-1", Priority: prio}
	item := session.Item{
		Type:          session.ItemTypeSession,
		Session:       inst,
		Level:         1,
		Path:          "test",
		IsLastInGroup: true,
	}
	snapshot := map[string]sessionRenderState{
		inst.ID: {status: session.StatusWaiting, tool: "shell"},
	}

	h := &Home{width: 140}
	var b strings.Builder
	h.renderSessionItem(&b, item, selected, snapshot, h.width)
	return b.String()
}

const (
	topPriorityMarker = "◆" // U+25C6 — rank-1 row marker
	selectionCursor   = "▶" // U+25B6 — selection/attached-row cursor
)

func TestPriorityMarker_OnlyOnRankOne(t *testing.T) {
	if row := renderPriorityRow(t, 1, false); !strings.Contains(row, topPriorityMarker) {
		t.Errorf("rank-1 row should carry the ◆ top-priority marker, got: %q", row)
	}
	for _, prio := range []int{0, 2, 3, 7} {
		if row := renderPriorityRow(t, prio, false); strings.Contains(row, topPriorityMarker) {
			t.Errorf("priority %d row must not carry the ◆ marker (rank-1 only), got: %q", prio, row)
		}
	}
}

// A selected rank-1 row must show BOTH glyphs: the ◆ priority marker AND the
// ▶ selection cursor. They live in different slots (gutter vs selection-prefix)
// so they coexist without collision.
func TestPriorityMarker_CoexistsWithSelectionCursor(t *testing.T) {
	row := renderPriorityRow(t, 1, true)
	if !strings.Contains(row, topPriorityMarker) {
		t.Errorf("selected rank-1 row should still carry the ◆ marker, got: %q", row)
	}
	if !strings.Contains(row, selectionCursor) {
		t.Errorf("selected rank-1 row should also carry the ▶ selection cursor, got: %q", row)
	}
}

// The marker lives in the reserved gutter, so a rank-1 row and an equally-badged
// rank-2 row must stay the same visible width — no column shift.
func TestPriorityMarker_PreservesAlignment(t *testing.T) {
	p1 := lipgloss.Width(renderPriorityRow(t, 1, false))
	p2 := lipgloss.Width(renderPriorityRow(t, 2, false))
	if p1 != p2 {
		t.Errorf("rank-1 marker shifted row width: P1=%d P2=%d (want equal — marker sits in the reserved gutter)", p1, p2)
	}
}
