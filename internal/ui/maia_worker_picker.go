package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// maiaReposDir is the parent directory scanned for MAIA worktrees.
// Personal fork customization.
const maiaReposDir = "/Users/c2k/MAIA/Repos"

// maiaMainRepo is the primary MAIA checkout — the repo root that ad-hoc
// worktrees are created from (git worktree add runs here, and the
// .agent-deck/worktree-setup.sh / worktree-destruction.sh hooks are read
// from this directory).
const maiaMainRepo = "/Users/c2k/MAIA/Repos/MAIA"

// maiaAdhocBranchPrefix namespaces the holding branch every ad-hoc worktree
// starts on (wt/<session-name>, based off fresh origin/dev by CreateWorktree).
// The worker's own pipeline (/lfg) checks out its real ck/MAIA-XXXX feature
// branch inside the worktree; this branch only exists so the worktree has a
// unique, disposable ref. The destruction hook names its recovery ref after
// the worktree dir, not this branch.
const maiaAdhocBranchPrefix = "wt/"

// Groups the picker pins new sessions to, per the user's workflow:
// workers land in maia/active, read-only dev sessions in maia/read-only-dev.
const (
	maiaWorkerGroup    = "maia/active"
	maiaRoDevGroup     = "maia/read-only-dev"
	maiaConductorGroup = "maia/conductor"
	maiaRemoteGroup    = "maia/remote"
)

// maiaRemoteScript is the agent-deck -cmd body for the 'R' (remote) hotkey: it
// creates a fresh ephemeral worktree on the shared dev box and attaches an
// interactive remote Claude TUI in it. It ships self-contained in this repo's
// scripts/ (no MAIA cross-repo path dependency); it only mirrors the MAIA skill's
// ephemeral-marker schema so remote-prune/remote-reclaim can clean up after it.
// Hardcoded path, matching maiaReposDir's personal-fork style.
const maiaRemoteScript = "/Users/c2k/repos/agent-deck/scripts/remote-claude-new"

// maiaReclaimBoxScript tears down the BOX side of a remote session (make-recoverable
// + remote-prune) without removing the local agent-deck session. deleteSession runs
// it when a maia/remote* session is deleted via the TUI, so the box worktree + tmux
// don't leak. Self-contained sibling of remote-claude-new in this repo's scripts/.
const maiaReclaimBoxScript = "/Users/c2k/repos/agent-deck/scripts/remote-reclaim-box"

// parseRemoteLaneID extracts the lane and ephemeral id from a remote session's stored
// command (`…/remote-claude-new <lane> <id> [model]`), so the delete hook can target
// the right box worktree. ok is false when the command isn't a remote-claude-new
// launch — the caller then skips the box teardown.
func parseRemoteLaneID(command string) (lane, id string, ok bool) {
	if !strings.Contains(command, "remote-claude-new") {
		return "", "", false
	}
	fields := strings.Fields(command)
	for i, f := range fields {
		if strings.HasSuffix(f, "remote-claude-new") && i+2 < len(fields) {
			return fields[i+1], fields[i+2], true
		}
	}
	return "", "", false
}

// resolveRemoteLane picks the box lane name for a remote session, mirroring the
// remote-* scripts' default chain (MAIA_REMOTE_LANE → $USER → "remote") so a
// picker-launched session lands in the same ephemeral worktree namespace as a
// scripted dispatch.
func resolveRemoteLane(envLane, envUser string) string {
	if envLane != "" {
		return envLane
	}
	if envUser != "" {
		return envUser
	}
	return "remote"
}

// MaiaWorkerPicker is the new-session dialog for MAIA work. The old model —
// browse a fixed pool of MAIA.worker-N worktrees and rotate through free
// slots — is retired: Enter now creates a FRESH ad-hoc worktree
// (MAIA.<session-name>, holding branch wt/<session-name> off origin/dev) via
// agent-deck's native worktree machinery, which runs the MAIA repo's
// .agent-deck/worktree-setup.sh hook (full dev setup) after git worktree add,
// and removes the worktree again when the session is deleted (destruction
// hook makes dirty/unpushed work recoverable first).
//
// The dialog therefore has no worker column: it is a tool switcher plus
// action hotkeys. 'c' (conductor) and 'r' (ro-dev) still target the shared
// long-lived worktrees; 'R' creates a remote box ephemeral; 's' creates a
// fresh worktree with a plain shell instead of a tool.
type MaiaWorkerPicker struct {
	visible    bool
	roDevs     []string // ro-dev worktree paths
	conductors []string // MAIA.conductor worktree paths (personal fork)
	tool       string   // active tool the action keys create with ("claude" | "codex")

	width   int
	height  int
	scanErr string
}

// NewMaiaWorkerPicker constructs an empty picker; the shared worktrees are
// scanned on each Show() so freshly-created worktrees appear without a restart.
func NewMaiaWorkerPicker() *MaiaWorkerPicker { return &MaiaWorkerPicker{} }

// Show opens the picker.
func (m *MaiaWorkerPicker) Show() {
	m.visible = true
	m.tool = maiaToolClaude // default to Claude on every open
	m.refreshWorktrees()
}

// Tools the picker can create with. Codex always launches YOLO in the MAIA
// flow (see createMaiaWorkerSession); Claude is the default.
const (
	maiaToolClaude = "claude"
	maiaToolCodex  = "codex"
)

