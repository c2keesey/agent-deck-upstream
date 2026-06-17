// Package ideas implements the quick "by the way" idea backlog: a single,
// append-only markdown file (~/.agent-deck/ideas.md) that captures an idea plus
// the session context it was jotted from, so a passing thought never has to
// fork attention into a new session. See docs/plans/2026-06-14-quick-note-capture.md.
package ideas

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// IdeaEntry is one captured idea plus the session context it was captured from.
// Every metadata field is optional; empty fields are omitted from the rendered
// entry.
type IdeaEntry struct {
	Text            string
	Session         string
	Project         string
	Tool            string
	ClaudeSessionID string
	LastMessageLine string
	At              time.Time
}

// IdeasPath returns the path to the global ideas backlog file. It lives at the
// agent-deck data root (not per-profile) so there is exactly one backlog.
func IdeasPath() (string, error) {
	dir, err := session.GetAgentDeckDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ideas.md"), nil
}

// AppendIdea appends a formatted idea entry to the global backlog.
func AppendIdea(e IdeaEntry) error {
	path, err := IdeasPath()
	if err != nil {
		return err
	}
	return appendIdeaTo(path, e)
}

// appendIdeaTo is the testable core: it writes to an explicit path so tests can
// point at a temp dir without touching the real backlog.
func appendIdeaTo(path string, e IdeaEntry) error {
	if strings.TrimSpace(e.Text) == "" {
		return fmt.Errorf("ideas: empty idea text")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(formatEntry(e))
	return err
}

// formatEntry renders a single markdown section. Metadata lines whose value is
// empty are dropped entirely.
func formatEntry(e IdeaEntry) string {
	text := strings.TrimSpace(e.Text)

	var b strings.Builder
	fmt.Fprintf(&b, "## %s — %s\n\n", e.At.Format("2006-01-02 15:04"), heading(text))
	b.WriteString(text)
	b.WriteString("\n\n")

	if e.Session != "" {
		if e.Tool != "" {
			fmt.Fprintf(&b, "- **session:** %s (%s)\n", e.Session, e.Tool)
		} else {
			fmt.Fprintf(&b, "- **session:** %s\n", e.Session)
		}
	}
	if e.Project != "" {
		fmt.Fprintf(&b, "- **project:** %s\n", e.Project)
	}
	if e.ClaudeSessionID != "" {
		fmt.Fprintf(&b, "- **claude session:** %s\n", e.ClaudeSessionID)
	}
	if e.LastMessageLine != "" {
		fmt.Fprintf(&b, "- **last message line:** %q\n", truncate(collapse(e.LastMessageLine), 200))
	}
	b.WriteString("\n")
	return b.String()
}

// heading collapses the idea to a single line and trims it to a short title.
func heading(text string) string {
	return truncate(collapse(text), 60)
}

// collapse flattens whitespace/newlines into single spaces.
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}
