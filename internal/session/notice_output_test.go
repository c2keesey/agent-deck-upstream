package session

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStderrForNotice swaps os.Stderr for a pipe while fn runs. NoticeOutput()
// must resolve os.Stderr at call time (not package-init time) to be observable.
func captureStderrForNotice(t *testing.T, fn func()) string {
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

// TestMacOSScratchWarning_GoesToStderrByDefault pins the CLI behavior: with no
// sink configured the notice banner still reaches os.Stderr.
func TestMacOSScratchWarning_GoesToStderrByDefault(t *testing.T) {
	SetNoticeOutput(nil)
	t.Cleanup(func() { SetNoticeOutput(nil) })

	got := captureStderrForNotice(t, func() { emitMacOSScratchWarning("/tmp/profile") })
	if !strings.Contains(got, "NOTICE: per-session plugin scratch on macOS") {
		t.Errorf("default notice must reach os.Stderr, got:\n%s", got)
	}
}

// TestMacOSScratchWarning_NeverTouchesStderrWhenRedirected is the regression
// test for TUI frame corruption: this multi-line box used to be written straight
// to os.Stderr during session creation. Inside the Bubble Tea alt screen that
// scrolls the frame and leaves a doubled/garbled render (same class of bug as
// the worktree destruction hook). Once the TUI redirects the sink, nothing may
// reach os.Stderr.
func TestMacOSScratchWarning_NeverTouchesStderrWhenRedirected(t *testing.T) {
	var sink bytes.Buffer
	SetNoticeOutput(&sink)
	t.Cleanup(func() { SetNoticeOutput(nil) })

	leaked := captureStderrForNotice(t, func() { emitMacOSScratchWarning("/tmp/profile") })
	if strings.TrimSpace(leaked) != "" {
		t.Errorf("notice leaked to os.Stderr and would corrupt the TUI frame:\n%s", leaked)
	}
	if !strings.Contains(sink.String(), "NOTICE: per-session plugin scratch on macOS") {
		t.Errorf("redirected sink missing the notice, got:\n%s", sink.String())
	}
}
