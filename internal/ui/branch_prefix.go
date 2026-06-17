package ui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// branchCache memoizes git HEAD lookups per repo dir. Branches change
// rarely (every few minutes at most during active dev), so a 30s TTL
// avoids re-reading .git/HEAD on every status snapshot tick while still
// picking up branch switches within a render cycle.
var (
	branchCacheMu sync.RWMutex
	branchCache   = map[string]branchCacheEntry{}
)

type branchCacheEntry struct {
	branch string
	at     time.Time
}

const branchCacheTTL = 30 * time.Second

// cachedGitBranch returns the current branch for the worktree rooted at
// repoDir, falling back to "" when the dir isn't a git checkout or HEAD
// is detached/unreadable. Cached behind a mutex with a short TTL — fast
// enough for the render-snapshot path (called once per session per
// background status tick).
func cachedGitBranch(repoDir string) string {
	if repoDir == "" {
		return ""
	}
	branchCacheMu.RLock()
	e, ok := branchCache[repoDir]
	branchCacheMu.RUnlock()
	if ok && time.Since(e.at) < branchCacheTTL {
		return e.branch
	}
	b := readGitBranch(repoDir)
	branchCacheMu.Lock()
	branchCache[repoDir] = branchCacheEntry{branch: b, at: time.Now()}
	branchCacheMu.Unlock()
	return b
}

// readGitBranch resolves the current HEAD ref by reading .git/HEAD
// directly. Handles three cases:
//
//   - .git is a directory → standard repo, read .git/HEAD.
//   - .git is a file → linked worktree, .git contains "gitdir: <path>"
//     pointing at the per-worktree HEAD inside the main repo.
//   - HEAD points at a ref (`ref: refs/heads/branch`) → return the
//     short branch name. Detached HEAD (a raw SHA) → return short SHA.
//
// Reading files instead of shelling out to `git branch --show-current`
// is ~100x faster and avoids forking on every status tick. The cost is
// that this won't handle exotic git configurations (worktree of a
// worktree, packed refs in the HEAD itself) — but for the standard
// `git worktree add` layout the user has, that's fine.
func readGitBranch(repoDir string) string {
	gitPath := filepath.Join(repoDir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return ""
	}
	var headPath string
	if info.IsDir() {
		headPath = filepath.Join(gitPath, "HEAD")
	} else {
		data, err := os.ReadFile(gitPath)
		if err != nil {
			return ""
		}
		line := strings.TrimSpace(string(data))
		if !strings.HasPrefix(line, "gitdir:") {
			return ""
		}
		gitdir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
		// gitdir paths are usually absolute, but accept relative-to-worktree too.
		if !filepath.IsAbs(gitdir) {
			gitdir = filepath.Join(repoDir, gitdir)
		}
		headPath = filepath.Join(gitdir, "HEAD")
	}
	data, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(data))
	if strings.HasPrefix(line, "ref:") {
		ref := strings.TrimSpace(strings.TrimPrefix(line, "ref:"))
		return strings.TrimPrefix(ref, "refs/heads/")
	}
	// Detached HEAD — return short SHA so the user has SOME identifier.
	if len(line) >= 7 {
		return line[:7]
	}
	return ""
}

// branchPrefixesToStrip removes the repetitive convention noise from
// branch display. Ordered longest-first so "ck/MAIA-" matches before the
// bare "ck/" fallback. Personal-fork list:
//
//   - "ck/MAIA-" / "zh/MAIA-": user's & teammate's MAIA-ticket branches
//   - "ck/" / "zh/": personal prefix for ad-hoc work
//   - "chore/" / "feat/" / "fix/" / "docs/" / "refactor/": conventional-
//     commits style branch prefixes used in shared repos
//
// Other devs forking this would update this list to their own convention.
var branchPrefixesToStrip = []string{
	"ck/MAIA-", "zh/MAIA-",
	"ck/", "zh/",
	"chore/", "feat/", "fix/", "docs/", "refactor/", "perf/",
}

// cleanBranchDisplay strips the user's branch-naming convention prefixes
// so the meaningful suffix is what reads. "ck/MAIA-1863-register-layer"
// becomes "1863-register-layer" — the ticket number is the actual ID
// the user cares about scanning for.
//
// After the convention prefixes, a bare leading "MAIA-<number>" Linear
// ticket prefix is also stripped ("MAIA-1963-register-layer" ->
// "1963-register-layer"), so branches that don't carry a "ck/" author
// prefix still scan by ticket number.
func cleanBranchDisplay(branch string) string {
	for _, p := range branchPrefixesToStrip {
		if strings.HasPrefix(branch, p) {
			branch = strings.TrimPrefix(branch, p)
			break
		}
	}
	return stripMaiaTicketPrefix(branch)
}

// stripMaiaTicketPrefix removes a leading "MAIA-" when it's immediately
// followed by a digit (a Linear ticket number). "MAIA-1963-foo" ->
// "1963-foo"; "MAIA-overhaul" (non-numeric) is left untouched.
func stripMaiaTicketPrefix(branch string) string {
	const p = "MAIA-"
	if strings.HasPrefix(branch, p) && len(branch) > len(p) {
		if c := branch[len(p)]; c >= '0' && c <= '9' {
			return branch[len(p):]
		}
	}
	return branch
}

// padToCells right-pads a pre-styled string with plain (unstyled) spaces
// so its visible width reaches colCells. Plain spaces avoid carrying the
// field's foreground/background into the gap between columns. No-op when
// the content already meets or exceeds the target.
func padToCells(s string, visible, colCells int) string {
	if pad := colCells - visible; pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}
