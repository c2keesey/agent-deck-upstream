package session

import (
	"os/exec"
	"testing"
)

func TestDiscoverExistingTmuxSessions(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	// Should not error even with no existing instances ("" = no profile guard, legacy behaviour)
	discovered, err := DiscoverExistingTmuxSessions([]*Instance{}, "")
	if err != nil {
		t.Logf("DiscoverExistingTmuxSessions error (may be expected): %v", err)
	}
	_ = discovered
}

func TestDiscoverSkipsAgentDeckSessions(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	// Create a mock existing instance
	existing := []*Instance{
		{
			ID:          "test-123",
			Title:       "existing-session",
			ProjectPath: "/tmp",
		},
	}

	discovered, err := DiscoverExistingTmuxSessions(existing, "")
	if err != nil {
		t.Logf("Error (may be expected): %v", err)
	}

	// Should not include sessions that are already tracked
	for _, d := range discovered {
		if d.Title == "existing-session" {
			t.Error("Should not discover already tracked sessions")
		}
	}
}

// TestDiscoverSkipsCrossProfileSessions verifies the profile-blindness fix: discovery
// running under one profile must NOT adopt agentdeck_ sessions owned by a DIFFERENT
// profile (their owner is stamped in the tmux env as AGENTDECK_PROFILE). A same-profile
// session IS still adopted. Regression test for the cross-profile "recovered" adoption bug.
func TestDiscoverSkipsCrossProfileSessions(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	// Two orphan agent-deck sessions on the default tmux server, stamped with different owners.
	mine := "agentdeck_xprofiletest-mine_aaaa1111"     // owner = "default" (the profile we discover under)
	theirs := "agentdeck_xprofiletest-theirs_bbbb2222" // owner = "personal" (a different profile)
	for name, owner := range map[string]string{mine: "default", theirs: "personal"} {
		_ = exec.Command("tmux", "kill-session", "-t", name).Run() // ensure clean
		if err := exec.Command("tmux", "new-session", "-d", "-s", name).Run(); err != nil {
			t.Skipf("cannot create tmux session %s: %v", name, err)
		}
		defer func(n string) { _ = exec.Command("tmux", "kill-session", "-t", n).Run() }(name)
		if err := exec.Command("tmux", "set-environment", "-t", name, "AGENTDECK_PROFILE", owner).Run(); err != nil {
			t.Fatalf("set-environment on %s: %v", name, err)
		}
	}

	discovered, err := DiscoverExistingTmuxSessions([]*Instance{}, "default")
	if err != nil {
		t.Logf("discovery error (may be expected on busy server): %v", err)
	}

	var sawMine, sawTheirs bool
	for _, d := range discovered {
		switch d.Title {
		case "xprofiletest-mine":
			sawMine = true
		case "xprofiletest-theirs":
			sawTheirs = true
		}
	}
	if sawTheirs {
		t.Error("CROSS-PROFILE ADOPTION: a session owned by profile 'personal' was adopted while discovering under 'default'")
	}
	if !sawMine {
		t.Error("a same-profile ('default') orphan should have been discovered but was not")
	}
}

func TestGroupByProjectDeep(t *testing.T) {
	instances := []*Instance{
		{Title: "s1", ProjectPath: "/home/user/projects/devops"},
		{Title: "s2", ProjectPath: "/home/user/projects/frontend"},
		{Title: "s3", ProjectPath: "/home/user/personal/blog"},
		{Title: "s4", ProjectPath: "/tmp"},
	}

	groups := GroupByProject(instances)

	// Check grouping
	if _, ok := groups["projects"]; !ok {
		t.Error("Expected 'projects' group")
	}
	if _, ok := groups["personal"]; !ok {
		t.Error("Expected 'personal' group")
	}
}

func TestFilterByQueryCaseInsensitive(t *testing.T) {
	instances := []*Instance{
		{Title: "DevOps-Claude", ProjectPath: "/tmp", Tool: "claude"},
		{Title: "frontend-shell", ProjectPath: "/tmp", Tool: "shell"},
	}

	// Should match case-insensitively
	result := FilterByQuery(instances, "DEVOPS")
	if len(result) != 1 {
		t.Errorf("Expected 1 result for 'DEVOPS', got %d", len(result))
	}

	result = FilterByQuery(instances, "Claude")
	if len(result) != 1 {
		t.Errorf("Expected 1 result for 'Claude', got %d", len(result))
	}
}

func TestDetectToolFromName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Claude uppercase", "CLAUDE-session", "claude"},
		{"claude lowercase", "my-claude-session", "claude"},
		{"Gemini mixed case", "Gemini-AI", "gemini"},
		{"OpenCode", "opencode-session", "opencode"},
		{"Codex", "codex-test", "codex"},
		{"Unknown", "random-session", "shell"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectToolFromName(tt.input)
			if result != tt.expected {
				t.Errorf("detectToolFromName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractProjectName(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"Deep path", "/home/user/projects/devops", "projects"},
		{"Home path", "/home/user/personal/blog", "personal"},
		{"Root level", "/tmp", "tmp"},
		{"Single level", "/home", "home"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractProjectName(tt.path)
			if result != tt.expected {
				t.Errorf("extractProjectName(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}
