package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// writeTranscript places a Claude transcript for (projectPath, claudeSessionID)
// under a fake $HOME the way Claude Code does:
// ~/.claude/projects/<slugified-path>/<session-id>.jsonl
func writeTranscript(t *testing.T, home, projectPath, claudeSessionID string, lines ...string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", claudeProjectSlug(projectPath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, claudeSessionID+".jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func compoundTestInstance(projectPath string) *session.Instance {
	return &session.Instance{
		ID:              "s1",
		Title:           "Session 1",
		Tool:            "claude",
		ProjectPath:     projectPath,
		ClaudeSessionID: "11111111-2222-3333-4444-555555555555",
	}
}

const (
	userMsgPlain    = `{"type":"user","message":{"role":"user","content":"fix the bug in teardown"}}`
	userMsgCompound = `{"type":"user","message":{"role":"user","content":"<command-name>/compound</command-name> document the teardown fix"}}`
	assistantMsg    = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}`
	// An assistant merely *mentioning* /compound must not count as it having run.
	assistantMentionsCompound = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"you could run /compound later"}]}}`
)

// TestForgotCompoundSkill pins the fork feature: before removing (d/teardown)
// or closing (ctrl+x) a Claude session, warn when no user message in its
// transcript ever invoked the /compound skill — the learnings were never
// documented and are about to be thrown away with the session.
func TestForgotCompoundSkill(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, home string, inst *session.Instance)
		want  bool
	}{
		{
			name: "user messages without compound → warn",
			setup: func(t *testing.T, home string, inst *session.Instance) {
				writeTranscript(t, home, inst.ProjectPath, inst.ClaudeSessionID, userMsgPlain, assistantMsg)
			},
			want: true,
		},
		{
			name: "user ran /compound → no warn",
			setup: func(t *testing.T, home string, inst *session.Instance) {
				writeTranscript(t, home, inst.ProjectPath, inst.ClaudeSessionID, userMsgPlain, assistantMsg, userMsgCompound)
			},
			want: false,
		},
		{
			name: "assistant mentioning /compound doesn't count → warn",
			setup: func(t *testing.T, home string, inst *session.Instance) {
				writeTranscript(t, home, inst.ProjectPath, inst.ClaudeSessionID, userMsgPlain, assistantMentionsCompound)
			},
			want: true,
		},
		{
			name:  "no transcript on disk → stay quiet",
			setup: func(t *testing.T, home string, inst *session.Instance) {},
			want:  false,
		},
		{
			name: "transcript with no user activity → stay quiet",
			setup: func(t *testing.T, home string, inst *session.Instance) {
				writeTranscript(t, home, inst.ProjectPath, inst.ClaudeSessionID, assistantMsg)
			},
			want: false,
		},
		{
			name: "non-claude tool → stay quiet",
			setup: func(t *testing.T, home string, inst *session.Instance) {
				writeTranscript(t, home, inst.ProjectPath, inst.ClaudeSessionID, userMsgPlain)
				inst.Tool = "codex"
			},
			want: false,
		},
		{
			name: "no recorded claude session id → stay quiet",
			setup: func(t *testing.T, home string, inst *session.Instance) {
				writeTranscript(t, home, inst.ProjectPath, inst.ClaudeSessionID, userMsgPlain)
				inst.ClaudeSessionID = ""
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			inst := compoundTestInstance(t.TempDir())
			tc.setup(t, home, inst)
			if got := forgotCompoundSkill(inst); got != tc.want {
				t.Fatalf("forgotCompoundSkill() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCompoundWarning_SurfacesInModal drives the real hotkeys: d (delete /
// MAIA teardown) and ctrl+x (close) on a Claude session whose transcript has
// no /compound user message must render the warning inside the confirmation
// modal; the warning must reset when the dialog is reused for a clean session.
func TestCompoundWarning_SurfacesInModal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	keys := map[string]tea.KeyMsg{
		"d":      {Type: tea.KeyRunes, Runes: []rune{'d'}},
		"ctrl+x": {Type: tea.KeyCtrlX, Runes: []rune{}},
	}
	for _, tc := range []struct {
		name        string
		key         string
		projectPath string
		wantType    ConfirmType
	}{
		{"delete modal", "d", t.TempDir(), ConfirmDeleteSession},
		{"teardown modal", "d", maiaReposDir + "/MAIA.unit-test-compound-warn", ConfirmTeardownSession},
		{"close modal", "ctrl+x", t.TempDir(), ConfirmCloseSession},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inst := compoundTestInstance(tc.projectPath)
			writeTranscript(t, home, tc.projectPath, inst.ClaudeSessionID, userMsgPlain, assistantMsg)
			items := []session.Item{{Type: session.ItemTypeSession, Session: inst, Level: 0}}
			h := newTestHomeWithItems(100, 40, items)
			h.cursor = 0

			model, _ := h.handleMainKey(keys[tc.key])
			h2 := model.(*Home)
			if !h2.confirmDialog.IsVisible() || h2.confirmDialog.GetConfirmType() != tc.wantType {
				t.Fatalf("expected visible confirm type %v, got visible=%v type=%v",
					tc.wantType, h2.confirmDialog.IsVisible(), h2.confirmDialog.GetConfirmType())
			}
			if view := h2.confirmDialog.View(); !strings.Contains(view, "/compound") {
				t.Fatalf("modal must surface the missing-/compound warning; view:\n%s", view)
			}

			// A session that DID run /compound must not warn — the flag must
			// not leak from the previous dialog use.
			h2.confirmDialog.Hide()
			writeTranscript(t, home, tc.projectPath, inst.ClaudeSessionID, userMsgPlain, userMsgCompound)
			model, _ = h2.handleMainKey(keys[tc.key])
			h3 := model.(*Home)
			if view := h3.confirmDialog.View(); strings.Contains(view, "/compound") {
				t.Fatalf("no warning expected once /compound was run; view:\n%s", view)
			}
		})
	}
}
