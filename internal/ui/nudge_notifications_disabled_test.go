package ui

import (
	"strings"
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/session"
	"github.com/asheshgoplani/agent-deck/internal/tmux"
)

// TestSyncNudge_RendersWhenNotificationsDisabled pins the live regression
// (2026-06-18): the ⚡ priority nudge was computed INSIDE
// syncNotificationsBackground, which returns early when notifications.enabled
// is false (notificationManager nil). A user who turned off the noisy [1]..[6]
// slot bar silently lost the nudge too, even though inject_status_line still
// lets agent-deck own the status line. The fix routes to syncPriorityNudgeOnly
// so the nudge renders on its own. Attached to an unranked conductor with a
// higher-ranked idle session ready ⇒ the status-left write carries the nudge.
func TestSyncNudge_RendersWhenNotificationsDisabled(t *testing.T) {
	const isoSocket = "" // default socket; socket isolation is off in this repro

	conductor := session.NewInstance("conductor", "/tmp/conductor")
	conductor.ID = "conductor"
	conductor.Priority = 0 // unranked
	conductor.Status = session.StatusRunning
	conductor.TmuxSocketName = isoSocket
	cts := tmux.NewSession("conductor", "/tmp/conductor")
	cts.Name = "conductor"
	cts.SocketName = isoSocket
	conductor.SetTmuxSessionForTest(cts)

	ranked := session.NewInstance("p3", "/tmp/p3")
	ranked.ID = "p3"
	ranked.Title = "MAIA-2547"
	ranked.Priority = 3
	ranked.Status = session.StatusWaiting

	insts := []*session.Instance{conductor, ranked}

	h := newTestHomeWithItems(120, 40, nil)
	// Notification slot bar OFF, but agent-deck still owns the status line.
	h.manageTmuxNotifications = true
	h.notificationsEnabled = false
	h.notificationManager = nil
	h.instancesMu.Lock()
	h.instances = insts
	h.instancesMu.Unlock()

	// Fake tmux: conductor is the attached client.
	origAttached := attachedSessionsOnSockets
	t.Cleanup(func() { attachedSessionsOnSockets = origAttached })
	attachedSessionsOnSockets = func(sockets ...string) []string { return []string{"conductor"} }

	// Capture the status-left write instead of touching a real tmux server.
	var got string
	var wrote bool
	origWriter := statusLeftWriter
	t.Cleanup(func() { statusLeftWriter = origWriter })
	statusLeftWriter = func(text string) { got = text; wrote = true }

	h.syncNotificationsBackground()

	if !wrote {
		t.Fatal("status-left was never written — nudge path did not run with notifications disabled")
	}
	if !strings.Contains(got, "ready — ^E") || !strings.Contains(got, "MAIA-2547") {
		t.Fatalf("status-left = %q, want it to carry the P3 MAIA-2547 nudge", got)
	}
}

// TestSyncNudge_ClearsWhenNothingReady ensures the nudge-only path restores the
// user's theme (empty write) when no higher-ranked session is ready, rather
// than leaving a stale nudge pinned.
func TestSyncNudge_ClearsWhenNothingReady(t *testing.T) {
	conductor := session.NewInstance("conductor", "/tmp/conductor")
	conductor.ID = "conductor"
	conductor.Status = session.StatusRunning
	cts := tmux.NewSession("conductor", "/tmp/conductor")
	cts.Name = "conductor"
	conductor.SetTmuxSessionForTest(cts)

	// A ranked session that is NOT ready (error) ⇒ no nudge.
	ranked := session.NewInstance("p3", "/tmp/p3")
	ranked.ID = "p3"
	ranked.Priority = 3
	ranked.Status = session.StatusError

	h := newTestHomeWithItems(120, 40, nil)
	h.manageTmuxNotifications = true
	h.notificationsEnabled = false
	h.notificationManager = nil
	h.instancesMu.Lock()
	h.instances = []*session.Instance{conductor, ranked}
	h.instancesMu.Unlock()
	// Pretend a nudge was previously shown so a change is detected.
	h.lastBarText = "stale-nudge"

	origAttached := attachedSessionsOnSockets
	t.Cleanup(func() { attachedSessionsOnSockets = origAttached })
	attachedSessionsOnSockets = func(sockets ...string) []string { return []string{"conductor"} }

	var got string
	origWriter := statusLeftWriter
	t.Cleanup(func() { statusLeftWriter = origWriter })
	statusLeftWriter = func(text string) { got = text }

	h.syncNotificationsBackground()

	if got != "" {
		t.Fatalf("status-left = %q, want empty (restore theme) when nothing is ready", got)
	}
}
