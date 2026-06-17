//go:build eval_smoke

// Behavioral eval for the `agent-deck idea` quick-capture subcommand. A Go unit
// test can prove AppendIdea formats correctly, but not that the new interactive
// prompt actually appears, that Enter persists the idea, and that Esc writes
// nothing. Those are user-observable behaviors, so they live here.
package idea_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/asheshgoplani/agent-deck/tests/eval/harness"
)

// findIdeasFile walks the sandbox HOME for the backlog file agent-deck wrote.
// Returns "" when no ideas.md exists anywhere under HOME.
func findIdeasFile(t *testing.T, home string) string {
	t.Helper()
	var found string
	_ = filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Name() == "ideas.md" {
			found = path
		}
		return nil
	})
	return found
}

func readIdeas(t *testing.T, home string) string {
	t.Helper()
	path := findIdeasFile(t, home)
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ideas.md: %v", err)
	}
	return string(b)
}

// Non-interactive path: `agent-deck idea "<text>"` persists immediately. This is
// the deterministic backbone — no PTY timing involved.
func TestEval_Idea_NonInteractive_WritesEntry(t *testing.T) {
	sb := harness.NewSandbox(t)

	p := sb.Spawn("idea", "ship the quick capture feature")
	defer p.Close()
	p.ExpectExit(0, 10*time.Second)

	got := readIdeas(t, sb.Home)
	if !strings.Contains(got, "ship the quick capture feature") {
		t.Fatalf("expected idea persisted to backlog, got:\n%s", got)
	}
}

// Interactive prompt + Enter persists the typed idea. The Bubble Tea renderer
// does not paint over the harness PTY (0x0 winsize), but its input loop still
// runs — so we gate on the startup terminal probe to know the process is live,
// then drive input and assert the observable outcome (the file on disk).
func TestEval_Idea_Interactive_EnterWrites(t *testing.T) {
	sb := harness.NewSandbox(t)

	p := sb.SpawnWithEnv([]string{"TERM=xterm-256color"}, "idea")
	defer p.Close()

	p.ExpectOutput("enter save", 10*time.Second) // rendered footer ⇒ raw mode + input loop live
	p.Send("typed via the popup\r")
	p.ExpectExit(0, 10*time.Second)

	got := readIdeas(t, sb.Home)
	if !strings.Contains(got, "typed via the popup") {
		t.Fatalf("expected typed idea persisted, got:\n%s", got)
	}
}

// Interactive prompt + Esc writes nothing.
func TestEval_Idea_Interactive_EscCancels(t *testing.T) {
	sb := harness.NewSandbox(t)

	p := sb.SpawnWithEnv([]string{"TERM=xterm-256color"}, "idea")
	defer p.Close()

	p.ExpectOutput("enter save", 10*time.Second) // rendered footer ⇒ raw mode + input loop live
	p.Send("\x1b")                               // Esc
	p.ExpectExit(0, 10*time.Second)

	if got := readIdeas(t, sb.Home); got != "" {
		t.Fatalf("Esc must write nothing, but backlog has:\n%s", got)
	}
}
