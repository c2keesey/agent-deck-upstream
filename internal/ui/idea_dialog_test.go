package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/asheshgoplani/agent-deck/internal/ideas"
	"github.com/asheshgoplani/agent-deck/internal/session"
)

// TestIdeaDialog_HotkeyOpens verifies Alt+I opens the capture dialog from the
// dashboard. Dashboard capture is intentionally CONTEXTLESS — it does not
// snapshot whichever session the cursor happens to be on.
func TestIdeaDialog_HotkeyOpens(t *testing.T) {
	inst := &session.Instance{ID: "s1", Title: "worker-2", ProjectPath: "/tmp/p", Tool: "claude"}
	items := []session.Item{{Type: session.ItemTypeSession, Session: inst, Level: 0}}
	h := newTestHomeWithItems(100, 30, items)
	h.cursor = 0
	h.instances = []*session.Instance{inst}
	if h.instanceByID == nil {
		h.instanceByID = map[string]*session.Instance{}
	}
	h.instanceByID[inst.ID] = inst

	model, _ := h.handleMainKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}, Alt: true})
	h2 := model.(*Home)
	if !h2.ideaDialog.IsVisible() {
		t.Fatalf("Alt+I should open the idea dialog")
	}
}

// TestIdeaDialog_EnterSavesContextlessEntry drives the full flow and asserts the
// idea is persisted as TEXT ONLY — no session/project/tool — even though a
// session is selected. appendIdeaFunc is stubbed so the backlog is never touched.
func TestIdeaDialog_EnterSavesContextlessEntry(t *testing.T) {
	var saved *ideas.IdeaEntry
	orig := appendIdeaFunc
	appendIdeaFunc = func(e ideas.IdeaEntry) error { saved = &e; return nil }
	t.Cleanup(func() { appendIdeaFunc = orig })

	// A session is selected, but the dashboard capture must ignore it.
	inst := &session.Instance{ID: "s1", Title: "worker-2", ProjectPath: "/tmp/p", Tool: "codex"}
	h := &Home{instanceByID: map[string]*session.Instance{"s1": inst}}
	h.instances = []*session.Instance{inst}
	h.ideaDialog = NewIdeaDialog()
	h.ideaDialog.Show()

	// Type the idea, then Enter.
	for _, r := range "fix the flaky test" {
		h.ideaDialog, _ = h.ideaDialog.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	h.handleIdeaDialogKey(tea.KeyMsg{Type: tea.KeyEnter})

	if h.ideaDialog.IsVisible() {
		t.Errorf("dialog should close after Enter")
	}
	if saved == nil {
		t.Fatalf("Enter should have persisted an idea")
	}
	if saved.Text != "fix the flaky test" {
		t.Errorf("saved idea text = %q", saved.Text)
	}
	if saved.Session != "" || saved.Project != "" || saved.Tool != "" || saved.ClaudeSessionID != "" {
		t.Errorf("dashboard idea must be contextless, got %+v", *saved)
	}
}

// TestIdeaDialog_EscCancelsWithoutSaving verifies Esc discards the draft.
func TestIdeaDialog_EscCancelsWithoutSaving(t *testing.T) {
	saved := false
	orig := appendIdeaFunc
	appendIdeaFunc = func(ideas.IdeaEntry) error { saved = true; return nil }
	t.Cleanup(func() { appendIdeaFunc = orig })

	h := &Home{}
	h.ideaDialog = NewIdeaDialog()
	h.ideaDialog.Show()
	h.ideaDialog, _ = h.ideaDialog.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	h.handleIdeaDialogKey(tea.KeyMsg{Type: tea.KeyEsc})

	if h.ideaDialog.IsVisible() {
		t.Errorf("Esc should close the dialog")
	}
	if saved {
		t.Errorf("Esc must not persist anything")
	}
}

// TestIdeaDialog_BuildEntryIsBare records a bare idea (text + timestamp only).
func TestIdeaDialog_BuildEntryIsBare(t *testing.T) {
	h := &Home{}
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	entry := h.buildIdeaEntry("a stray thought", now)
	if entry.Text != "a stray thought" || entry.At != now || entry.Session != "" || entry.Project != "" {
		t.Errorf("bare idea entry = %+v, want text + timestamp only", entry)
	}
}
