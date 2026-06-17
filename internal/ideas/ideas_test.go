package ideas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixedTime() time.Time {
	return time.Date(2026, 6, 14, 15, 42, 0, 0, time.UTC)
}

func TestAppendIdea_CreatesAndAppendsInOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ideas.md")

	if err := appendIdeaTo(path, IdeaEntry{Text: "first idea", At: fixedTime()}); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := appendIdeaTo(path, IdeaEntry{Text: "second idea", At: fixedTime()}); err != nil {
		t.Fatalf("second append: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	out := string(data)

	i1 := strings.Index(out, "first idea")
	i2 := strings.Index(out, "second idea")
	if i1 < 0 || i2 < 0 {
		t.Fatalf("both ideas should be present, got:\n%s", out)
	}
	if i1 > i2 {
		t.Fatalf("entries should be appended in order (first before second):\n%s", out)
	}
	if got := strings.Count(out, "## 2026-06-14 15:42"); got != 2 {
		t.Fatalf("expected 2 entry headings, got %d:\n%s", got, out)
	}
}

func TestFormatEntry_OmitsEmptyMetadata(t *testing.T) {
	out := formatEntry(IdeaEntry{Text: "lonely idea", At: fixedTime()})

	for _, banned := range []string{"**session:**", "**project:**", "**claude session:**", "**last message line:**"} {
		if strings.Contains(out, banned) {
			t.Fatalf("entry with no metadata should not contain %q:\n%s", banned, out)
		}
	}
	if !strings.Contains(out, "lonely idea") {
		t.Fatalf("idea text missing:\n%s", out)
	}
}

func TestFormatEntry_FullMetadata(t *testing.T) {
	out := formatEntry(IdeaEntry{
		Text:            "improve fork sync conflict handling",
		Session:         "local-branch-work",
		Project:         "~/repos/agent-deck",
		Tool:            "claude",
		ClaudeSessionID: "0c2fe91",
		LastMessageLine: "want me to commit this to the local branch?",
		At:              fixedTime(),
	})

	wants := []string{
		"## 2026-06-14 15:42 — improve fork sync conflict handling",
		"- **session:** local-branch-work (claude)",
		"- **project:** ~/repos/agent-deck",
		"- **claude session:** 0c2fe91",
		"- **last message line:**",
		"want me to commit this to the local branch?",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Fatalf("expected entry to contain %q:\n%s", w, out)
		}
	}
}

func TestFormatEntry_SessionWithoutTool(t *testing.T) {
	out := formatEntry(IdeaEntry{Text: "x", Session: "s1", At: fixedTime()})
	if !strings.Contains(out, "- **session:** s1\n") {
		t.Fatalf("session without tool should render plainly:\n%s", out)
	}
	if strings.Contains(out, "s1 (") {
		t.Fatalf("no tool parens expected:\n%s", out)
	}
}

func TestHeadingTruncates(t *testing.T) {
	long := strings.Repeat("a", 200)
	h := heading(long)
	if !strings.HasSuffix(h, "…") {
		t.Fatalf("long heading should be truncated with ellipsis, got %q", h)
	}
	if len([]rune(h)) > 61 {
		t.Fatalf("heading too long: %d runes", len([]rune(h)))
	}
}

func TestAppendIdea_EmptyTextErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ideas.md")
	if err := appendIdeaTo(path, IdeaEntry{Text: "   ", At: fixedTime()}); err == nil {
		t.Fatal("expected error for empty idea text")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("no file should be created for empty idea, stat err: %v", err)
	}
}
