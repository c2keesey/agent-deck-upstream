package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// personalProjectsDir is the parent directory scanned for personal projects.
// Personal fork customization (mirrors maiaReposDir for the work picker).
const personalProjectsDir = "/Users/c2k/Projects"

// agentDeckDir is the agent-deck repo itself, pinned in the picker so a session
// on the fork's own source is always one keystroke away (it lives under ~/repos,
// not ~/Projects, so the scan below would never surface it otherwise).
const agentDeckDir = "/Users/c2k/repos/agent-deck"

// optiplexSSHCommand launches an interactive shell on the personal OptiPlex box
// (hostname c2k-optiplex, reached over Tailscale). Mirrors the `sshc` shell alias.
const (
	optiplexSSHCommand = "ssh c2k@100.82.177.26"
	optiplexLabel      = "optiplex"
)

// Tools the personal picker can create with. Shell is a first-class option here
// (unlike the MAIA picker, where shell is a side hotkey) because personal work
// is a mix of agent sessions and plain terminals.
const (
	personalToolClaude = "claude"
	personalToolCodex  = "codex"
	personalToolShell  = "shell"
)

// personalTargetKind classifies a row so Enter knows what to create.
const (
	personalKindHome    = "home"    // session rooted at the home dir, active tool
	personalKindSSH     = "ssh"     // shell session running optiplexSSHCommand
	personalKindProject = "project" // session rooted at a ~/Projects subdir, active tool
)

// personalTarget is one selectable row in the personal picker.
type personalTarget struct {
	label   string // display + filter text
	kind    string // personalKind*
	path    string // cwd for the session ("" → resolve home at create time)
	command string // explicit command (ssh); empty → derive from active tool
}

// PersonalPicker is the new-session picker for personal use. It lists the home
// dir, the OptiPlex SSH shortcut, and every directory under ~/Projects (most
// recently modified first), with a type-to-filter finder and a Claude/Codex/Shell
// tool switcher. Enter creates a session at the highlighted target with the
// active tool (the SSH row always runs ssh regardless of tool).
type PersonalPicker struct {
	visible  bool
	targets  []personalTarget // home, ssh, then projects (unfiltered, stable order)
	occupied map[string]bool  // project path -> already hosts a session
	filter   string
	cursor   int // index into the filtered slice
	tool     string
	width    int
	height   int
	scanErr  string
}

// NewPersonalPicker constructs an empty picker; projects are scanned on each
// Show() so freshly-created directories appear without a restart.
func NewPersonalPicker() *PersonalPicker { return &PersonalPicker{} }

// Show opens the picker. occupied maps project paths that already host an
// agent-deck session (any status), used only to tag rows with a busy dot.
func (m *PersonalPicker) Show(occupied map[string]bool) {
	m.visible = true
	m.occupied = occupied
	m.tool = personalToolClaude // default to Claude on every open
	m.filter = ""
	m.cursor = 0
	m.refreshTargets()
}

// Hide closes the picker.
func (m *PersonalPicker) Hide() { m.visible = false }

// IsVisible reports whether the picker is shown. Nil-safe so the key-router and
// View overlay checks don't panic on a Home built without a picker.
func (m *PersonalPicker) IsVisible() bool { return m != nil && m.visible }

// SetSize updates the viewport for centering.
func (m *PersonalPicker) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// ActiveTool returns the tool Enter will create with (Claude default).
func (m *PersonalPicker) ActiveTool() string {
	if m.tool == "" {
		return personalToolClaude
	}
	return m.tool
}

// ToggleTool cycles the active tool: Claude → Codex → Shell → Claude.
func (m *PersonalPicker) ToggleTool() {
	switch m.ActiveTool() {
	case personalToolClaude:
		m.tool = personalToolCodex
	case personalToolCodex:
		m.tool = personalToolShell
	default:
		m.tool = personalToolClaude
	}
}

// Selected returns the highlighted target. ok is false when the filtered list
// is empty (nothing to create).
func (m *PersonalPicker) Selected() (target personalTarget, ok bool) {
	filtered := m.filtered()
	if m.cursor < 0 || m.cursor >= len(filtered) {
		return personalTarget{}, false
	}
	return filtered[m.cursor], true
}

// filtered returns the targets matching the current filter (case-insensitive
// substring on the label). An empty filter returns all targets.
func (m *PersonalPicker) filtered() []personalTarget {
	if m.filter == "" {
		return m.targets
	}
	needle := strings.ToLower(m.filter)
	var out []personalTarget
	for _, t := range m.targets {
		if strings.Contains(strings.ToLower(t.label), needle) {
			out = append(out, t)
		}
	}
	return out
}

// Update handles navigation and filter typing. Enter/Esc/Tab are handled by the
// caller so they can trigger session creation / tool toggling.
func (m *PersonalPicker) Update(msg tea.KeyMsg) (*PersonalPicker, tea.Cmd) {
	switch msg.String() {
	case "up", "ctrl+p":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "ctrl+n":
		if m.cursor < len(m.filtered())-1 {
			m.cursor++
		}
	case "backspace":
		if m.filter != "" {
			m.filter = m.filter[:len(m.filter)-1]
			m.clampCursor()
		}
	default:
		// Printable single runes extend the filter. Anything else is ignored
		// so stray control keys don't corrupt the query.
		if r := []rune(msg.String()); len(r) == 1 && r[0] >= ' ' {
			m.filter += msg.String()
			m.clampCursor()
		}
	}
	return m, nil
}

