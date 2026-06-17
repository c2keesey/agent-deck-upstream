package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// switcherIdleCommit is how long the switcher waits after the last Ctrl+S /
// Ctrl+A before auto-committing to the highlighted session. It approximates
// "switch when I let go of the key" — terminals do not deliver key-release
// events, so we commit on a brief idle instead. Enter commits immediately; Esc
// cancels; arrow-key navigation cancels the auto-commit (manual mode).
const switcherIdleCommit = 1 * time.Second

// switcherRepeatGuard is the minimum gap between accepted Ctrl+S / Ctrl+A
// advances. Terminal auto-repeat fires far faster than this (~15–40ms), so
// holding the key down advances at most a step or two instead of spinning
// through every session; deliberate taps (~100ms+ apart) all register.
const switcherRepeatGuard = 80 * time.Millisecond

// SessionSwitcher is the session switcher overlay. It opens on Ctrl+S — both
// while attached (the tmux attach loop hands control back to the TUI) and from
// the overview — pre-highlighted on the session you came from. Ctrl+S / Ctrl+A
// cycle forward / backward, arrow keys browse, and the highlight is attached on
// Enter or after a brief idle once you've cycled.
type SessionSwitcher struct {
	visible          bool
	width, height    int
	sessions         []*session.Instance // active sessions, MRU-ordered
	cursor           int
	fromID           string                  // session the picker was opened from
	subtitles        map[string]string       // sessionID -> dim conversation/pane title (matches the overview)
	labels           map[string]primaryLabel // sessionID -> dashboard primary label (name/branch/folder/broadcast); set by openSessionSwitcher
	reattachOnCancel bool                    // Esc re-attaches to fromID (opened while attached) vs. just closing (opened from the overview)
	// commitGen is bumped on every open/cycle/cancel so a stale idle-commit
	// timer (scheduled before a later keypress) is ignored when it fires. It is
	// intentionally monotonic — never reset — so a timer from a previous
	// switcher session can never collide with a new one.
	commitGen int
	// lastCycleAt is the time of the last accepted Ctrl+S / Ctrl+A advance, used
	// to swallow terminal key-repeat (see switcherRepeatGuard).
	lastCycleAt time.Time
}

// bumpCommitGen advances and returns the commit generation. armSwitcherCommit
// schedules a timer tagged with the returned value; calling it WITHOUT
// scheduling a new timer (e.g. on arrow navigation) simply invalidates any
// pending auto-commit. Only a timer carrying the current generation commits
// (see Home.handleSwitcherCommit).
func (s *SessionSwitcher) bumpCommitGen() int {
	s.commitGen++
	return s.commitGen
}

// cycle advances the highlight one step (forward => next, else prev) unless the
// previous accepted advance was within switcherRepeatGuard, which swallows
// key-repeat from a held Ctrl+S / Ctrl+A. It reports whether it moved.
func (s *SessionSwitcher) cycle(forward bool, now time.Time) bool {
	if !s.lastCycleAt.IsZero() && now.Sub(s.lastCycleAt) < switcherRepeatGuard {
		return false
	}
	s.lastCycleAt = now
	if forward {
		s.next()
	} else {
		s.prev()
	}
	return true
}

// NewSessionSwitcher creates a new (hidden) session switcher.
func NewSessionSwitcher() *SessionSwitcher { return &SessionSwitcher{} }

// Show builds the MRU-ordered list of switchable sessions and pre-selects the
// session the picker was opened from (fromID), so an immediate Enter drops the
// user right back where they were and Ctrl+S/Ctrl+A step away from there.
// subtitles maps a session ID to its dim conversation/pane title (the same text
// the overview shows next to an entry); a nil map renders no subtitles. It
// returns false (and stays hidden) when fewer than two sessions are available —
// there is nothing to switch between, so the caller falls back to a normal detach.
//
// Scope: the switcher is local-only by design — it takes local
// *session.Instance rows and re-attaches via the local tmux attach loop. Remote
// (SSH) sessions reach a session over a different attach path, so they are
// intentionally excluded from the picker for now (see
// TestSessionSwitcher_RemoteSessionsUnsupported); supporting them needs a remote
// re-attach path and is tracked as a follow-up.
func (s *SessionSwitcher) Show(fromID string, allInstances []*session.Instance, subtitles map[string]string) bool {
	list := make([]*session.Instance, 0, len(allInstances))
	for _, inst := range allInstances {
		if inst == nil {
			continue
		}
		// Mirror the send-output picker: only switchable (live) sessions.
		switch inst.GetStatusThreadSafe() {
		case session.StatusError, session.StatusStopped:
			continue
		}
		list = append(list, inst)
	}
	if len(list) < 2 {
		// Nothing to switch between. Clear any prior selection so a switcher that
		// was already open (e.g. live-session count just dropped below two) does
		// not linger on stale state — Show's contract is "stays hidden" here.
		s.Hide()
		return false
	}

	// Most-recently-accessed first. The just-detached session was
	// MarkAccessed'd on detach, so it sorts to the front — pre-selecting it
	// means the first Ctrl+S step lands on the most-recent other session.
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].LastAccessedAt.After(list[j].LastAccessedAt)
	})

	cursor := 0
	for i, inst := range list {
		if inst.ID == fromID {
			cursor = i
			break
		}
	}

	s.visible = true
	s.sessions = list
	s.cursor = cursor
	s.fromID = fromID
	s.subtitles = subtitles
	return true
}

