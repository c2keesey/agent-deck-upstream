package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// TestTeardownCmd_DeletesInline_NoUpdateLoopHop pins the fix for teardown
// stalling while the user is attached to a session. Attach runs under
// tea.Exec, which blocks the Bubble Tea update loop; any teardown step that
// rides a message hop (teardownResultMsg → Update → deleteSession cmd) parks
// in the queue until detach, so the tmux kill / worktree removal only ran
// once the dashboard was visible again — the "teardown pauses unless I'm in
// the dashboard view" bug.
//
// The teardown cmd must therefore complete ALL side-effect steps on its own
// goroutine and hand the loop a single sessionDeletedMsg whose processing is
// purely cosmetic (row removal, toasts).
func TestTeardownCmd_DeletesInline_NoUpdateLoopHop(t *testing.T) {
	inst := &session.Instance{
		ID:          "s1",
		Title:       "Session 1",
		ProjectPath: maiaReposDir + "/MAIA.unit-test-attach-stall-does-not-exist",
	}
	items := []session.Item{
		{Type: session.ItemTypeSession, Session: inst, Level: 0},
	}
	home := newTestHomeWithItems(100, 30, items)
	home.cursor = 0
	home.instances = []*session.Instance{inst}
	if home.instanceByID == nil {
		home.instanceByID = map[string]*session.Instance{}
	}
	home.instanceByID[inst.ID] = inst

	// d on a MAIA worktree session → teardown confirmation → confirm.
	model, _ := home.handleMainKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	h := model.(*Home)
	if !h.confirmDialog.IsVisible() || h.confirmDialog.GetConfirmType() != ConfirmTeardownSession {
		t.Fatalf("expected the teardown confirmation, got visible=%v type=%v",
			h.confirmDialog.IsVisible(), h.confirmDialog.GetConfirmType())
	}
	model, cmd := h.handleConfirmDialogKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	h = model.(*Home)
	if cmd == nil {
		t.Fatal("confirming teardown produced no cmd")
	}

	// Execute only the cmd goroutine's work, WITHOUT pumping any intermediate
	// message through Update — exactly the situation while the user is
	// attached (tea.Exec blocks the loop; queued messages are not processed).
	// The MAIA.<name> dir doesn't exist, so the shell step fails fast (warn
	// path) and no destructive `git reset --hard` runs.
	msg := cmd()
	del, ok := msg.(sessionDeletedMsg)
	if !ok {
		t.Fatalf("teardown cmd must run the delete inline and return sessionDeletedMsg, got %T — "+
			"an intermediate message hop stalls the teardown until the user detaches", msg)
	}
	if del.deletedID != "s1" {
		t.Fatalf("deleted id = %q, want %q", del.deletedID, "s1")
	}
	if del.teardownTitle != "Session 1" {
		t.Fatalf("teardownTitle = %q, want %q (needed for the completion toast)", del.teardownTitle, "Session 1")
	}
	if del.teardownWarn == "" {
		t.Fatal("expected a cleanup warning for the nonexistent worktree dir")
	}

	// Once the user detaches, applying the single queued message removes the row.
	model, _ = h.Update(msg)
	h = model.(*Home)
	if _, still := h.instanceByID["s1"]; still {
		t.Fatal("session should be removed once the queued deletion message is applied")
	}
}
