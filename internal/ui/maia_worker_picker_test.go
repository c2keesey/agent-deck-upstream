package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMaiaWorkerPicker_ShellHint(t *testing.T) {
	p := &MaiaWorkerPicker{
		visible: true,
		workers: []string{"/r/MAIA.worker-1"},
		width:   80,
		height:  24,
	}
	view := p.View()
	if !strings.Contains(view, "s shell") {
		t.Errorf("picker hint should advertise the raw-shell option; got:\n%s", view)
	}
	if !strings.Contains(view, "tool") {
		t.Errorf("picker hint should advertise the tool switcher; got:\n%s", view)
	}
	if !strings.Contains(view, "~ home") {
		t.Errorf("picker hint should advertise the home-root option; got:\n%s", view)
	}
	// The tool switcher names both tools.
	if !strings.Contains(view, "Claude") || !strings.Contains(view, "Codex") {
		t.Errorf("picker should render a Claude/Codex tool switcher; got:\n%s", view)
	}
}

func TestMaiaWorkerPicker_ToggleTool(t *testing.T) {
	p := &MaiaWorkerPicker{}
	// Defaults to Claude.
	if got := p.ActiveTool(); got != maiaToolClaude {
		t.Fatalf("default ActiveTool = %q, want %q", got, maiaToolClaude)
	}
	// Toggle to Codex, then back to Claude.
	p.ToggleTool()
	if got := p.ActiveTool(); got != maiaToolCodex {
		t.Fatalf("after toggle, ActiveTool = %q, want %q", got, maiaToolCodex)
	}
	p.ToggleTool()
	if got := p.ActiveTool(); got != maiaToolClaude {
		t.Fatalf("after second toggle, ActiveTool = %q, want %q", got, maiaToolClaude)
	}
}

func TestMaiaWorkerPicker_NextOpenWorker(t *testing.T) {
	w := []string{"/r/MAIA.worker-1", "/r/MAIA.worker-2", "/r/MAIA.worker-3"}

	// worker-1 occupied -> next open is index 1 (worker-2).
	p := &MaiaWorkerPicker{workers: w, occupied: map[string]bool{"/r/MAIA.worker-1": true}}
	if got := p.nextOpenWorker(); got != 1 {
		t.Errorf("nextOpenWorker = %d, want 1", got)
	}

	// none occupied -> index 0.
	p = &MaiaWorkerPicker{workers: w, occupied: map[string]bool{}}
	if got := p.nextOpenWorker(); got != 0 {
		t.Errorf("nextOpenWorker (none occupied) = %d, want 0", got)
	}

	// all occupied -> fall back to 0.
	p = &MaiaWorkerPicker{workers: w, occupied: map[string]bool{
		"/r/MAIA.worker-1": true, "/r/MAIA.worker-2": true, "/r/MAIA.worker-3": true,
	}}
	if got := p.nextOpenWorker(); got != 0 {
		t.Errorf("nextOpenWorker (all occupied) = %d, want 0", got)
	}
}

func TestMaiaWorkerPicker_Selected(t *testing.T) {
	p := &MaiaWorkerPicker{
		workers:      []string{"/r/MAIA.worker-1", "/r/MAIA.worker-2"},
		roDevs:       []string{"/r/MAIA.ro-dev"},
		workerCursor: 1,
	}

	if path, group := p.Selected(); path != "/r/MAIA.worker-2" || group != maiaWorkerGroup {
		t.Errorf("worker Selected = (%q, %q), want (worker-2, %q)", path, group, maiaWorkerGroup)
	}

	// RoDevSelected returns the shared ro-dev worktree regardless of the worker
	// cursor — it's reached by the 'r' hotkey, not by browsing.
	if path, group := p.RoDevSelected(); path != "/r/MAIA.ro-dev" || group != maiaRoDevGroup {
		t.Errorf("RoDevSelected = (%q, %q), want (ro-dev, %q)", path, group, maiaRoDevGroup)
	}

	// No ro-dev worktree -> empty.
	p.roDevs = nil
	if path, _ := p.RoDevSelected(); path != "" {
		t.Errorf("RoDevSelected with no ro-dev = %q, want empty", path)
	}
}

func TestMaiaWorkerPicker_Navigation(t *testing.T) {
	p := &MaiaWorkerPicker{
		workers: []string{"/r/MAIA.worker-1", "/r/MAIA.worker-2"},
		roDevs:  []string{"/r/MAIA.ro-dev"},
	}
	key := func(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

	// Down advances the worker cursor within range.
	p, _ = p.Update(key(tea.KeyDown))
	if p.workerCursor != 1 {
		t.Fatalf("after down, workerCursor = %d, want 1", p.workerCursor)
	}
	// Down is clamped at the last worker.
	p, _ = p.Update(key(tea.KeyDown))
	if p.workerCursor != 1 {
		t.Fatalf("workerCursor = %d, want 1 (clamped)", p.workerCursor)
	}
	// Up moves back.
	p, _ = p.Update(key(tea.KeyUp))
	if p.workerCursor != 0 {
		t.Fatalf("after up, workerCursor = %d, want 0", p.workerCursor)
	}
}

func TestWorkerSortKey(t *testing.T) {
	if a, b := workerSortKey("/r/MAIA.worker-2"), workerSortKey("/r/MAIA.worker-10"); a >= b {
		t.Errorf("worker-2 (%d) should sort before worker-10 (%d)", a, b)
	}
	if a, b := workerSortKey("/r/MAIA.worker-9"), workerSortKey("/r/MAIA.worker-retry"); a >= b {
		t.Errorf("worker-9 (%d) should sort before worker-retry (%d)", a, b)
	}
}
