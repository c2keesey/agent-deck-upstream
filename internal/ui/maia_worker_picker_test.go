package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

func TestMaiaWorkerPicker_Hints(t *testing.T) {
	p := &MaiaWorkerPicker{
		visible: true,
		width:   120,
		height:  30,
	}
	view := p.View()
	// The primary action is a fresh ad-hoc worktree — no worker pool to browse.
	if !strings.Contains(view, "new worktree") {
		t.Errorf("picker should advertise the new-worktree action; got:\n%s", view)
	}
	for _, hint := range []string{"s shell", "~ home", "c conductor", "r ro-dev", "R remote", "Tab tool"} {
		if !strings.Contains(view, hint) {
			t.Errorf("picker hint should advertise %q; got:\n%s", hint, view)
		}
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

func TestNewWorktreeSpec(t *testing.T) {
	name, worktreePath, branch, err := NewWorktreeSpec(nil, maiaWorkerGroup)
	if err != nil {
		t.Fatalf("NewWorktreeSpec error: %v", err)
	}
	if name == "" {
		t.Fatal("NewWorktreeSpec returned empty name")
	}
	if want := filepath.Join(maiaReposDir, "MAIA."+name); worktreePath != want {
		t.Errorf("worktreePath = %q, want %q", worktreePath, want)
	}
	if want := maiaAdhocBranchPrefix + name; branch != want {
		t.Errorf("branch = %q, want %q", branch, want)
	}
	// A fresh spec must not point at a directory that already exists — a stale
	// dir from an interrupted teardown must never be silently reused.
	if _, statErr := os.Stat(worktreePath); !os.IsNotExist(statErr) {
		t.Errorf("worktreePath %q already exists on disk", worktreePath)
	}
	// The generated name must be unique against live instances too (the
	// generator's contract) — sanity-check by passing an instance holding the
	// same name and confirming a different one comes back.
	inst := &session.Instance{Title: name, GroupPath: maiaWorkerGroup}
	name2, _, _, err := NewWorktreeSpec([]*session.Instance{inst}, maiaWorkerGroup)
	if err != nil {
		t.Fatalf("NewWorktreeSpec (collision) error: %v", err)
	}
	if name2 == name {
		t.Errorf("NewWorktreeSpec reused a live session name %q", name)
	}
}

func TestMaiaWorkerPicker_RoDevSelected(t *testing.T) {
	p := &MaiaWorkerPicker{roDevs: []string{"/r/MAIA.ro-dev"}}

	// RoDevSelected returns the shared ro-dev worktree — reached by the 'r'
	// hotkey, not by browsing.
	if path, group := p.RoDevSelected(); path != "/r/MAIA.ro-dev" || group != maiaRoDevGroup {
		t.Errorf("RoDevSelected = (%q, %q), want (ro-dev, %q)", path, group, maiaRoDevGroup)
	}

	// No ro-dev worktree -> empty.
	p.roDevs = nil
	if path, _ := p.RoDevSelected(); path != "" {
		t.Errorf("RoDevSelected with no ro-dev = %q, want empty", path)
	}
}

func TestMaiaWorkerPicker_ConductorSelected(t *testing.T) {
	// With a MAIA.conductor worktree present, 'c' targets it in the conductor group.
	p := &MaiaWorkerPicker{conductors: []string{"/r/MAIA.conductor"}}
	if path, group := p.ConductorSelected(); path != "/r/MAIA.conductor" || group != maiaConductorGroup {
		t.Errorf("ConductorSelected = (%q, %q), want (MAIA.conductor, %q)", path, group, maiaConductorGroup)
	}

	// No conductor worktree -> empty (the 'c' hotkey then no-ops in the caller).
	p.conductors = nil
	if path, _ := p.ConductorSelected(); path != "" {
		t.Errorf("ConductorSelected with no conductor = %q, want empty", path)
	}
}

func TestResolveRemoteLane(t *testing.T) {
	// MAIA_REMOTE_LANE wins when set.
	if got := resolveRemoteLane("ck", "chris"); got != "ck" {
		t.Errorf("resolveRemoteLane(ck, chris) = %q, want ck", got)
	}
	// Falls back to $USER when the lane env is empty.
	if got := resolveRemoteLane("", "chris"); got != "chris" {
		t.Errorf("resolveRemoteLane(\"\", chris) = %q, want chris", got)
	}
	// Final fallback when neither is set.
	if got := resolveRemoteLane("", ""); got != "remote" {
		t.Errorf("resolveRemoteLane(\"\", \"\") = %q, want remote", got)
	}
}

func TestParseRemoteLaneID(t *testing.T) {
	// A real remote-claude-new launch (with the absolute script path) parses lane+id.
	cmd := "/Users/c2k/repos/agent-deck/scripts/remote-claude-new chris 20260624-174229-768"
	if lane, id, ok := parseRemoteLaneID(cmd); !ok || lane != "chris" || id != "20260624-174229-768" {
		t.Errorf("parseRemoteLaneID(%q) = (%q, %q, %v), want (chris, 20260624-174229-768, true)", cmd, lane, id, ok)
	}

	// A trailing model arg doesn't change lane/id extraction.
	cmd = "/x/remote-claude-new ck 20260101-000000-1 sonnet"
	if lane, id, ok := parseRemoteLaneID(cmd); !ok || lane != "ck" || id != "20260101-000000-1" {
		t.Errorf("parseRemoteLaneID with model = (%q, %q, %v), want (ck, 20260101-000000-1, true)", lane, id, ok)
	}

	// A non-remote command (a worker/shell session) is not a remote launch.
	if _, _, ok := parseRemoteLaneID("claude"); ok {
		t.Errorf("parseRemoteLaneID(\"claude\") ok = true, want false")
	}
	// remote-claude-new with no args -> not enough fields, ok=false (caller skips teardown).
	if _, _, ok := parseRemoteLaneID("/x/remote-claude-new"); ok {
		t.Errorf("parseRemoteLaneID with no args ok = true, want false")
	}
}
