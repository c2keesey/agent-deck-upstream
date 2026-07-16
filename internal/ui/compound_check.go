package ui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// Missing-/compound warning (local fork). Before a session is removed (d /
// MAIA teardown) or closed (ctrl+x), its Claude transcript is scanned for a
// user message that invoked the /compound skill (compound-engineering's
// "document what we just solved" step). If the session did real work but
// /compound never ran, the confirmation modal warns — the learnings are about
// to be thrown away with the session.

// forgotCompoundSkill reports whether inst's Claude transcript shows user
// activity but no user message ever invoking /compound. It stays quiet (false)
// whenever it can't verify: non-Claude tools, no recorded session ID, no
// transcript on disk, or a transcript with no user messages (nothing worth
// documenting).
func forgotCompoundSkill(inst *session.Instance) bool {
	if inst == nil || !session.IsClaudeCompatible(inst.Tool) || inst.ClaudeSessionID == "" {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	// Claude writes the transcript under the slug of its cwd: the worktree dir
	// for worktree sessions (EffectiveWorkingDir), the project dir otherwise.
	tried := map[string]bool{}
	for _, dir := range []string{inst.EffectiveWorkingDir(), inst.ProjectPath} {
		if dir == "" || tried[dir] {
			continue
		}
		tried[dir] = true
		path := filepath.Join(home, ".claude", "projects", claudeProjectSlug(dir), inst.ClaudeSessionID+".jsonl")
		if hasUser, hasCompound, err := scanTranscriptForCompound(path); err == nil {
			return hasUser && !hasCompound
		}
	}
	return false
}

// scanTranscriptForCompound streams the transcript once and reports whether it
// holds any user-typed message and whether any of them mentions "/compound"
// (covers a raw "/compound" prompt, the <command-name>/compound</command-name>
// slash-command record, and namespaced variants like
// "/compound-engineering:workflows:compound").
func scanTranscriptForCompound(path string) (hasUser, hasCompound bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return false, false, err
	}
	defer f.Close()

	userMarker := []byte(`"type":"user"`)
	compoundMarker := []byte("/compound")
	sc := bufio.NewScanner(f)
	// Transcript lines can be huge (pasted files, tool results); default 64KB
	// would error out mid-file.
	sc.Buffer(make([]byte, 0, 256*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.Contains(line, userMarker) {
			continue
		}
		hasUser = true
		if !bytes.Contains(line, compoundMarker) {
			continue
		}
		// Cheap prefilter hit — confirm the mention sits in a genuine user
		// entry's message payload, not e.g. an assistant line that happens to
		// embed the marker bytes elsewhere.
		var entry struct {
			Type    string          `json:"type"`
			Message json.RawMessage `json:"message"`
		}
		if json.Unmarshal(line, &entry) == nil && entry.Type == "user" &&
			bytes.Contains(entry.Message, compoundMarker) {
			return true, true, nil
		}
	}
	if err := sc.Err(); err != nil {
		return hasUser, hasCompound, err
	}
	return hasUser, false, nil
}

// claudeProjectSlug converts a working directory to Claude Code's transcript
// directory slug: / and . become - (mirrors costs.slugifyProjectPath, kept
// fork-local so upstream merges stay clean).
func claudeProjectSlug(dir string) string {
	dir = strings.TrimRight(dir, "/")
	dir = strings.ReplaceAll(dir, "/", "-")
	return strings.ReplaceAll(dir, ".", "-")
}
