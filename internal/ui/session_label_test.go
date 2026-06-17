package ui

import (
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// TestCleanBranchDisplay covers convention-prefix stripping plus the bare
// "MAIA-<number>" Linear ticket prefix.
func TestCleanBranchDisplay(t *testing.T) {
	cases := map[string]string{
		"ck/MAIA-1963-register-layer": "1963-register-layer", // author + ticket prefix
		"zh/MAIA-1850-foo":            "1850-foo",
		"MAIA-1963-register-layer":    "1963-register-layer", // bare ticket prefix
		"feat/MAIA-42-thing":          "42-thing",            // conventional + ticket
		"MAIA-overhaul":               "MAIA-overhaul",       // non-numeric: untouched
		"ck/quick-fix":                "quick-fix",
		"main":                        "main", // no prefix
	}
	for in, want := range cases {
		if got := cleanBranchDisplay(in); got != want {
			t.Errorf("cleanBranchDisplay(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestPadToCells confirms plain-space padding to the target and the no-op
// path when content already meets or exceeds it.
func TestPadToCells(t *testing.T) {
	if got := padToCells("ab", 2, 5); got != "ab   " {
		t.Fatalf("padToCells(ab,2,5) = %q, want %q", got, "ab   ")
	}
	if got := padToCells("abcde", 5, 3); got != "abcde" {
		t.Fatalf("padToCells should be a no-op when wider than target, got %q", got)
	}
}

// TestPrimaryLabelFor_Precedence locks the dynamic-precedence identity
// engine the user approved: custom name > distinguishing branch >
// Claude broadcast > folder/worktree > auto name. (Broadcast now outranks
// the folder so an active worker shows its live activity, not "worker-N".)
func TestPrimaryLabelFor_Precedence(t *testing.T) {
	cases := []struct {
		name        string
		inst        *session.Instance
		st          sessionRenderState
		branchCount map[string]int
		wantText    string
		wantKind    primaryKind
	}{
		{
			name:     "custom name beats everything",
			inst:     &session.Instance{Title: "CONDUCTOR", ProjectPath: "/x/MAIA.worker-3"},
			st:       sessionRenderState{branch: "ck/MAIA-1963-foo"},
			wantText: "CONDUCTOR",
			wantKind: primaryName,
		},
		{
			name:     "MAIA-ticket title strips the MAIA- prefix",
			inst:     &session.Instance{Title: "MAIA-2213 migrate user-fields", ProjectPath: "/x/MAIA.worker-6"},
			st:       sessionRenderState{branch: "ck/MAIA-2213-migrate"},
			wantText: "2213 migrate user-fields",
			wantKind: primaryName,
		},
		{
			name:        "worker: unique branch wins over auto name, convention stripped",
			inst:        &session.Instance{Title: "prairie-condor", ProjectPath: "/x/MAIA.worker-3"},
			st:          sessionRenderState{branch: "ck/MAIA-1963-register-layer"},
			branchCount: map[string]int{"ck/MAIA-1963-register-layer": 1},
			wantText:    "1963-register-layer",
			wantKind:    primaryBranch,
		},
		{
			name:     "ro-dev: custom name still wins",
			inst:     &session.Instance{Title: "investigate-auth", ProjectPath: "/x/MAIA.ro-dev"},
			st:       sessionRenderState{branch: "ck/MAIA-1963-register-layer"},
			wantText: "investigate-auth",
			wantKind: primaryName,
		},
		{
			name:        "ro-dev: branch is NOT used even when unique -> broadcast",
			inst:        &session.Instance{Title: "scarlet-meadow", ProjectPath: "/x/MAIA.ro-dev"},
			st:          sessionRenderState{branch: "ck/MAIA-1963-register-layer", paneTitle: "Reading logs"},
			branchCount: map[string]int{"ck/MAIA-1963-register-layer": 1},
			wantText:    "Reading logs",
			wantKind:    primaryBroadcast,
		},
		{
			name:        "ro-dev: auto name when no broadcast (branch/folder skipped)",
			inst:        &session.Instance{Title: "scarlet-meadow", ProjectPath: "/x/MAIA.ro-dev-2"},
			st:          sessionRenderState{branch: "ck/MAIA-1963-register-layer"},
			branchCount: map[string]int{"ck/MAIA-1963-register-layer": 1},
			wantText:    "scarlet-meadow",
			wantKind:    primaryAuto,
		},
		{
			name:     "base branch is non-distinguishing -> folder",
			inst:     &session.Instance{Title: "light-thorn", ProjectPath: "/x/MAIA.worker-8"},
			st:       sessionRenderState{branch: "main"},
			wantText: "worker-8",
			wantKind: primaryFolder,
		},
		{
			// The MAIA-worker fix: an active worker shows what it's doing (live
			// pane title) instead of its interchangeable "worker-N" folder. The
			// folder still appears as the color chip; the broadcast wins the label.
			name:     "worker: active pane title beats the worktree folder",
			inst:     &session.Instance{Title: "light-thorn", ProjectPath: "/x/MAIA.worker-3"},
			st:       sessionRenderState{branch: "main", paneTitle: "Exploring messaging"},
			wantText: "Exploring messaging",
			wantKind: primaryBroadcast,
		},
		{
			name:     "worker: idle (no pane title) falls back to the worktree folder",
			inst:     &session.Instance{Title: "light-thorn", ProjectPath: "/x/MAIA.worker-3"},
			st:       sessionRenderState{branch: "main"},
			wantText: "worker-3",
			wantKind: primaryFolder,
		},
		{
			// A distinguishing branch still outranks the broadcast.
			name:        "worker: unique branch still beats pane title",
			inst:        &session.Instance{Title: "light-thorn", ProjectPath: "/x/MAIA.worker-3"},
			st:          sessionRenderState{branch: "ck/MAIA-1963-register-layer", paneTitle: "Exploring messaging"},
			branchCount: map[string]int{"ck/MAIA-1963-register-layer": 1},
			wantText:    "1963-register-layer",
			wantKind:    primaryBranch,
		},
		{
			name:     "no branch, no folder -> broadcast",
			inst:     &session.Instance{Title: "shadow-yarrow"},
			st:       sessionRenderState{paneTitle: "Editing config.go"},
			wantText: "Editing config.go",
			wantKind: primaryBroadcast,
		},
		{
			name:     "nothing else -> auto name as last resort",
			inst:     &session.Instance{Title: "shadow-yarrow"},
			st:       sessionRenderState{},
			wantText: "shadow-yarrow",
			wantKind: primaryAuto,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := primaryLabelFor(tc.inst, tc.st, tc.branchCount)
			if got.text != tc.wantText || got.kind != tc.wantKind {
				t.Fatalf("got {%q, %d}, want {%q, %d}", got.text, got.kind, tc.wantText, tc.wantKind)
			}
		})
	}
}

// TestIsAutoGeneratedName guards the meaningful-vs-disposable title split
// that decides whether a name outranks branch/worktree.
func TestIsAutoGeneratedName(t *testing.T) {
	auto := []string{"prairie-condor", "light-thorn", "scarlet-meadow", "shadow-yarrow", "amber-fox-1717000000"}
	for _, n := range auto {
		if !session.IsAutoGeneratedName(n) {
			t.Errorf("%q should be detected as auto-generated", n)
		}
	}
	custom := []string{"CONDUCTOR", "dev", "o", "agent-deck", "fix-the-bug", "amber-notaword"}
	for _, n := range custom {
		if session.IsAutoGeneratedName(n) {
			t.Errorf("%q should NOT be detected as auto-generated", n)
		}
	}
}
