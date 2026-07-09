package git

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStderr swaps os.Stderr for a pipe while fn runs and returns whatever
// was written to it. HookOutput() must resolve os.Stderr at call time (not at
// package-init time) for this to observe anything.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	_ = w.Close()
	os.Stderr = orig
	out := <-done
	_ = r.Close()
	return out
}

// newDoomedWorktree builds a repo with a destruction hook that prints to both
// its stdout and stderr, plus a linked worktree ready to be removed.
func newDoomedWorktree(t *testing.T) (repoDir, worktreePath string) {
	t.Helper()
	repoDir = t.TempDir()
	createTestRepoForSetup(t, repoDir)

	scriptDir := filepath.Join(repoDir, ".agent-deck")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho HOOK_STDOUT_MARKER\necho HOOK_STDERR_MARKER >&2\n"
	if err := os.WriteFile(filepath.Join(scriptDir, "worktree-destruction.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	worktreePath = filepath.Join(repoDir, ".worktrees", "doomed")
	if err := CreateWorktree(repoDir, worktreePath, "doomed"); err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	return repoDir, worktreePath
}

// TestRemoveWorktree_HookOutputGoesToStderrByDefault pins the CLI behavior:
// with no sink configured, hook progress + script output land on os.Stderr so
// `agent-deck session remove` still shows what a slow hook is doing.
func TestRemoveWorktree_HookOutputGoesToStderrByDefault(t *testing.T) {
	SetHookOutput(nil)
	t.Cleanup(func() { SetHookOutput(nil) })
	repoDir, worktreePath := newDoomedWorktree(t)

	var err error
	got := captureStderr(t, func() { err = RemoveWorktree(repoDir, worktreePath, true) })
	if err != nil {
		t.Fatalf("remove worktree: %v", err)
	}
	for _, want := range []string{"Running worktree destruction script", "HOOK_STDOUT_MARKER", "HOOK_STDERR_MARKER"} {
		if !strings.Contains(got, want) {
			t.Errorf("default hook output must reach os.Stderr; missing %q in:\n%s", want, got)
		}
	}
}

// TestRemoveWorktree_HookOutputNeverTouchesStderrWhenRedirected is the
// regression test for the TUI double-render: RemoveWorktree used to hardcode
// os.Stderr for the destruction hook, so deleting a worktree session from the
// Bubble Tea TUI wrote raw text straight to the terminal. That scrolled the
// alt-screen and left a corrupted frame — duplicate group headers and stray
// "Running worktree destruction script..." lines under the footer. The TUI
// redirects the sink; NOTHING may reach os.Stderr once it does.
func TestRemoveWorktree_HookOutputNeverTouchesStderrWhenRedirected(t *testing.T) {
	var sink bytes.Buffer
	SetHookOutput(&sink)
	t.Cleanup(func() { SetHookOutput(nil) })
	repoDir, worktreePath := newDoomedWorktree(t)

	var err error
	leaked := captureStderr(t, func() { err = RemoveWorktree(repoDir, worktreePath, true) })
	if err != nil {
		t.Fatalf("remove worktree: %v", err)
	}
	if strings.TrimSpace(leaked) != "" {
		t.Errorf("destruction hook leaked to os.Stderr and would corrupt the TUI frame:\n%s", leaked)
	}
	for _, want := range []string{"Running worktree destruction script", "HOOK_STDOUT_MARKER", "HOOK_STDERR_MARKER"} {
		if !strings.Contains(sink.String(), want) {
			t.Errorf("redirected sink missing %q in:\n%s", want, sink.String())
		}
	}
}