// Hide closes the switcher and resets state. commitGen is intentionally left
// untouched (monotonic) so a pending timer from this session can't commit after
// a future re-open.
func (s *SessionSwitcher) Hide() {
	s.visible = false
	s.cursor = 0
	s.sessions = nil
	s.fromID = ""
	s.subtitles = nil
	s.labels = nil
	s.reattachOnCancel = false
	s.lastCycleAt = time.Time{}
}

// IsVisible reports whether the switcher is currently shown.
func (s *SessionSwitcher) IsVisible() bool { return s != nil && s.visible }

// SetSize updates the dimensions used for centering.
func (s *SessionSwitcher) SetSize(w, h int) {
	s.width = w
	s.height = h
}

// GetSelected returns the highlighted session, or nil.
func (s *SessionSwitcher) GetSelected() *session.Instance {
	if len(s.sessions) == 0 || s.cursor < 0 || s.cursor >= len(s.sessions) {
		return nil
	}
	return s.sessions[s.cursor]
}

func (s *SessionSwitcher) next() {
	if len(s.sessions) > 0 {
		s.cursor = (s.cursor + 1) % len(s.sessions)
	}
}

func (s *SessionSwitcher) prev() {
	if len(s.sessions) > 0 {
		s.cursor = (s.cursor - 1 + len(s.sessions)) % len(s.sessions)
	}
}

// primaryLabelStyle styles a switcher row's identity label by the same rules the
// dashboard row render uses, so the switcher matches the list: a real name uses
// the plain text color, a distinguishing branch is purple, a worktree folder
// takes its own worktree color, and a live broadcast / auto name fades to dim.
// The selected row gets the accent highlight bar.
func primaryLabelStyle(lbl primaryLabel, selected bool) lipgloss.Style {
	switch {
	case selected:
		return SessionTitleSelStyle
	case lbl.kind == primaryBranch:
		return lipgloss.NewStyle().Foreground(ColorPurple).Bold(true)
	case lbl.kind == primaryFolder:
		return lipgloss.NewStyle().Foreground(worktreeColor(lbl.text)).Bold(true)
	case lbl.kind == primaryBroadcast, lbl.kind == primaryAuto:
		return lipgloss.NewStyle().Foreground(ColorTextDim)
	default: // primaryName
		return lipgloss.NewStyle().Foreground(ColorText)
	}
}

// switcherMaxVisibleRows caps how many session rows render at once; when there
// are more, a window centered on the cursor scrolls with "↑/↓ N more" markers.
const switcherMaxVisibleRows = 12

