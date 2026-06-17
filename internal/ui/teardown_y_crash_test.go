package ui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// TestTeardownCommand_DetachesFromControllingTerminal pins the fix for the `y`
// hotkey "crash". teardown shells out to an interactive zsh (`zsh -ic`) so the
// user's `gr` function (defined in ~/.zshrc) is available. An interactive shell
// enables job control and calls tcsetpgrp() on its controlling terminal to put
// itself in the foreground. Because the child inherited agent-deck's controlling
// terminal, it stole the foreground process group; agent-deck's next input read
// then raised SIGTTIN and stopped the TUI process — the "crash".
//
// The cleanup child MUST run in its own session (Setsid) so it has no
// controlling terminal and cannot touch agent-deck's. If this regresses
// (Setsid dropped, or switched to Setpgid which keeps the controlling
// terminal), the `y` freeze comes back.
func TestTeardownCommand_DetachesFromControllingTerminal(t *testing.T) {
	cmd := teardownCommand(context.Background(), t.TempDir())
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Fatalf("teardown command must run with Setsid to detach from the TUI's controlling terminal "+
			"(the `y` hotkey crash); got SysProcAttr=%+v", cmd.SysProcAttr)
	}
}

// TestTeardownHotkeyY_DoesNotCrash drives the full `y` teardown flow the way the
// TUI does: press y on a session, run the returned cmd, feed the result msg back
// through Update, run the follow-up delete cmd and its message, rendering View()
// at each step. A non-MAIA temp dir makes `gr` exit non-zero (the warn path) so
// no destructive `git reset --hard` runs. It must not panic.
func TestTeardownHotkeyY_DoesNotCrash(t *testing.T) {
	dir := t.TempDir()
	inst := &session.Instance{ID: "s1", Title: "Session 1", ProjectPath: dir}
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

	model, cmd := home.handleMainKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatalf("expected a teardown command from `y`, got nil")
	}
	h2 := model.(*Home)
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
}