// ActiveTool returns the tool the action keys (Enter, r, ~) will create with.
func (m *MaiaWorkerPicker) ActiveTool() string {
	if m.tool == "" {
		return maiaToolClaude
	}
	return m.tool
}

// ToggleTool flips the active tool between Claude and Codex.
func (m *MaiaWorkerPicker) ToggleTool() {
	if m.ActiveTool() == maiaToolCodex {
		m.tool = maiaToolClaude
	} else {
		m.tool = maiaToolCodex
	}
}

// ConductorSelected returns the MAIA.conductor worktree path and group, used by
// the 'c' hotkey to create a session in the conductor worktree directly (no
// column to browse). Empty path means no conductor worktree exists.
func (m *MaiaWorkerPicker) ConductorSelected() (path, group string) {
	if len(m.conductors) > 0 {
		return m.conductors[0], maiaConductorGroup
	}
	return "", ""
}

// Hide closes the picker.
func (m *MaiaWorkerPicker) Hide() { m.visible = false }

// IsVisible reports whether the picker is shown. Nil-safe so the key-router
// and View overlay checks don't panic on a Home built without a picker.
func (m *MaiaWorkerPicker) IsVisible() bool { return m != nil && m.visible }

// SetSize updates the viewport for centering.
func (m *MaiaWorkerPicker) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// NewWorktreeSpec picks the identity for a fresh ad-hoc worktree session: a
// generated session name that is unique among live instances AND whose
// MAIA.<name> directory does not already exist on disk (a stale dir from an
// interrupted teardown must not be silently reused as if fresh). Returns the
// session name, the worktree path, and the holding branch.
func NewWorktreeSpec(instances []*session.Instance, group string) (name, worktreePath, branch string, err error) {
	for attempt := 0; attempt < 20; attempt++ {
		name = session.GenerateUniqueSessionName(instances, group)
		worktreePath = filepath.Join(maiaReposDir, "MAIA."+name)
		if _, statErr := os.Stat(worktreePath); os.IsNotExist(statErr) {
			return name, worktreePath, maiaAdhocBranchPrefix + name, nil
		}
	}
	return "", "", "", fmt.Errorf("could not find an unused MAIA.<name> directory in %s after 20 attempts", maiaReposDir)
}

// RoDevSelected returns the shared ro-dev worktree path and group, used by the
// 'r' hotkey to create a read-only dev session directly (no column to browse).
// Empty path means no ro-dev worktree exists.
func (m *MaiaWorkerPicker) RoDevSelected() (path, group string) {
	if len(m.roDevs) > 0 {
		return m.roDevs[0], maiaRoDevGroup
	}
	return "", ""
}

// Update handles keys the caller doesn't route (there is no cursor anymore;
// navigation keys are no-ops).
func (m *MaiaWorkerPicker) Update(msg tea.KeyMsg) (*MaiaWorkerPicker, tea.Cmd) {
	return m, nil
}

// refreshWorktrees scans maiaReposDir for the shared long-lived worktrees the
// hotkeys target (MAIA.ro-dev*, MAIA.conductor). Ad-hoc worktrees are created
// on demand, not browsed, so they are not collected here.
func (m *MaiaWorkerPicker) refreshWorktrees() {
	m.roDevs = nil
	m.conductors = nil
	m.scanErr = ""
	entries, err := os.ReadDir(maiaReposDir)
	if err != nil {
		m.scanErr = err.Error()
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		path := filepath.Join(maiaReposDir, name)
		switch {
		case name == "MAIA.ro-dev" || strings.HasPrefix(name, "MAIA.ro-dev"):
			m.roDevs = append(m.roDevs, path)
		case name == "MAIA.conductor":
			m.conductors = append(m.conductors, path)
		}
	}
	sort.Strings(m.roDevs)
}

// View renders the action dialog, centered.
func (m *MaiaWorkerPicker) View() string {
	if !m.visible {
		return ""
	}

	title := DialogTitleStyle.Render("New MAIA Session")

	toolBar := m.renderToolSwitcher()

	var body string
	if m.scanErr != "" {
		body = lipgloss.NewStyle().Foreground(ColorRed).Render("⚠ " + m.scanErr)
	} else {
		accent := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
		dim := lipgloss.NewStyle().Foreground(ColorTextDim)
		body = accent.Render("⏎  new worktree session") + "\n" +
			dim.Render("   fresh MAIA.<name> off origin/dev · full setup runs")
	}

	hintText := "Enter new worktree · Tab tool · c conductor · r ro-dev · R remote · s shell · ~ home · Esc"
	hint := lipgloss.NewStyle().Foreground(ColorComment).Render(hintText)

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", toolBar, "", body, "", hint)
	dialog := DialogBoxStyle.Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
}

// renderToolSwitcher renders the "Tool:  Claude  Codex" selector with the
// active tool highlighted, so the user can see (and toggle with Tab) which
// tool the action keys will create with.
func (m *MaiaWorkerPicker) renderToolSwitcher() string {
	labelStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	active := lipgloss.NewStyle().Foreground(ColorBg).Background(ColorAccent).Bold(true)
	inactive := lipgloss.NewStyle().Foreground(ColorTextDim)

	claude, codex := inactive, inactive
	if m.ActiveTool() == maiaToolCodex {
		codex = active
	} else {
		claude = active
	}
	return labelStyle.Render("Tool: ") +
		claude.Render(" Claude ") + "  " + codex.Render(" Codex ")
}