// View renders the centered switcher box.
func (s *SessionSwitcher) View() string {
	if !s.visible {
		return ""
	}

	dialogWidth := 70
	if s.width > 0 && s.width < dialogWidth+8 {
		dialogWidth = s.width - 8
	}
	if dialogWidth < 40 {
		dialogWidth = 40
	}
	// Content area inside the rounded border + Padding(1,2): horizontal padding
	// eats 4 cells.
	contentWidth := dialogWidth - 4
	if contentWidth < 24 {
		contentWidth = 24
	}

	total := len(s.sessions)

	// Name column width: the widest label, clamped so the live-activity subtitle
	// always keeps room. Aligns tools + subtitles into clean columns.
	nameCol := 0
	for _, inst := range s.sessions {
		if w := cellWidth(s.labelText(inst)); w > nameCol {
			nameCol = w
		}
	}
	// Cap so one long label (often a live-broadcast activity sentence) can't push
	// the subtitle column off the row; 24 keeps names compact yet readable.
	nameCol = max(min(nameCol, min(24, contentWidth/2)), 10)

	// --- header: title on the left, position on the right, then a rule ---
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	head := titleStyle.Render("⇄  Switch Session")
	pos := DimStyle.Render(fmt.Sprintf("%d/%d", s.cursor+1, total))
	gap := max(contentWidth-cellWidth(head)-cellWidth(pos), 1)

	var lines []string
	lines = append(lines, head+strings.Repeat(" ", gap)+pos)
	lines = append(lines, lipgloss.NewStyle().Foreground(ColorBorder).Render(strings.Repeat("─", contentWidth)))

	// --- scroll window centered on the cursor ---
	start, end := 0, total
	if total > switcherMaxVisibleRows {
		start = max(s.cursor-switcherMaxVisibleRows/2, 0)
		end = start + switcherMaxVisibleRows
		if end > total {
			end = total
			start = end - switcherMaxVisibleRows
		}
	}
	if start > 0 {
		lines = append(lines, DimStyle.Render(fmt.Sprintf("  ↑ %d more", start)))
	}

	selBar := lipgloss.NewStyle().Background(ColorAccent).Foreground(ColorBg).Bold(true)

	for i := start; i < end; i++ {
		inst := s.sessions[i]
		selected := i == s.cursor
		status := inst.GetStatusThreadSafe()

		// Identity label: reuse the dashboard's primary-label engine (name >
		// distinguishing branch > worktree folder > live broadcast > auto) so
		// the switcher shows the same names + colors as the list.
		lbl, ok := s.labels[inst.ID]
		if !ok || lbl.text == "" {
			lbl = primaryLabel{text: inst.Title, kind: primaryName}
		}
		name := lbl.text
		if cellWidth(name) > nameCol {
			name = cellTruncate(name, nameCol, "…")
		}

		// Tool name — hidden for the default "claude" (matches the dashboard).
		toolName := ""
		if inst.Tool != "" && inst.Tool != "claude" {
			toolName = inst.Tool
		}

		// Live-activity subtitle, unless the broadcast already won the label.
		sub := ""
		if lbl.kind != primaryBroadcast {
			sub = s.subtitles[inst.ID]
		}

		if selected {
			// One uniform accent bar: build the row as plain text (so the bar's
			// background paints evenly) and pad it to the full content width.
			row := "▸ " + statusGlyph(status) + " " + padCells(name, nameCol)
			used := 2 + 1 + 1 + nameCol
			if toolName != "" {
				row += " " + toolName
				used += 1 + cellWidth(toolName)
			}
			if sub != "" {
				if rem := contentWidth - used - 2; rem >= 6 {
					row += "  " + cellTruncate(sub, rem, "…")
				}
			}
			lines = append(lines, selBar.Render(padCells(row, contentWidth)))
			continue
		}

		// Non-selected: per-kind colors, columns aligned.
		row := "  " + statusIndicator(status) + " " +
			primaryLabelStyle(lbl, false).Render(name) + strings.Repeat(" ", max(0, nameCol-cellWidth(name)))
		used := 2 + 1 + 1 + nameCol
		if toolName != "" {
			row += GetToolStyle(toolName).Render(" " + toolName)
			used += 1 + cellWidth(toolName)
		}
		if sub != "" {
			if rem := contentWidth - used - 2; rem >= 6 {
				row += "  " + DimStyle.Render(cellTruncate(sub, rem, "…"))
			}
		}
		lines = append(lines, row)
	}

	if end < total {
		lines = append(lines, DimStyle.Render(fmt.Sprintf("  ↓ %d more", total-end)))
	}

	// --- footer: keycap hints ---
	lines = append(lines, "")
	lines = append(lines, switcherFooter(s.reattachOnCancel))

	content := strings.Join(lines, "\n")
	box := DialogBoxStyle.Width(dialogWidth).Render(content)
	return centerInScreen(box, s.width, s.height)
}

// labelText returns the precomputed primary-label text for inst, or its raw
// title as a fallback (e.g. before openSessionSwitcher set the labels, or in tests).
func (s *SessionSwitcher) labelText(inst *session.Instance) string {
	if lbl, ok := s.labels[inst.ID]; ok && lbl.text != "" {
		return lbl.text
	}
	return inst.Title
}

// statusGlyph is the uncolored status bullet, for the selected row's accent bar
// where a single background style paints the whole line.
func statusGlyph(st session.Status) string {
	switch st {
	case session.StatusRunning:
		return "●"
	case session.StatusWaiting:
		return "◐"
	case session.StatusIdle:
		return "○"
	default:
		return "✕"
	}
}

// padCells right-pads s with spaces to exactly target display cells, truncating
// with an ellipsis if it is longer. Keeps switcher columns aligned.
func padCells(s string, target int) string {
	if w := cellWidth(s); w < target {
		return s + strings.Repeat(" ", target-w)
	}
	return cellTruncate(s, target, "…")
}

// switcherFooter renders the key hints as keycaps (bright keys, dim labels).
// The Esc hint reflects context: "esc back" re-attaches to the origin session
// when the picker was opened while attached (reattachOnCancel), otherwise
// "esc close" since opening from the overview just dismisses the picker.
func switcherFooter(reattachOnCancel bool) string {
	key := lipgloss.NewStyle().Foreground(ColorText).Bold(true)
	desc := lipgloss.NewStyle().Foreground(ColorComment)
	sep := desc.Render("   ")
	escLabel := " close"
	if reattachOnCancel {
		escLabel = " back"
	}
	parts := []string{
		key.Render("Ctrl+W") + desc.Render(" next"),
		key.Render("Ctrl+A") + desc.Render(" prev"),
		key.Render("↑↓") + desc.Render(" browse"),
		key.Render("⏎") + desc.Render(" attach"),
		key.Render("esc") + desc.Render(escLabel),
	}
	return strings.Join(parts, sep)
}