// clampCursor keeps the cursor inside the filtered range after the list changes.
func (m *PersonalPicker) clampCursor() {
	n := len(m.filtered())
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// refreshTargets rebuilds the target list: home, the OptiPlex SSH shortcut, then
// every directory under ~/Projects sorted most-recently-modified first. Hidden
// directories (dotfiles) are skipped.
func (m *PersonalPicker) refreshTargets() {
	m.scanErr = ""
	m.targets = []personalTarget{
		{label: "~  (home / root dir)", kind: personalKindHome},
		{label: "agent-deck  (" + agentDeckDir + ")", kind: personalKindProject, path: agentDeckDir},
		{label: optiplexLabel + "  (" + optiplexSSHCommand + ")", kind: personalKindSSH, command: optiplexSSHCommand},
	}

	entries, err := os.ReadDir(personalProjectsDir)
	if err != nil {
		m.scanErr = err.Error()
		return
	}

	type dirInfo struct {
		path  string
		mtime int64
	}
	var dirs []dirInfo
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(personalProjectsDir, e.Name())
		var mtime int64
		if info, ierr := e.Info(); ierr == nil {
			mtime = info.ModTime().UnixNano()
		}
		dirs = append(dirs, dirInfo{path: path, mtime: mtime})
	}
	sort.SliceStable(dirs, func(i, j int) bool { return dirs[i].mtime > dirs[j].mtime })

	for _, d := range dirs {
		m.targets = append(m.targets, personalTarget{
			label: filepath.Base(d.path),
			kind:  personalKindProject,
			path:  d.path,
		})
	}
}

// View renders the tool switcher, filter line, and target list, centered.
func (m *PersonalPicker) View() string {
	if !m.visible {
		return ""
	}

	title := DialogTitleStyle.Render("New Personal Session")
	toolBar := m.renderToolSwitcher()

	filterStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	filterText := m.filter
	if filterText == "" {
		filterText = "(type to filter)"
	}
	filterLine := filterStyle.Render("Filter: ") + lipgloss.NewStyle().Foreground(ColorText).Render(filterText)

	var body string
	if m.scanErr != "" {
		body = lipgloss.NewStyle().Foreground(ColorRed).Render("⚠ " + m.scanErr)
	} else {
		body = m.renderList()
	}

	hint := lipgloss.NewStyle().Foreground(ColorComment).
		Render("↑/↓ pick · Tab tool · type filter · Enter create · Esc")

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", toolBar, "", filterLine, "", body, "", hint)
	dialog := DialogBoxStyle.Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
}

// renderToolSwitcher renders "Tool:  Claude  Codex  Shell" with the active tool
// highlighted (Tab cycles).
func (m *PersonalPicker) renderToolSwitcher() string {
	labelStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	active := lipgloss.NewStyle().Foreground(ColorBg).Background(ColorAccent).Bold(true)
	inactive := lipgloss.NewStyle().Foreground(ColorTextDim)

	style := func(tool string) lipgloss.Style {
		if m.ActiveTool() == tool {
			return active
		}
		return inactive
	}
	return labelStyle.Render("Tool: ") +
		style(personalToolClaude).Render(" Claude ") + "  " +
		style(personalToolCodex).Render(" Codex ") + "  " +
		style(personalToolShell).Render(" Shell ")
}

// renderList renders the filtered target rows inside a bordered box. Project rows
// already hosting a session are tagged with a busy dot.
func (m *PersonalPicker) renderList() string {
	const (
		colWidth = 40
		maxRows  = 16
	)

	rowStyle := lipgloss.NewStyle().Foreground(ColorText)
	selStyle := lipgloss.NewStyle().Foreground(ColorBg).Background(ColorAccent).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	busyStyle := lipgloss.NewStyle().Foreground(ColorTextDim)

	filtered := m.filtered()

	// Scroll window so the cursor stays visible in long lists.
	start := 0
	if m.cursor >= maxRows {
		start = m.cursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(filtered) {
		end = len(filtered)
	}

	var b strings.Builder
	if len(filtered) == 0 {
		b.WriteString(dimStyle.Render("(no matches)"))
	}
	for i := start; i < end; i++ {
		t := filtered[i]
		marker := ""
		if t.kind == personalKindProject && m.occupied[t.path] {
			marker = busyStyle.Render(" ·")
		}
		if i == m.cursor {
			b.WriteString(selStyle.Render(" " + t.label + " "))
		} else {
			b.WriteString(rowStyle.Render("  " + t.label))
		}
		b.WriteString(marker)
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	if end < len(filtered) {
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  …"))
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorAccent).
		Width(colWidth).
		Padding(0, 1)
	return box.Render(b.String())
}
