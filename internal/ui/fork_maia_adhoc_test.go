package ui

import (
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/session"
	"github.com/stretchr/testify/assert"
)

// TestIsMaiaForkSource pins the gate that routes `f` into the MAIA ad-hoc
// worktree fork: a source is "MAIA" when it belongs to the MAIA main repo,
// whether via its worktree repo root, by sitting directly in the main repo,
// or by living in a MAIA.<name> worktree dir. Everything else forks normally.
func TestIsMaiaForkSource(t *testing.T) {
	cases := []struct {
		name  string
		setup func() *session.Instance
		want  bool
	}{
		{"nil", func() *session.Instance { return nil }, false},
		{"worktree repo root is maia main", func() *session.Instance {
			i := session.NewInstanceWithTool("w", maiaReposDir+"/MAIA.foo", "claude")
			i.WorktreeRepoRoot = maiaMainRepo
			return i
		}, true},
		{"session in maia main repo", func() *session.Instance {
			return session.NewInstanceWithTool("w", maiaMainRepo, "claude")
		}, true},
		{"maia worktree dir without recorded repo root", func() *session.Instance {
			return session.NewInstanceWithTool("w", maiaReposDir+"/MAIA.bar", "claude")
		}, true},
		{"non-maia project", func() *session.Instance {
			return session.NewInstanceWithTool("w", "/Users/c2k/repos/agent-deck", "claude")
		}, false},
		{"unrelated dir under maia repos", func() *session.Instance {
			return session.NewInstanceWithTool("w", maiaReposDir+"/notes", "claude")
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isMaiaForkSource(tc.setup()))
		})
	}
}

// TestBuildMaiaForkSpec_UsesMaiaLayoutAndFreshWorktree asserts the MAIA fork
// spec mirrors a new MAIA session's worktree setup (maia/active group,
// MAIA.<name> worktree off the main repo, title = generated name, locked) and
// crucially forks WITHOUT state materialization — a new MAIA worktree starts
// fresh off origin/dev, so the fork does too. The parent's conversation is what
// carries over, not its uncommitted files.
func TestBuildMaiaForkSpec_UsesMaiaLayoutAndFreshWorktree(t *testing.T) {
	src := session.NewInstanceWithTool("worker", maiaMainRepo, "claude")
	src.WorktreeRepoRoot = maiaMainRepo

	wtPath := maiaReposDir + "/MAIA.risen-lotus"
	spec := buildMaiaForkSpec(src, "risen-lotus", wtPath, maiaAdhocBranchPrefix+"risen-lotus")

	assert.Equal(t, "risen-lotus", spec.Title)
	assert.Equal(t, maiaWorkerGroup, spec.Group)

	assert.True(t, spec.Toggles.Worktree)
	assert.True(t, spec.Toggles.LockTitle)
	assert.False(t, spec.Toggles.WithState, "fresh MAIA worktree carries no parent WIP")
	assert.False(t, spec.Toggles.WithIgnored)

	if assert.NotNil(t, spec.Opts) {
		assert.Equal(t, wtPath, spec.Opts.WorkDir)
		assert.Equal(t, wtPath, spec.Opts.WorktreePath)
		assert.Equal(t, maiaMainRepo, spec.Opts.WorktreeRepoRoot)
		assert.Equal(t, maiaAdhocBranchPrefix+"risen-lotus", spec.Opts.WorktreeBranch)
	}
}
