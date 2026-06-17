package session

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Start-path twin of issue #1147 (see issue1147_multi_session_cwd_test.go).
//
// ensureClaudeSessionIDFromDiskForRestart honors an explicit `--session-id` in
// i.Command, but the cold-boot Start-path prelude
// (ensureClaudeSessionIDFromDisk) did NOT — so two sessions recovering in the
// same cwd, each with its own --session-id, both disk-discovered the newest
// sibling JSONL by mtime and converged on one ClaudeSessionID. That shared id
// then leaks across the title-sync path (ReconcileTitleFromClaude): a name set
// on one session reappears on its siblings. The fix mirrors the restart variant.

// TestStartPath_ExplicitID_TwoSessions_SameCWD: two sessions sharing a cwd,
// each launched with its own explicit --session-id, must each retain its own
// UUID after the Start-path disk-discovery prelude — never the newest sibling.
func TestStartPath_ExplicitID_TwoSessions_SameCWD(t *testing.T) {
	home := isolatedHomeDir(t)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	ClearUserConfigCache()

	const (
		uuidA       = "a1a1a1a1-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		uuidB       = "b1b1b1b1-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		uuidHostile = "ffffffff-eeee-dddd-cccc-bbbbbbbbbbbb"
	)

	cmdA := "claude --session-id " + uuidA + " --dangerously-skip-permissions"
	cmdB := "claude --session-id " + uuidB + " --dangerously-skip-permissions"

	instA := newClaudeInstanceForExplicitID(t, home, cmdA)
	instB := newClaudeInstanceForExplicitID(t, home, cmdB)
	// Both sessions share a cwd — mirror the real bug scenario.
	instB.ProjectPath = instA.ProjectPath

	// Cold-boot recovery: the in-memory id was lost, prior conversations exist
	// on disk, and a hostile sibling JSONL is the newest by mtime.
	now := time.Now()
	stageJSONL(t, home, instA.ProjectPath, uuidA, now.Add(-30*time.Second))
	stageJSONL(t, home, instA.ProjectPath, uuidB, now.Add(-15*time.Second))
	stageJSONL(t, home, instA.ProjectPath, uuidHostile, now)

	instA.ensureClaudeSessionIDFromDisk()
	instB.ensureClaudeSessionIDFromDisk()

	require.Equal(t, uuidA, instA.ClaudeSessionID,
		"Start path must adopt the explicit --session-id %s from Command, not the newest sibling JSONL %s. Got %q.",
		uuidA, uuidHostile, instA.ClaudeSessionID)
	require.Equal(t, uuidB, instB.ClaudeSessionID,
		"Start path must adopt the explicit --session-id %s from Command, not the newest sibling JSONL %s. Got %q.",
		uuidB, uuidHostile, instB.ClaudeSessionID)
	require.NotEqual(t, instA.ClaudeSessionID, instB.ClaudeSessionID,
		"two sessions in one cwd with distinct explicit --session-id values must not converge — that convergence is what leaks a renamed title across rows.")
}

// TestStartPath_NoExplicitID_DiskDiscoveryStillWorks pins #956/#608 parity on
// the Start path: a custom-wrapper session with NO --session-id in its Command
// must still auto-bind from the newest JSONL so history survives a cold boot.
func TestStartPath_NoExplicitID_DiskDiscoveryStillWorks(t *testing.T) {
	home := isolatedHomeDir(t)
	// macOS: t.TempDir() lives under /var/folders, a symlink to
	// /private/var/folders. discoverLatestClaudeJSONL EvalSymlinks the project
	// path before encoding it, so the JSONL must be staged under the resolved
	// path or the encoded project dir won't match. (The sibling disk-discovery
	// tests omit this and are therefore macOS-local-only failures.)
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	ClearUserConfigCache()

	// Custom wrapper, no --session-id baked in. The fixture stamps
	// ClaudeDetectedAt ~1h in the past, satisfying the #608 gate.
	inst := newClaudeInstanceForExplicitID(t, home, "/usr/local/bin/my-wrapper.sh")

	const jsonlUUID = "95600000-9560-9560-9560-956000000956"
	stageJSONL(t, home, inst.ProjectPath, jsonlUUID, time.Now())

	inst.ensureClaudeSessionIDFromDisk()

	require.Equal(t, jsonlUUID, inst.ClaudeSessionID,
		"Start path must NOT regress #956: a custom-wrapper session with no explicit --session-id must still auto-bind to the newest JSONL on cold boot. Got %q.",
		inst.ClaudeSessionID)
}
