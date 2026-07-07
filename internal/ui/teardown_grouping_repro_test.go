package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// dumpListStructure renders flatItems + groupTree membership compactly for
// structural comparison in tests.
func dumpListStructure(h *Home) string {
	var b strings.Builder
	b.WriteString("flatItems:\n")
	for i, it := range h.flatItems {
		switch it.Type {
		case session.ItemTypeGroup:
			fmt.Fprintf(&b, "  [%d] GROUP %s\n", i, it.Path)
		case session.ItemTypeSession:
			id := "<nil>"
			if it.Session != nil {
				id = it.Session.ID
			}
			fmt.Fprintf(&b, "  [%d] SESSION %s (path=%s)\n", i, id, it.Path)
		default:
			fmt.Fprintf(&b, "  [%d] OTHER type=%v path=%s\n", i, it.Type, it.Path)
		}
	}
	b.WriteString("groupTree:\n")
	for _, g := range h.groupTree.GroupList {
		fmt.Fprintf(&b, "  group %s:", g.Path)
		for _, s := range g.Sessions {
			fmt.Fprintf(&b, " %s(gp=%s)", s.ID, s.GroupPath)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// newGroupedTeardownHome builds a Home with two groups and three sessions,
// fully wired (instances, instanceByID, groupTree, flatItems) the way the live
// model is. a2 gets the given ProjectPath so callers can make it a MAIA
// teardown target or a plain session.
func newGroupedTeardownHome(t *testing.T, a2Path string) *Home {
	t.Helper()
	dir := t.TempDir()
	mk := func(id, group, path string) *session.Instance {
		return &session.Instance{ID: id, Title: id, ProjectPath: path, GroupPath: group}
	}
	instances := []*session.Instance{
		mk("a1", "alpha", dir), mk("a2", "alpha", a2Path),
		mk("b1", "beta", dir),
	}
	home := NewHome()
	home.width, home.height = 100, 30
	home.initialLoading = false
	home.instances = instances
	for _, inst := range instances {
		home.instanceByID[inst.ID] = inst
	}
	home.groupTree = session.NewGroupTree(instances)
	home.rebuildFlatItems()
	return home
}

func pumpAll(t *testing.T, h *Home, cmd tea.Cmd) *Home {
	t.Helper()
	for i := 0; i < 8 && cmd != nil; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		var m tea.Model
		m, cmd = h.Update(msg)
		h = m.(*Home)
		_ = h.View()
	}
	return h
}

// TestSessionDeletedDuringReload_RequeuedNotDropped pins the fix for the lost
// delete: a sessionDeletedMsg arriving while a storage reload is in flight
// used to be dropped outright — after the delete cmd had already killed tmux
// and removed the worktree — leaving a zombie row that the reload resurrected.
// The handler must requeue the message and apply it once the reload finishes.
func TestSessionDeletedDuringReload_RequeuedNotDropped(t *testing.T) {
	home := newGroupedTeardownHome(t, t.TempDir())

	home.reloadMu.Lock()
	home.isReloading = true
	home.reloadMu.Unlock()

	model, cmd := home.Update(sessionDeletedMsg{deletedID: "a2"})
	home = model.(*Home)
	if cmd == nil {
		t.Fatalf("sessionDeletedMsg during reload must be requeued, not dropped")
	}
	if _, still := home.instanceByID["a2"]; !still {
		t.Fatalf("delete must not apply while a reload is in flight")
	}

	// Reload finishes; the requeued tick re-delivers the message.
	home.reloadMu.Lock()
	home.isReloading = false
	home.reloadMu.Unlock()

	msg := cmd() // tea.Tick fires after its delay in real time; invoke directly
	model, _ = home.Update(msg)
	home = model.(*Home)
	if _, still := home.instanceByID["a2"]; still {
		t.Fatalf("requeued delete must apply once the reload completes")
	}
	if strings.Contains(dumpListStructure(home), "a2") {
		t.Fatalf("deleted session must be gone from the tree and flat items:\n%s", dumpListStructure(home))
	}
}

// TestTeardownDelete_GroupingMatchesPlainDelete is a regression test for the
// grouping corruption seen after the old `y` teardown: the teardown-flavored
// delete (d on a MAIA worktree session → confirm → gr/make down → delete) must
// leave the group tree and flat item list in exactly the same shape as a plain
// delete of the same session.
func TestTeardownDelete_GroupingMatchesPlainDelete(t *testing.T) {
	// Path A: plain delete of a2 (what 'd' + confirm runs for non-MAIA rows).
	homeD := newGroupedTeardownHome(t, t.TempDir())
	homeD = pumpAll(t, homeD, homeD.deleteSession(homeD.instanceByID["a2"]))
	gotD := dumpListStructure(homeD)

	// Path B: d-teardown of a2 (MAIA worktree path; the dir doesn't exist so
	// the shell step fails fast via the warn path and no reset runs).
	homeY := newGroupedTeardownHome(t, maiaReposDir+"/MAIA.unit-test-grouping-does-not-exist")
	for i, it := range homeY.flatItems {
		if it.Type == session.ItemTypeSession && it.Session != nil && it.Session.ID == "a2" {
			homeY.cursor = i
		}
	}
	model, _ := homeY.handleMainKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	homeY = model.(*Home)
	if !homeY.confirmDialog.IsVisible() || homeY.confirmDialog.GetConfirmType() != ConfirmTeardownSession {
		t.Fatalf("expected teardown confirmation, got visible=%v type=%v",
			homeY.confirmDialog.IsVisible(), homeY.confirmDialog.GetConfirmType())
	}
	model, cmd := homeY.handleConfirmDialogKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	homeY = pumpAll(t, model.(*Home), cmd)
	gotY := dumpListStructure(homeY)

	if gotD != gotY {
		t.Errorf("structures diverge:\n--- plain delete ---\n%s\n--- teardown delete ---\n%s", gotD, gotY)
	}
	if strings.Contains(gotY, "a2") {
		t.Errorf("torn-down session still present:\n%s", gotY)
	}
}
