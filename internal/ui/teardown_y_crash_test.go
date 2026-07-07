package ui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// TestTeardownCommand_DetachesFromControllingTerminal pins the fix for the
// teardown "crash" (formerly on the `y` hotkey, now the MAIA flavor of `d`).
// teardown shells out to an interactive zsh (`zsh -ic`) so the user's `gr`
// function (defined in ~/.zshrc) is available. An interactive shell enables
// job control and calls tcsetpgrp() on its controlling terminal to put itself
// in the foreground. Because the child inherited agent-deck's controlling
// terminal, it stole the foreground process group; agent-deck's next input
// read then raised SIGTTIN and stopped the TUI process — the "crash".
//
// The cleanup child MUST run in its own session (Setsid) so it has no
// controlling terminal and cannot touch agent-deck's. If this regresses
// (Setsid dropped, or switched to Setpgid which keeps the controlling
// terminal), the teardown freeze comes back.
func TestTeardownCommand_DetachesFromControllingTerminal(t *testing.T) {
	cmd := teardownCommand(context.Background(), t.TempDir())
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Fatalf("teardown command must run with Setsid to detach from the TUI's controlling terminal "+
			"(the teardown hotkey crash); got SysProcAttr=%+v", cmd.SysProcAttr)
	}
}

// TestTeardownHotkeyRemoved pins the removal of the standalone `y` teardown
// binding: pressing y on a session row must be a no-op (no command, session
// untouched). Teardown now rides the `d` delete flow for MAIA worktree
// sessions only.
func TestTeardownHotkeyRemoved(t *testing.T) {
	inst := &session.Instance{ID: "s1", Title: "Session 1", ProjectPath: t.TempDir()}
	items := []session.Item{
		{Type: session.ItemTypeSession, Session: inst, Level: 0},
	}
	home := newTestHomeWithItems(100, 30, items)
	home.cursor = 0

	model, cmd := home.handleMainKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd != nil {
		t.Fatalf("`y` must no longer produce a command (teardown binding removed), got %T", cmd())
	}
	h2 := model.(*Home)
	if h2.confirmDialog.IsVisible() {
		t.Fatalf("`y` must not open any confirmation dialog")
	}
}

// TestDeleteHotkeyMaiaTeardown_DoesNotCrash drives the full d-teardown flow the
// way the TUI does: press d on a MAIA-worktree session (routes to the teardown
// confirmation), confirm with y, run the returned teardown cmd, feed the
// result msg back through Update, run the follow-up delete cmd and its
// message, rendering View() at each step. The MAIA.<name> dir doesn't exist,
// so the shell step fails fast (the warn path) and no destructive
// `git reset --hard` runs. It must not panic and must delete the session.
func TestDeleteHotkeyMaiaTeardown_DoesNotCrash(t *testing.T) {
	inst := &session.Instance{
		ID:          "s1",
		Title:       "Session 1",
		ProjectPath: maiaReposDir + "/MAIA.unit-test-teardown-does-not-exist",
	}
	items := []session.Item{
		{Type: session.ItemTypeSession, Session: inst, Level: 0},
	}
	home := newTestHomeWithItems(100, 30, items)
	home.cursor = 0
	// Faithfully register the instance the way the live model does, so the
	// teardownResultMsg handler re-resolves by ID and reaches deleteSession.
	home.instances = []*session.Instance{inst}
	if home.instanceByID == nil {
		home.instanceByID = map[string]*session.Instance{}
	}
	home.instanceByID[inst.ID] = inst

	_ = home.View()

	model, cmd := home.handleMainKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("`d` should only open the confirmation dialog, got a command")
	}
	h2 := model.(*Home)
	if !h2.confirmDialog.IsVisible() || h2.confirmDialog.GetConfirmType() != ConfirmTeardownSession {
		t.Fatalf("expected the teardown confirmation for a MAIA worktree session, got visible=%v type=%v",
			h2.confirmDialog.IsVisible(), h2.confirmDialog.GetConfirmType())
	}
	_ = h2.View()

	model, cmd = h2.handleConfirmDialogKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	h2 = model.(*Home)
	if cmd == nil {
		t.Fatalf("expected a teardown command from confirming the dialog, got nil")
	}
	if h2.confirmDialog.IsVisible() {
		t.Fatalf("confirmation dialog should hide once teardown starts")
	}
	_ = h2.View()

	// Pump messages until the command chain drains, rendering each step.
	msg := cmd()
	for i := 0; i < 5 && msg != nil; i++ {
		m, next := h2.Update(msg)
		h2 = m.(*Home)
		_ = h2.View()
		if next == nil {
			break
		}
		msg = next()
	}

	if _, still := h2.instanceByID["s1"]; still {
		t.Fatalf("session should be deleted after the teardown flow completes")
	}
}

