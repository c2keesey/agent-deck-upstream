package ui

import (
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/session"
	"github.com/asheshgoplani/agent-deck/internal/tmux"
)

// TestGetAttachedSessionID_IsolatedSocket pins the priority-nudge root cause
// (v1.9.71): getAttachedSessionID only queried the DEFAULT tmux socket, so a
// session attached on an isolated agent-deck socket (TmuxSocketName != "") —
// which is how the conductor and most fork sessions run — resolved to "".
// attentionNudgeText then bailed at its attachedID == "" guard, so the ⚡ nudge
// never rendered even though a higher-ranked session was idle and the Ctrl+E
// jump (which doesn't depend on attach detection) worked.
//
// The fake stands in for tmux: it reports the conductor attached ONLY when its
// isolated socket is among the probed sockets. A regression to a
// default-socket-only query (no isolated socket passed) makes this fail.
func TestGetAttachedSessionID_IsolatedSocket(t *testing.T) {
	mkOnSocket := func(id, socket string) *session.Instance {
		inst := session.NewInstance(id, "/tmp/"+id)
		inst.ID = id
		inst.TmuxSocketName = socket
		ts := tmux.NewSession(id, "/tmp/"+id)
		ts.Name = id
		ts.SocketName = socket
		inst.SetTmuxSessionForTest(ts)
		return inst
	}

	const isoSocket = "adtest-conductor-iso"
	conductor := mkOnSocket("conductor", isoSocket) // attached, isolated socket
	worker := mkOnSocket("worker", "")              // default socket, not attached

	h := newTestHomeWithItems(120, 40, nil)
	h.instancesMu.Lock()
	h.instances = []*session.Instance{conductor, worker}
	h.instancesMu.Unlock()

	// Fake tmux: conductor is attached on its isolated socket. Returns its name
	// only if that socket was probed — exactly what the default-socket-only bug
	// failed to do.
	orig := attachedSessionsOnSockets
	t.Cleanup(func() { attachedSessionsOnSockets = orig })
	attachedSessionsOnSockets = func(sockets ...string) []string {
		for _, s := range sockets {
			if s == isoSocket {
				return []string{"conductor"}
			}
		}
		return nil
	}

	if got := h.getAttachedSessionID(); got != "conductor" {
		t.Fatalf("getAttachedSessionID() = %q, want \"conductor\" (isolated socket must be probed)", got)
	}
}

// TestGetAttachedSessionID_NudgeRendersFromIsolatedAttach is the end-to-end
// guard: attached to an UNRANKED session on an isolated socket while a
// higher-ranked session is idle ⇒ the nudge string is non-empty. This is the
// exact live conductor scenario the bug suppressed.
func TestGetAttachedSessionID_NudgeRendersFromIsolatedAttach(t *testing.T) {
	const isoSocket = "adtest-conductor-iso"
	conductor := session.NewInstance("conductor", "/tmp/conductor")
	conductor.ID = "conductor"
	conductor.Priority = 0 // unranked
	conductor.Status = session.StatusRunning
	conductor.TmuxSocketName = isoSocket
	cts := tmux.NewSession("conductor", "/tmp/conductor")
	cts.Name = "conductor"
	cts.SocketName = isoSocket
	conductor.SetTmuxSessionForTest(cts)

	ranked := session.NewInstance("p1", "/tmp/p1")
	ranked.ID = "p1"
	ranked.Priority = 1 // outranks the unranked conductor
	ranked.Status = session.StatusIdle

	insts := []*session.Instance{conductor, ranked}
	h := newTestHomeWithItems(120, 40, nil)
	h.instancesMu.Lock()
	h.instances = insts
	h.instancesMu.Unlock()

	orig := attachedSessionsOnSockets
	t.Cleanup(func() { attachedSessionsOnSockets = orig })
	attachedSessionsOnSockets = func(sockets ...string) []string {
		for _, s := range sockets {
			if s == isoSocket {
				return []string{"conductor"}
			}
		}
		return nil
	}

	attachedID := h.getAttachedSessionID()
	if attachedID != "conductor" {
		t.Fatalf("attached id = %q, want \"conductor\"", attachedID)
	}
	if nudge := h.attentionNudgeText(attachedID, insts); nudge == "" {
		t.Fatal("nudge empty while attached to unranked conductor with a higher-ranked idle session ready")
	}
}
