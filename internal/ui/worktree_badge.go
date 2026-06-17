package ui

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// worktreeKey returns the display identifier for a session's worktree —
// the thing that decides both the badge text and the color bucket.
// Priority:
//  1. WorktreeBranch (real git worktree created via agent-deck)
//  2. ProjectPath basename (pre-created worktrees like MAIA.worker-N,
//     where ProjectPath itself IS the worktree dir and WorktreeBranch
//     is empty)
//  3. "" when neither is available — render no badge.
//
// The "MAIA." prefix is stripped from both inputs so the bare worker
// identifier (e.g. "worker-7", "ro-dev") drives the key.
func worktreeKey(inst *session.Instance) string {
	if inst == nil {
		return ""
	}
	if inst.WorktreeBranch != "" {
		return strings.TrimPrefix(inst.WorktreeBranch, "MAIA.")
	}
	if inst.ProjectPath == "" {
		return ""
	}
	base := filepath.Base(strings.TrimRight(inst.ProjectPath, "/"))
	if base == "" || base == "." || base == "/" {
		return ""
	}
	return strings.TrimPrefix(base, "MAIA.")
}

// worktreeColors is the package-singleton color assigner. Hashing keys
// directly into a fixed palette birthday-paradoxed on the user's actual
// key set (worker-1/3/5/7/9 all landing in the same color bucket under
// FNV-1a; even SHA-256 had worker-1/2/3/5 within 18°). Insertion-order
// + golden-step assignment guarantees the closest two assigned hues are
// maximally far apart for any N up to the palette size.
//
// Per-theme palettes are computed lazily on first call so we pick up
// whichever theme InitTheme set. Switching themes at runtime clears
// the cache — Reset() exists for the theme-watcher path.
var worktreeColors = newWorktreeColorAssigner()

type worktreeColorAssigner struct {
	mu         sync.Mutex
	colors     map[string]lipgloss.Color
	nextIdx    int
	cachedDark bool // theme cached colors were generated for
}

func newWorktreeColorAssigner() *worktreeColorAssigner {
	return &worktreeColorAssigner{colors: map[string]lipgloss.Color{}}
}

// palette size — 24 distinct hues evenly spaced over the wheel before
// golden-step shuffling. More than the user is likely to ever need, so
// insertion-order rarely wraps.
const worktreePaletteSize = 24

// goldenStep is chosen coprime to worktreePaletteSize so the sequence
// visits every slot before repeating, and chosen close to N * φ-1 so
// successive indices land on opposite sides of the wheel. For N=24,
// 13/24 ≈ 0.54 (golden conjugate is ~0.618 — 13 and 17 are the closest
// coprime options; 13 was picked for being slightly less symmetric).
const goldenStep = 13

// Color returns the assigned color for key, allocating one on first call.
func (a *worktreeColorAssigner) Color(key string) lipgloss.Color {
	if key == "" {
		return lipgloss.Color("")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	// Reset the cache when the theme flipped — colors generated for the
	// dark palette look terrible against light and vice versa.
	dark := isDarkTheme()
	if dark != a.cachedDark && len(a.colors) > 0 {
		a.colors = map[string]lipgloss.Color{}
		a.nextIdx = 0
	}
	a.cachedDark = dark
	if c, ok := a.colors[key]; ok {
		return c
	}
	idx := (a.nextIdx * goldenStep) % worktreePaletteSize
	c := paletteColor(idx, dark)
	a.colors[key] = c
	a.nextIdx++
	return c
}

// paletteColor generates the nth color of the worktree palette as a hex
// truecolor value. HSL parameters are tuned per theme:
//
//   - Dark theme (L=0.72): bright pastels that pop against #1a1b26.
//     Saturation kept at 0.55 so colors are unmistakably colored
//     without going neon.
//   - Light theme (L=0.38): darker, more saturated so colors stand out
//     against #d5d6db. Lightness any higher washes out into the
//     background; saturation any lower reads as gray.
//
// Hue is the slot index spread evenly around the wheel. Slot 0 starts
// at 20° (orange-red) rather than 0° (pure red) so the first key
// doesn't collide visually with the error-status red.
func paletteColor(slot int, dark bool) lipgloss.Color {
	hue := math.Mod(float64(slot)*(360.0/worktreePaletteSize)+20.0, 360.0) / 360.0
	var s, l float64
	if dark {
		s, l = 0.55, 0.72
	} else {
		s, l = 0.62, 0.38
	}
	r, g, b := hslToRGB(hue, s, l)
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r, g, b))
}

func worktreeColor(key string) lipgloss.Color {
	return worktreeColors.Color(key)
}

// isDarkTheme reports whether the current UI theme is the dark one.
// Reads the theme-mu-protected currentTheme global. Defaults to dark
// if InitTheme has not yet been called.
func isDarkTheme() bool {
	themeMu.RLock()
	defer themeMu.RUnlock()
	return currentTheme != ThemeLight
}

// hslToRGB converts HSL (each 0..1) to 8-bit RGB. Standard formula —
// kept inline to avoid a color library dependency.
func hslToRGB(h, s, l float64) (uint8, uint8, uint8) {
	if s == 0 {
		v := uint8(math.Round(l * 255))
		return v, v, v
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	r := hueToRGB(p, q, h+1.0/3.0)
	g := hueToRGB(p, q, h)
	b := hueToRGB(p, q, h-1.0/3.0)
	return uint8(math.Round(r * 255)), uint8(math.Round(g * 255)), uint8(math.Round(b * 255))
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	switch {
	case t < 1.0/6.0:
		return p + (q-p)*6*t
	case t < 1.0/2.0:
		return q
	case t < 2.0/3.0:
		return p + (q-p)*(2.0/3.0-t)*6
	}
	return p
}

// worktreeBadgeGlyph is the visual anchor that precedes the worktree
// name. ⎇ (U+2387, "alternative key") is widely used in git/branch
// contexts (vim airline, starship prompts), reads as "branch" at a
// glance, and renders as a single cell in standard monospace fonts.
const worktreeBadgeGlyph = "⎇"

// renderWorktreeBadge renders the worktree tag for a session row.
func renderWorktreeBadge(inst *session.Instance, selected bool) string {
	key := worktreeKey(inst)
	if key == "" {
		return ""
	}
	display := key
	if len(display) > 20 {
		display = display[:17] + "…"
	}
	style := lipgloss.NewStyle().Foreground(worktreeColor(key))
	if selected {
		style = SessionStatusSelStyle
	}
	return " " + style.Render(worktreeBadgeGlyph+" "+display)
}
