package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// sampleTargets builds a picker with deterministic targets (bypassing the
// filesystem scan) for testing the pure filter/navigation/selection logic.
func samplePicker() *PersonalPicker {
	return &PersonalPicker{
		visible: true,
		width:   80,
		height:  24,
		targets: []personalTarget{
			{label: "~  (home / root dir)", kind: personalKindHome},
			{label: "optiplex  (" + optiplexSSHCommand + ")", kind: personalKindSSH, command: optiplexSSHCommand},
			{label: "spotify-dl", kind: personalKindProject, path: "/Users/c2k/Projects/spotify-dl"},
			{label: "polymarket-bot", kind: personalKindProject, path: "/Users/c2k/Projects/polymarket-bot"},
			{label: "quant", kind: personalKindProject, path: "/Users/c2k/Projects/quant"},
		},
	}
}

func TestPersonalPicker_ToggleTool(t *testing.T) {
	p := &PersonalPicker{}
	if got := p.ActiveTool(); got != personalToolClaude {
		t.Fatalf("default ActiveTool = %q, want %q", got, personalToolClaude)
	}
	p.ToggleTool()
	if got := p.ActiveTool(); got != personalToolCodex {
		t.Fatalf("ActiveTool = %q, want %q", got, personalToolCodex)
	}
	p.ToggleTool()
	if got := p.ActiveTool(); got != personalToolShell {
		t.Fatalf("ActiveTool = %q, want %q", got, personalToolShell)
	}
	p.ToggleTool()
	if got := p.ActiveTool(); got != personalToolClaude {
		t.Fatalf("ActiveTool wrapped to %q, want %q", got, personalToolClaude)
	}
}

func TestPersonalPicker_Filter(t *testing.T) {
	p := samplePicker()
	// Type "spot" → only spotify-dl matches.
	for _, r := range "spot" {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	got := p.filtered()
	if len(got) != 1 || got[0].label != "spotify-dl" {
		t.Fatalf("filter \"spot\" = %v, want [spotify-dl]", labels(got))
	}
	// Backspace twice → "sp" still matches spotify-dl only.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if p.filter != "sp" {
		t.Fatalf("filter after 2 backspaces = %q, want %q", p.filter, "sp")
	}
}

func TestPersonalPicker_FilterMatchesSpecialRows(t *testing.T) {
	p := samplePicker()
	for _, r := range "opti" {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	got := p.filtered()
	if len(got) != 1 || got[0].kind != personalKindSSH {
		t.Fatalf("filter \"opti\" = %v, want the optiplex ssh row", labels(got))
	}
}

func TestPersonalPicker_NavigationAndSelect(t *testing.T) {
	p := samplePicker()
	// Cursor starts on home.
	if target, ok := p.Selected(); !ok || target.kind != personalKindHome {
		t.Fatalf("initial Selected = %+v ok=%v, want home", target, ok)
	}
	// Down twice → first project (spotify-dl).
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	target, ok := p.Selected()
	if !ok || target.kind != personalKindProject || target.label != "spotify-dl" {
		t.Fatalf("after 2x down Selected = %+v, want spotify-dl project", target)
	}
}

func TestPersonalPicker_CursorClampsAfterFilter(t *testing.T) {
	p := samplePicker()
	// Move cursor to the last row.
	for i := 0; i < 4; i++ {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if p.cursor != 4 {
		t.Fatalf("cursor = %d, want 4", p.cursor)
	}
	// Filter down to a single match; cursor must clamp into range.
	for _, r := range "quant" {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if p.cursor >= len(p.filtered()) {
		t.Fatalf("cursor %d out of range after filter (len=%d)", p.cursor, len(p.filtered()))
	}
	if target, ok := p.Selected(); !ok || target.label != "quant" {
		t.Fatalf("Selected after filter = %+v, want quant", target)
	}
}

func TestPersonalPicker_EmptyFilterSelectionSafe(t *testing.T) {
	p := samplePicker()
	for _, r := range "zzznomatch" {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if _, ok := p.Selected(); ok {
		t.Fatalf("Selected should report ok=false when nothing matches")
	}
}

func TestPersonalPicker_View(t *testing.T) {
	p := samplePicker()
	view := p.View()
	for _, want := range []string{"Claude", "Codex", "Shell", "optiplex", "Filter", "Enter create"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q; got:\n%s", want, view)
		}
	}
}

func TestPersonalPicker_AgentDeckPinned(t *testing.T) {
	p := NewPersonalPicker()
	// refreshTargets always adds the pinned rows before scanning ~/Projects, so
	// the agent-deck shortcut is present even if the scan errors.
	p.refreshTargets()

	var found *personalTarget
	for i := range p.targets {
		if p.targets[i].path == agentDeckDir {
			found = &p.targets[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("agent-deck shortcut missing from picker targets: %v", labels(p.targets))
	}
	if found.kind != personalKindProject {
		t.Errorf("agent-deck row kind = %q, want %q (so Enter creates a session at the repo with the active tool)", found.kind, personalKindProject)
	}
	if !strings.Contains(found.label, "agent-deck") {
		t.Errorf("agent-deck row label = %q, want it to contain \"agent-deck\" so it's filterable", found.label)
	}
}

func labels(ts []personalTarget) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.label
	}
	return out
}
