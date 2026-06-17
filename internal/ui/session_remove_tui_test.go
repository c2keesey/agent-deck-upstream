package ui

import (
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/session"
	tea "github.com/charmbracelet/bubbletea"
)

// newRemoveTestItem builds a flatItems entry for Seam A tests with a given status.
func newRemoveTestItem(id, title string, status session.Status) session.Item {
	return session.Item{
		Type: session.ItemTypeSession,
		Session: &session.Instance{
			ID:     id,
			Title:  title,
			Status: status,
		},
	}
}

// TestSessionRemoveTUI_CapitalX_OnStopped_OpensConfirm — 'X' over a stopped
// session opens the remove-confirm dialog (distinct from the existing 'd'
// destructive-delete dialog).
func TestSessionRemoveTUI_CapitalX_OnStopped_OpensConfirm(t *testing.T) {
	h := newSeamATestHome()
	h.flatItems = []session.Item{newRemoveTestItem("id-1", "stopped-one", session.StatusStopped)}
	h.cursor = 0

	newModel, _ := h.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	got := newModel.(*Home)

	if !got.confirmDialog.IsVisible() {
		t.Fatalf("confirm dialog should be visible after 'X' on stopped session")
	}
	if got.confirmDialog.GetConfirmType() != ConfirmRemoveSession {
		t.Fatalf("expected ConfirmRemoveSession, got %v", got.confirmDialog.GetConfirmType())
	}
	if got.confirmDialog.GetTargetID() != "id-1" {
		t.Fatalf("expected targetID 'id-1', got %q", got.confirmDialog.GetTargetID())
	}
}

// TestSessionRemoveTUI_CapitalX_OnErrored_OpensConfirm — error state qualifies.
func TestSessionRemoveTUI_CapitalX_OnErrored_OpensConfirm(t *testing.T) {
	h := newSeamATestHome()
	h.flatItems = []session.Item{newRemoveTestItem("id-err", "err-one", session.StatusError)}
	h.cursor = 0

	newModel, _ := h.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	got := newModel.(*Home)

	if !got.confirmDialog.IsVisible() {
		t.Fatalf("confirm dialog should be visible after 'X' on errored session")
	}
	if got.confirmDialog.GetConfirmType() != ConfirmRemoveSession {
		t.Fatalf("expected ConfirmRemoveSession, got %v", got.confirmDialog.GetConfirmType())
	}
}

// TestSessionRemoveTUI_CapitalX_OnRunning_ShowsError — safety gate in the UI.
func TestSessionRemoveTUI_CapitalX_OnRunning_ShowsError(t *testing.T) {
	h := newSeamATestHome()
	h.flatItems = []session.Item{newRemoveTestItem("id-run", "running-one", session.StatusRunning)}
	h.cursor = 0

	newModel, _ := h.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	got := newModel.(*Home)

	if got.confirmDialog.IsVisible() {
		t.Fatalf("confirm dialog should NOT open for a running session")
	}
	if got.err == nil {
		t.Fatalf("expected an error message steering user to 'd' for destructive delete")
	}
}

// Personal fork: upstream's Ctrl+X "bulk-remove all errored sessions" was
// dropped here — Ctrl+X is remapped to "close session" in the local hotkey
// scheme, so the bulk-remove tests (and bulkRemoveErrored) were removed.
