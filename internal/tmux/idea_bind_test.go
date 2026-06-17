package tmux

import (
	"strings"
	"testing"
)

func TestIdeaCaptureBindArgs(t *testing.T) {
	args := ideaCaptureBindArgs("/usr/local/bin/agent-deck")
	joined := strings.Join(args, " ")

	// Root key table, no prefix, bound to Ctrl+Alt+I.
	for _, want := range []string{"bind-key", "-n", "-T", "root", "C-M-i", "if-shell", "-F"} {
		if !contains(args, want) {
			t.Fatalf("expected arg %q in %v", want, args)
		}
	}
	// Guard restricts the bind to agentdeck-managed sessions.
	if !strings.Contains(joined, "#{m:agentdeck_*,#{session_name}}") {
		t.Fatalf("expected agentdeck session guard, got: %s", joined)
	}
	// Floats a popup over the agent and runs `agent-deck idea`.
	if !strings.Contains(joined, "display-popup -E") {
		t.Fatalf("expected display-popup, got: %s", joined)
	}
	if !strings.Contains(joined, "/usr/local/bin/agent-deck idea") {
		t.Fatalf("expected `agent-deck idea` invocation, got: %s", joined)
	}
}

func TestIdeaCaptureBindArgs_EmptyExeFallsBack(t *testing.T) {
	joined := strings.Join(ideaCaptureBindArgs(""), " ")
	if !strings.Contains(joined, "agent-deck idea") {
		t.Fatalf("empty exe should fall back to bare agent-deck, got: %s", joined)
	}
}

func TestIdeaCaptureBindArgs_ShellEscapesExe(t *testing.T) {
	joined := strings.Join(ideaCaptureBindArgs("/opt/my apps/agent-deck"), " ")
	// A path with a space must be quoted so the shell treats it as one token.
	if !strings.Contains(joined, "'/opt/my apps/agent-deck' idea") {
		t.Fatalf("exe path with space should be shell-quoted, got: %s", joined)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