// TestDeleteHotkeyNonMaia_PlainDelete pins that `d` on a session outside the
// MAIA worktree namespace keeps its original behavior: the plain delete
// confirmation, no teardown.
func TestDeleteHotkeyNonMaia_PlainDelete(t *testing.T) {
	inst := &session.Instance{ID: "s1", Title: "Session 1", ProjectPath: t.TempDir()}
	items := []session.Item{
		{Type: session.ItemTypeSession, Session: inst, Level: 0},
	}
	home := newTestHomeWithItems(100, 30, items)
	home.cursor = 0

	model, _ := home.handleMainKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	h2 := model.(*Home)
	if !h2.confirmDialog.IsVisible() || h2.confirmDialog.GetConfirmType() != ConfirmDeleteSession {
		t.Fatalf("expected the plain delete confirmation for a non-MAIA session, got visible=%v type=%v",
			h2.confirmDialog.IsVisible(), h2.confirmDialog.GetConfirmType())
	}
}

// TestIsMaiaTeardownWorktree pins the disposable-vs-shared classification of
// MAIA checkouts: only MAIA.<name> dirs directly under the MAIA repos dir are
// teardown targets, and the shared long-lived checkouts (main repo,
// conductor, ro-dev*) are excluded so `d` can never `gr`-reset them.
func TestIsMaiaTeardownWorktree(t *testing.T) {
	cases := []struct {
		dir  string
		want bool
	}{
		{maiaReposDir + "/MAIA.risen-lotus", true},
		{maiaReposDir + "/MAIA.worker-3", true},
		{maiaReposDir + "/MAIA.wt-fix-thing", true},
		{maiaReposDir + "/MAIA.cc-review/", true}, // trailing slash cleaned
		{maiaReposDir + "/MAIA", false},           // main repo
		{maiaReposDir + "/MAIA.conductor", false},
		{maiaReposDir + "/MAIA.ro-dev", false},
		{maiaReposDir + "/MAIA.ro-dev-2", false},
		{maiaReposDir + "/glib", false},                    // sibling non-MAIA repo
		{maiaReposDir + "/nested/MAIA.worker-1", false},    // not directly under repos dir
		{"/tmp/elsewhere/MAIA.worker-1", false},            // outside repos dir
		{"", false},
	}
	for _, tc := range cases {
		if got := isMaiaTeardownWorktree(tc.dir); got != tc.want {
			t.Errorf("isMaiaTeardownWorktree(%q) = %v, want %v", tc.dir, got, tc.want)
		}
	}
}

// TestIsMaiaTeardownSession_WorktreePathFallback: a session whose ProjectPath
// points elsewhere but whose recorded WorktreePath is a disposable MAIA
// worktree still routes to teardown.
func TestIsMaiaTeardownSession_WorktreePathFallback(t *testing.T) {
	inst := &session.Instance{
		ID:           "s1",
		ProjectPath:  "/tmp/somewhere-else",
		WorktreePath: maiaReposDir + "/MAIA.tawny-wave",
	}
	if !isMaiaTeardownSession(inst) {
		t.Fatalf("session with a MAIA WorktreePath should be a teardown target")
	}
	if isMaiaTeardownSession(nil) {
		t.Fatalf("nil session must not be a teardown target")
	}
}
