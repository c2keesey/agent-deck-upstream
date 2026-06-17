// Package main — `agent-deck idea` subcommand.
//
// Quick "by the way" idea capture. While attached to a Claude session, a tmux
// root-table bind (Ctrl+Alt+I, set up in internal/tmux on session start) floats
// a `display-popup` that runs this command. It snapshots the session context
// (project, Claude session id, last user message), prompts for one line, and
// appends to the global backlog (~/.agent-deck/ideas.md). Claude underneath is
// never sent any input. See docs/plans/2026-06-14-quick-note-capture.md.
//
// Usage:
//
//	agent-deck idea                      # interactive prompt (popup)
//	agent-deck idea --session <name>     # context from a specific tmux session
//	agent-deck idea "fix the flaky test" # non-interactive: text from args
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/asheshgoplani/agent-deck/internal/ideas"
	"github.com/asheshgoplani/agent-deck/internal/session"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func handleIdea(_ string, args []string) {
	fs := flag.NewFlagSet("idea", flag.ExitOnError)
	sessionFlag := fs.String("session", "", "tmux session name to capture context from (default: current session)")
	if err := fs.Parse(normalizeArgs(fs, args)); err != nil {
		os.Exit(1)
	}

	sessName := *sessionFlag
	if sessName == "" {
		sessName = currentTmuxSession()
	}

	// Snapshot session context at capture time so it reflects the moment the
	// idea struck, not whenever the entry is later read.
	entry := ideas.IdeaEntry{Session: sessName, At: time.Now()}
	if sessName != "" {
		entry.ClaudeSessionID = tmuxSessionEnv(sessName, "CLAUDE_SESSION_ID")
		entry.Project = tmuxDisplay(sessName, "#{pane_current_path}")
		if entry.ClaudeSessionID != "" {
			entry.Tool = "claude"
			if anchor, _ := session.LastClaudeChatAnchor(entry.ClaudeSessionID, entry.Project); anchor != "" {
				entry.LastMessageLine = anchor
			}
		}
	}

	// Idea text: positional args (non-interactive) win; otherwise prompt.
	text := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if text == "" {
		var captured bool
		text, captured = promptForIdea(sessName)
		if !captured || strings.TrimSpace(text) == "" {
			return // Esc / empty — write nothing.
		}
	}
	entry.Text = text

	if err := ideas.AppendIdea(entry); err != nil {
		fmt.Fprintf(os.Stderr, "idea: %v\n", err)
		os.Exit(1)
	}
}

// --- tmux context helpers -------------------------------------------------
//
// The popup runs inside the agent-deck tmux server (TMUX is set), so a bare
// `tmux` invocation targets the right socket automatically.

func currentTmuxSession() string {
	out, err := exec.Command("tmux", "display-message", "-p", "#{session_name}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func tmuxDisplay(sessionName, format string) string {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", sessionName, format).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func tmuxSessionEnv(sessionName, key string) string {
	out, err := exec.Command("tmux", "show-environment", "-t", sessionName, key).Output()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(out))
	// "KEY=value" when set; "-KEY" when explicitly unset.
	if eq := strings.IndexByte(line, '='); eq >= 0 {
		return line[eq+1:]
	}
	return ""
}

// --- interactive prompt ---------------------------------------------------

type ideaPromptModel struct {
	ti        textinput.Model
	session   string
	submitted bool
	cancelled bool
}

func (m ideaPromptModel) Init() tea.Cmd { return textinput.Blink }

func (m ideaPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyEnter:
			m.submitted = true
			return m, tea.Quit
		case tea.KeyEsc, tea.KeyCtrlC:
			m.cancelled = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	return m, cmd
}

func (m ideaPromptModel) View() string {
	header := "💡 Capture idea"
	if m.session != "" {
		header += "  ·  " + m.session
	}
	return fmt.Sprintf("\n  %s\n\n  %s\n\n  enter save · esc cancel\n", header, m.ti.View())
}

// promptForIdea runs the one-line capture prompt. Returns (text, captured)
// where captured is false on cancel/error.
func promptForIdea(sessionName string) (string, bool) {
	ti := textinput.New()
	ti.Placeholder = "what's the idea?"
	ti.Prompt = "▸ "
	ti.Width = 60
	ti.Focus()

	out, err := tea.NewProgram(ideaPromptModel{ti: ti, session: sessionName}).Run()
	if err != nil {
		return "", false
	}
	final, ok := out.(ideaPromptModel)
	if !ok || final.cancelled || !final.submitted {
		return "", false
	}
	return final.ti.Value(), true
}
