package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAssistantTranscript(t *testing.T, configDir, projectPath, sessionID string, lines []string) {
	t.Helper()
	resolved := projectPath
	if r, err := filepath.EvalSymlinks(projectPath); err == nil {
		resolved = r
	}
	dir := filepath.Join(configDir, "projects", ConvertToClaudeDirName(resolved))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLastClaudeChatAnchor_StringContent(t *testing.T) {
	configDir := t.TempDir()
	project := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)

	writeAssistantTranscript(t, configDir, project, "sess1", []string{
		`{"message":{"role":"assistant","content":"first reply"}}`,
		`{"message":{"role":"user","content":"a question"}}`,
		`{"message":{"role":"assistant","content":"latest reply"}}`,
	})

	got, err := LastClaudeChatAnchor("sess1", project)
	if err != nil {
		t.Fatal(err)
	}
	if got != "latest reply" {
		t.Fatalf("want %q, got %q", "latest reply", got)
	}
}

func TestLastClaudeChatAnchor_ContentBlocks(t *testing.T) {
	configDir := t.TempDir()
	project := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)

	writeAssistantTranscript(t, configDir, project, "sess2", []string{
		`{"message":{"role":"assistant","content":[{"type":"text","text":"block one"},{"type":"text","text":"block two"}]}}`,
	})

	got, err := LastClaudeChatAnchor("sess2", project)
	if err != nil {
		t.Fatal(err)
	}
	// The anchor is only the LAST line of the agent's message.
	if got != "block two" {
		t.Fatalf("want last line %q, got %q", "block two", got)
	}
}

// When the user typed last (note captured before the agent replied), the anchor
// is the user's line — not the prior assistant message.
func TestLastClaudeChatAnchor_LastIsUserLine(t *testing.T) {
	configDir := t.TempDir()
	project := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)

	writeAssistantTranscript(t, configDir, project, "sess4", []string{
		`{"message":{"role":"assistant","content":"agent said something"}}`,
		`{"message":{"role":"user","content":"and the user just\nreplied here"}}`,
	})

	got, err := LastClaudeChatAnchor("sess4", project)
	if err != nil {
		t.Fatal(err)
	}
	if got != "replied here" {
		t.Fatalf("want last line of the user message, got %q", got)
	}
}

// A tool_result record (role:user, no text block) must not become the anchor —
// the genuine prior user/assistant text line should win.
func TestLastClaudeChatAnchor_SkipsToolResult(t *testing.T) {
	configDir := t.TempDir()
	project := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)

	writeAssistantTranscript(t, configDir, project, "sess5", []string{
		`{"message":{"role":"assistant","content":"agent final words"}}`,
		`{"message":{"role":"user","content":[{"type":"tool_result","content":"exit 0"}]}}`,
	})

	got, err := LastClaudeChatAnchor("sess5", project)
	if err != nil {
		t.Fatal(err)
	}
	if got != "agent final words" {
		t.Fatalf("tool_result should be skipped, want %q, got %q", "agent final words", got)
	}
}

// A subagent (sidechain) reply must not be mistaken for the agent's own last
// message.
func TestLastClaudeChatAnchor_SkipsSidechain(t *testing.T) {
	configDir := t.TempDir()
	project := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)

	writeAssistantTranscript(t, configDir, project, "sess3", []string{
		`{"message":{"role":"assistant","content":"parent agent reply"}}`,
		`{"isSidechain":true,"message":{"role":"assistant","content":"subagent output"}}`,
	})

	got, err := LastClaudeChatAnchor("sess3", project)
	if err != nil {
		t.Fatal(err)
	}
	if got != "parent agent reply" {
		t.Fatalf("want parent reply, got %q", got)
	}
}

func TestLastClaudeChatAnchor_MissingTranscript(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	got, err := LastClaudeChatAnchor("nope", t.TempDir())
	if err != nil {
		t.Fatalf("missing transcript should not error, got %v", err)
	}
	if got != "" {
		t.Fatalf("want empty for missing transcript, got %q", got)
	}
}

func TestLastClaudeChatAnchor_EmptySessionID(t *testing.T) {
	got, err := LastClaudeChatAnchor("", t.TempDir())
	if err != nil || got != "" {
		t.Fatalf("empty session id should yield (\"\", nil), got (%q, %v)", got, err)
	}
}
