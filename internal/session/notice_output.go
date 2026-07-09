package session

import (
	"io"
	"os"
	"sync"
)

// noticeOutput is the sink for user-facing notice banners emitted during
// session setup. nil means "os.Stderr, resolved at call time" — the right
// default for CLI commands.
//
// The TUI owns the terminal: raw writes scroll the Bubble Tea alt screen and
// leave a corrupted, doubled frame. It MUST call SetNoticeOutput with a
// non-terminal sink at startup. Mirrors git.SetHookOutput.
var (
	noticeOutputMu sync.RWMutex
	noticeOutput   io.Writer
)

// SetNoticeOutput redirects session notice banners. Pass nil to restore os.Stderr.
func SetNoticeOutput(w io.Writer) {
	noticeOutputMu.Lock()
	noticeOutput = w
	noticeOutputMu.Unlock()
}

// NoticeOutput returns the current notice sink, defaulting to os.Stderr. The
// default is read at call time so callers (and tests) can swap os.Stderr.
func NoticeOutput() io.Writer {
	noticeOutputMu.RLock()
	w := noticeOutput
	noticeOutputMu.RUnlock()
	if w == nil {
		return os.Stderr
	}
	return w
}
