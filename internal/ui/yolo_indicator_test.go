package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// YOLO (auto-approve / dangerous) mode is shown by recoloring the tool word and
// appending a compact "!" (e.g. "codex!"), NOT a separate 7-cell " [YOLO]"
// badge. The old badge ragged every trailing column on worker-N rows: some
// workers had it, some didn't, so worktree chips and the activity trailer no
// longer lined up. Folding it into the tool word — which codex/gemini/hermes
// always render — adds at most one cell, keeping rows aligned. (local fork)
func renderYoloRow(t *testing.T, tool string, yolo bool) string {
	t.Helper()
	forceTrueColorProfile()

	inst := &session.Instance{ID: "yolo-sess", Title: "worker-3"}
	if yolo {
		y := true
		if err := inst.SetCodexOptions(&session.CodexOptions{YoloMode: &y}); err != nil {
			t.Fatalf("SetCodexOptions: %v", err)
		}
	}
	item := session.Item{
		Type:          session.ItemTypeSession,
		Session:       inst,
		Level:         1,
		Path:          "test",
		IsLastInGroup: true,
	}
	snapshot := map[string]sessionRenderState{
		inst.ID: {status: session.StatusRunning, tool: tool},
	}

	h := &Home{width: 140}
	var b strings.Builder
	h.renderSessionItem(&b, item, false, snapshot, h.width)
	return b.String()
}

func TestYolo_FoldsIntoToolWord(t *testing.T) {
	row := renderYoloRow(t, "codex", true)
	if strings.Contains(row, "[YOLO]") {
		t.Errorf("YOLO row still uses the wide [YOLO] badge; want it folded into the tool word.\nrow: %q", row)
	}
	if !strings.Contains(row, "codex!") {
		t.Errorf("YOLO codex row should show the tool word with a trailing !, got: %q", row)
	}
}

func TestYolo_OffShowsPlainToolWord(t *testing.T) {
	row := renderYoloRow(t, "codex", false)
	if strings.Contains(row, "codex!") {
		t.Errorf("non-YOLO row should not have the ! marker, got: %q", row)
	}
	if !strings.Contains(row, "codex") {
		t.Errorf("codex row should still show the tool word, got: %q", row)
	}
}

// The YOLO marker must add at most one visible cell over the plain tool word,
// so a worker with YOLO and one without stay aligned (the old badge added 7).
func TestYolo_AddsAtMostOneCell(t *testing.T) {
	on := lipgloss.Width(renderYoloRow(t, "codex", true))
	off := lipgloss.Width(renderYoloRow(t, "codex", false))
	if delta := on - off; delta > 1 {
		t.Errorf("YOLO marker added %d cells over the plain tool word, want <= 1", delta)
	}
}
