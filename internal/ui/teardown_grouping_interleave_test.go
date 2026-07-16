package ui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// These tests simulate realistic message interleavings of the teardown flow
// (d on a MAIA worktree session → confirm → shell cleanup + inline delete on
// the cmd goroutine → sessionDeletedMsg) with storage reloads (storageChangedMsg →
// loadSessionsMsg with restoreState) and tick-driven rebuilds, and assert
// grouping integrity invariants after every step:
//
//  1. every group in the tree only holds sessions whose GroupPath matches it
//  2. every flatItems session row renders under (nearest preceding header of)
//     its own group, and row.Path == Session.GroupPath
//  3. no session appears twice; no deleted session appears anywhere
//  4. h.instances and h.instanceByID agree
//
// They pin the fixes for the post-teardown grouping corruption:
//   - loadSessionsMsg staleness guard (out-of-order reload snapshots)
//   - sessionDeletedMsg requeue instead of drop during reloads
//
// Fixture mirrors the production shape: nested groups "maia" + "maia/active"
// plus another root group, teardown targets in the child group.

// ---------------------------------------------------------------------------
// fixture + invariant helpers
// ---------------------------------------------------------------------------

func buildTeardownFixture(t *testing.T) *Home {
	t.Helper()
	setXDGTestHome(t)

	h := NewHome()
	h.width, h.height = 120, 40
	h.initialLoading = false
	// The real storage watcher would make listenForReloads block forever when
	// drainCmd executes the storageChangedMsg batch; tests drive reloads by hand.
	h.storageWatcher = nil

	mk := func(id, title, group, path string, status session.Status, order int) *session.Instance {
		return &session.Instance{
			ID: id, Title: title, GroupPath: group,
			ProjectPath: path, Status: status, Order: order,
			Tool: "claude",
		}
	}
	// The teardown targets live in (nonexistent) disposable MAIA worktree
	// paths so `d` routes them to the teardown confirmation; the dirs are
	// never touched because the shell cmd is not executed in these tests.
	insts := []*session.Instance{
		mk("act1", "Act One", "maia/active", maiaReposDir+"/MAIA.itest-act1", session.StatusRunning, 0),
		mk("act2", "Act Two", "maia/active", maiaReposDir+"/MAIA.itest-act2", session.StatusRunning, 1),
		mk("root1", "Root One", "maia", t.TempDir(), session.StatusIdle, 0),
		mk("oth1", "Other One", "other", t.TempDir(), session.StatusIdle, 0),
	}
	groups := []*session.GroupData{
		{Name: "maia", Path: "maia", Expanded: true, Order: 0},
		{Name: "active", Path: "maia/active", Expanded: true, Order: 0},
		{Name: "other", Path: "other", Expanded: true, Order: 1},
	}

	h.instances = insts
	h.instanceByID = make(map[string]*session.Instance, len(insts))
	for _, in := range insts {
		h.instanceByID[in.ID] = in
	}
	h.groupTree = session.NewGroupTreeWithGroups(insts, groups)
	h.rebuildFlatItems()

	// Persist so simulated reloads read a real DB snapshot (fresh clone
	// pointers, exactly like a production storage-watcher reload).
	if h.storage == nil {
		t.Fatal("test Home has no storage")
	}
	if err := h.storage.SaveWithGroups(insts, h.groupTree); err != nil {
		t.Fatalf("seed SaveWithGroups: %v", err)
	}
	return h
}

// checkGroupingIntegrity asserts the tree/flatItems/instances invariants.
// deletedIDs are sessions that must be absent everywhere.
func checkGroupingIntegrity(t *testing.T, h *Home, label string, deletedIDs ...string) {
	t.Helper()
	deleted := map[string]bool{}
	for _, id := range deletedIDs {
		deleted[id] = true
	}
	norm := func(gp string) string {
		if gp == "" {
			return session.DefaultGroupPath
		}
		return gp
	}

	// 1. tree consistency
	seenInTree := map[string]string{} // sessionID -> group path
	for path, g := range h.groupTree.Groups {
		if g.Path != path {
			t.Errorf("%s: tree map key %q != group.Path %q", label, path, g.Path)
		}
		for _, s := range g.Sessions {
			if deleted[s.ID] {
				t.Errorf("%s: deleted session %q still present in tree group %q", label, s.ID, g.Path)
			}
			if norm(s.GroupPath) != g.Path {
				t.Errorf("%s: session %q has GroupPath %q but is held by tree group %q",
					label, s.ID, s.GroupPath, g.Path)
			}
			if prev, dup := seenInTree[s.ID]; dup {
				t.Errorf("%s: session %q appears in two tree groups: %q and %q", label, s.ID, prev, g.Path)
			}
			seenInTree[s.ID] = g.Path
		}
	}

	// 2. instances / instanceByID consistency
	for _, s := range h.instances {
		if deleted[s.ID] {
			t.Errorf("%s: deleted session %q still present in h.instances", label, s.ID)
		}
		if got := h.instanceByID[s.ID]; got != s {
			t.Errorf("%s: h.instanceByID[%q] pointer mismatch with h.instances entry", label, s.ID)
		}
		if _, ok := seenInTree[s.ID]; !ok {
			t.Errorf("%s: session %q in h.instances but missing from group tree", label, s.ID)
		}
	}
	for id := range h.instanceByID {
		if deleted[id] {
			t.Errorf("%s: deleted session %q still present in h.instanceByID", label, id)
		}
	}

	// 3. flatItems rendering invariant: a session row must sit under the header
	// of its own group (nearest preceding group header). Windows and dividers
	// are skipped; duplicated headers from view-mode partitioning are fine.
	lastHeader := ""
	rowCount := map[string]int{}
	for i, it := range h.flatItems {
		switch it.Type {
		case session.ItemTypeGroup:
			lastHeader = it.Path
		case session.ItemTypeSession:
			if it.Session == nil {
				t.Errorf("%s: flatItems[%d] session row with nil Session", label, i)
				continue
			}
			rowCount[it.Session.ID]++
			if deleted[it.Session.ID] {
				t.Errorf("%s: deleted session %q still rendered in flatItems", label, it.Session.ID)
			}
			gp := norm(it.Session.GroupPath)
			if it.Path != gp {
				t.Errorf("%s: flatItems[%d] session %q row Path %q != its GroupPath %q",
					label, i, it.Session.ID, it.Path, gp)
			}
			if lastHeader != gp {
				t.Errorf("%s: flatItems[%d] session %q (group %q) renders under header %q — WRONG GROUP",
					label, i, it.Session.ID, gp, lastHeader)
			}
		}
	}
	for id, n := range rowCount {
		if n > 1 {
			t.Errorf("%s: session %q rendered %d times in flatItems", label, id, n)
		}
	}
	// Completeness: every live, non-archived instance must render exactly once
	// (all fixture groups stay expanded in these tests).
	for _, s := range h.instances {
		if s.IsArchived() {
			continue
		}
		if rowCount[s.ID] != 1 {
			t.Errorf("%s: session %q in h.instances rendered %d times (want 1)", label, s.ID, rowCount[s.ID])
		}
	}
}

// pressKey routes a rune key through the main key handler.
func pressKey(t *testing.T, h *Home, r rune) tea.Cmd {
	t.Helper()
	model, cmd := h.handleMainKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	if model.(*Home) != h {
		t.Fatalf("handleMainKey returned a different model")
	}
	return cmd
}

// startTeardown drives the user-visible teardown entry: d on the cursor row
// (must be a MAIA worktree session → teardown confirmation) and confirm. The
// returned shell cmd is deliberately NOT executed — individual tests trigger
// its delete step by hand via runTeardownResult (finishTeardown).
func startTeardown(t *testing.T, h *Home) {
	t.Helper()
	if cmd := pressKey(t, h, 'd'); cmd != nil {
		t.Fatalf("d should only open the confirm dialog, got cmd")
	}
	if !h.confirmDialog.IsVisible() || h.confirmDialog.GetConfirmType() != ConfirmTeardownSession {
		t.Fatalf("expected teardown confirmation, got visible=%v type=%v",
			h.confirmDialog.IsVisible(), h.confirmDialog.GetConfirmType())
	}
	if cmd := h.confirmAction(); cmd == nil {
		t.Fatalf("confirming teardown produced no cmd")
	}
}

func cursorTo(t *testing.T, h *Home, sessionID string) {
	t.Helper()
	for i, it := range h.flatItems {
		if it.Type == session.ItemTypeSession && it.Session != nil && it.Session.ID == sessionID {
			h.cursor = i
			return
		}
	}
	t.Fatalf("session %q not found in flatItems", sessionID)
}

// beginReload mimics storageChangedMsg exactly (sets isReloading, bumps
// reloadVersion, captures restoreState) and returns the loadSessionsMsg the
// async DB read would deliver — WITHOUT applying it, so tests control when
// the reload "lands".
func beginReload(t *testing.T, h *Home) loadSessionsMsg {
	t.Helper()
	model, cmd := h.Update(storageChangedMsg{})
	if model.(*Home) != h {
		t.Fatalf("Update returned different model")
	}
	if cmd == nil {
		t.Fatalf("storageChangedMsg returned nil cmd")
	}
	// The handler returns tea.Batch(loadCmd, listenForReloads). Run the
	// sub-commands and pick out the loadSessionsMsg (the real DB read).
	msgs := drainCmd(cmd)
	for _, m := range msgs {
		if lm, ok := m.(loadSessionsMsg); ok {
			return lm
		}
	}
	t.Fatalf("no loadSessionsMsg produced by storageChangedMsg cmd (got %d msgs)", len(msgs))
	return loadSessionsMsg{}
}

// drainCmd executes a tea.Cmd, flattening tea.BatchMsg, and returns all
// non-nil messages produced.
func drainCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, drainCmd(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

// runTeardownResult finishes the teardown for id the way the production cmd
// does — the delete side effects run inline on the cmd goroutine, not via an
// update-loop hop (which would stall while the user is attached under
// tea.Exec) — and returns the resulting sessionDeletedMsg (not applied).
// Returns nil if the teardown aborted (e.g. session no longer exists).
func runTeardownResult(t *testing.T, h *Home, id, title string) tea.Msg {
	t.Helper()
	if dm, ok := h.finishTeardown(id, title, "").(sessionDeletedMsg); ok {
		return dm
	}
	return nil
}

// apply feeds msg through Update and returns any follow-up cmd (e.g. the
// requeue tick a sessionDeletedMsg gets during a reload window).
func apply(t *testing.T, h *Home, msg tea.Msg) tea.Cmd {
	t.Helper()
	if msg == nil {
		return nil
	}
	model, cmd := h.Update(msg)
	if model.(*Home) != h {
		t.Fatalf("Update returned different model")
	}
	return cmd
}

// chase runs a follow-up cmd chain (requeue ticks) to convergence. Bounded so
// a regression to an infinite requeue loop fails the test instead of hanging.
func chase(t *testing.T, h *Home, cmd tea.Cmd) {
	t.Helper()
	for i := 0; cmd != nil; i++ {
		if i >= 20 {
			t.Fatalf("requeue chain did not converge after %d rounds", i)
		}
		var next tea.Cmd
		for _, m := range drainCmd(cmd) {
			next = apply(t, h, m)
		}
		cmd = next
	}
}

// tickRebuild mimics the tickMsg-driven rebuild used in non-Normal view modes.
func tickRebuild(h *Home) {
	if h.groupViewMode != session.GroupViewNormal {
		selectedBefore := h.captureSelectedItemIdentity()
		h.rebuildFlatItemsPreservingSelection(selectedBefore)
	}
}

// ---------------------------------------------------------------------------
// Reload (fresh clone pointers) lands mid-teardown, target in a nested child
// group. Full teardown flow afterwards.
// ---------------------------------------------------------------------------

func TestTeardown_ReloadLandsMidTeardown_GroupingIntact(t *testing.T) {
	h := buildTeardownFixture(t)
	cursorTo(t, h, "act1")
	startTeardown(t, h)
	checkGroupingIntegrity(t, h, "after teardown start")

	// A storage change arrives and its reload fully lands DURING the teardown.
	reload := beginReload(t, h)
	apply(t, h, reload)
	checkGroupingIntegrity(t, h, "after mid-teardown reload")

	// Teardown finishes; delete cmd runs; deletion message applies.
	delMsg := runTeardownResult(t, h, "act1", "Act One")
	if delMsg == nil {
		t.Fatal("teardownResultMsg produced no sessionDeletedMsg")
	}
	chase(t, h, apply(t, h, delMsg))

	checkGroupingIntegrity(t, h, "after full teardown flow", "act1")
}

// ---------------------------------------------------------------------------
// sessionDeletedMsg arrives while isReloading=true, with the queued
// loadSessionsMsg landing after. The teardown's destructive side effects
// (tmux kill, worktree removal) have already run in the cmd. The old code
// DROPPED the deletion here — session never removed from UI or DB, ghost row
// until restart. It must now be requeued and applied once the reload settles.
// ---------------------------------------------------------------------------

func TestTeardown_DeleteDuringReload_RequeuedNotLost(t *testing.T) {
	h := buildTeardownFixture(t)
	cursorTo(t, h, "act1")
	startTeardown(t, h)

	// Teardown finishes first; the delete cmd runs its destructive side
	// effects and produces sessionDeletedMsg...
	delMsg := runTeardownResult(t, h, "act1", "Act One")
	if delMsg == nil {
		t.Fatal("teardownResultMsg produced no sessionDeletedMsg")
	}

	// ...but before it is processed, a storage change (e.g. the torn-down
	// agent's own status flip persisted by the status worker, or fleet churn)
	// starts a reload: isReloading=true.
	reload := beginReload(t, h)

	// sessionDeletedMsg lands during the reload window → must be requeued.
	requeue := apply(t, h, delMsg)
	if requeue == nil {
		t.Fatal("sessionDeletedMsg during reload must return a requeue cmd")
	}
	// The queued reload lands (its DB snapshot predates the delete), then the
	// requeued deletion applies.
	apply(t, h, reload)
	chase(t, h, requeue)

	checkGroupingIntegrity(t, h, "after requeued delete", "act1")
	if insts, _, err := h.storage.LoadWithGroups(); err == nil {
		for _, in := range insts {
			if in.ID == "act1" {
				t.Errorf("torn-down session act1 still present in DB (DeleteInstance never ran)")
			}
		}
	}
}

// Same interleaving through the plain `d` delete path (non-MAIA session), to
// establish the hazard is fixed for both flavors.
func TestDDelete_DuringReload_RequeuedNotLost(t *testing.T) {
	h := buildTeardownFixture(t)
	cursorTo(t, h, "oth1")

	// d → plain delete confirm → confirmAction returns the deleteSession cmd.
	if cmd := pressKey(t, h, 'd'); cmd != nil {
		t.Fatalf("d should only open the confirm dialog, got cmd")
	}
	if h.confirmDialog.GetConfirmType() != ConfirmDeleteSession {
		t.Fatalf("expected plain delete confirmation for non-MAIA session, got %v", h.confirmDialog.GetConfirmType())
	}
	cmd := h.confirmAction()
	if cmd == nil {
		t.Fatal("confirmAction returned no delete cmd")
	}
	var delMsg tea.Msg
	for _, m := range drainCmd(cmd) {
		if dm, ok := m.(sessionDeletedMsg); ok {
			delMsg = dm
		}
	}
	if delMsg == nil {
		t.Fatal("delete cmd produced no sessionDeletedMsg")
	}

	reload := beginReload(t, h)
	requeue := apply(t, h, delMsg) // requeued, not dropped
	apply(t, h, reload)
	chase(t, h, requeue)

	checkGroupingIntegrity(t, h, "after requeued d-delete", "oth1")
}

// ---------------------------------------------------------------------------
// Two teardowns completing out of order, with a reload landing between the
// completions.
// ---------------------------------------------------------------------------

func TestTeardown_TwoTeardownsOutOfOrder_WithReloadBetween(t *testing.T) {
	h := buildTeardownFixture(t)

	cursorTo(t, h, "act1")
	startTeardown(t, h)
	cursorTo(t, h, "act2")
	startTeardown(t, h)

	// Second teardown finishes FIRST.
	del2 := runTeardownResult(t, h, "act2", "Act Two")
	chase(t, h, apply(t, h, del2))
	checkGroupingIntegrity(t, h, "after act2 deleted", "act2")

	// Reload lands between the two completions (reads post-act2-delete DB).
	reload := beginReload(t, h)
	apply(t, h, reload)
	checkGroupingIntegrity(t, h, "after mid reload", "act2")

	// First teardown finishes last.
	del1 := runTeardownResult(t, h, "act1", "Act One")
	chase(t, h, apply(t, h, del1))
	checkGroupingIntegrity(t, h, "after act1 deleted", "act1", "act2")
}

// ---------------------------------------------------------------------------
// GroupViewActiveTop with tick-driven rebuilds between every step.
// ---------------------------------------------------------------------------

func TestTeardown_ActiveTopViewMode_TickRebuilds(t *testing.T) {
	h := buildTeardownFixture(t)
	h.groupViewMode = session.GroupViewActiveTop
	h.rebuildFlatItems()
	checkGroupingIntegrity(t, h, "activetop initial")

	cursorTo(t, h, "act1")
	startTeardown(t, h)
	tickRebuild(h)
	checkGroupingIntegrity(t, h, "activetop after teardown start+tick")

	reload := beginReload(t, h)
	tickRebuild(h)
	apply(t, h, reload)
	tickRebuild(h)
	checkGroupingIntegrity(t, h, "activetop after reload+ticks")

	delMsg := runTeardownResult(t, h, "act1", "Act One")
	tickRebuild(h)
	chase(t, h, apply(t, h, delMsg))
	tickRebuild(h)
	checkGroupingIntegrity(t, h, "activetop after delete+ticks", "act1")
}

// ---------------------------------------------------------------------------
// Out-of-order reload application. storageChangedMsg dispatches an async DB
// read per event; bubbletea gives NO ordering guarantee between two in-flight
// cmd goroutines. Without the loadSessionsMsg staleness guard, an OLDER DB
// snapshot could apply after a newer one and rebuild the whole in-memory
// world from stale data — THE post-teardown grouping corruption ("sessions
// render under wrong groups, disk is fine, until restart"). A teardown emits
// a burst of DB writes near its completion (status flips of the dying agent,
// DeleteInstance, SaveWithGroups), so overlapping reloads cluster exactly
// then.
//
// Repro A: a reload snapshot read BEFORE the deletion must not resurrect the
// torn-down session by applying AFTER it.
// ---------------------------------------------------------------------------

func TestTeardown_OutOfOrderReloads_StaleSnapshotDropped(t *testing.T) {
	h := buildTeardownFixture(t)
	cursorTo(t, h, "act1")
	startTeardown(t, h)

	// Fleet churn during the minutes-long teardown: two storage events land
	// back to back. R1's DB read happens now (snapshot still contains act1);
	// its apply is delayed (slow goroutine). R2 is dispatched right after and
	// completes first.
	r1 := beginReload(t, h) // stale-to-be snapshot, in flight
	r2 := beginReload(t, h)
	apply(t, h, r2) // fresh reload lands; isReloading=false again

	// Teardown finishes; deletion fully applies (memory + DB).
	delMsg := runTeardownResult(t, h, "act1", "Act One")
	if delMsg == nil {
		t.Fatal("no sessionDeletedMsg")
	}
	chase(t, h, apply(t, h, delMsg))
	checkGroupingIntegrity(t, h, "after delete applied", "act1")

	// The straggler stale reload finally applies — the staleness guard must
	// drop it (it used to resurrect act1 into tree + flatItems).
	apply(t, h, r1)

	if insts, _, err := h.storage.LoadWithGroups(); err == nil {
		for _, in := range insts {
			if in.ID == "act1" {
				t.Errorf("act1 present in DB — unexpected")
			}
		}
	}
	checkGroupingIntegrity(t, h, "after stale reload dropped", "act1")
}

// Repro B: same out-of-order reload race, applied to a concurrent group MOVE
// (e.g. conductor shifts a session between groups during the teardown
// window). Without the guard, the stale snapshot applied last and the session
// rendered under its OLD group in memory while the DB held the new one.
func TestOutOfOrderReloads_GroupMoveNotRevertedByStaleSnapshot(t *testing.T) {
	h := buildTeardownFixture(t)

	// R1 reads the DB now: root1 still in "maia". Its apply will be delayed.
	r1 := beginReload(t, h)

	// External writer (conductor CLI) moves root1 maia -> other and persists.
	insts, groups, err := h.storage.LoadWithGroups()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, in := range insts {
		if in.ID == "root1" {
			in.GroupPath = "other"
		}
	}
	tree := session.NewGroupTreeWithGroups(insts, groups)
	if err := h.storage.SaveWithGroups(insts, tree); err != nil {
		t.Fatalf("external save: %v", err)
	}

	// R2 reads the post-move DB and applies first.
	r2 := beginReload(t, h)
	apply(t, h, r2)
	if got := h.instanceByID["root1"].GroupPath; got != "other" {
		t.Fatalf("fresh reload should show root1 in 'other', got %q", got)
	}

	// The stale R1 read applies last — must be dropped by the version guard.
	apply(t, h, r1)

	checkGroupingIntegrity(t, h, "after stale reload")
	if got := h.instanceByID["root1"].GroupPath; got != "other" {
		t.Errorf("root1 renders under stale group %q; DB says \"other\" (stale reload applied last)", got)
	}
}

// ---------------------------------------------------------------------------
// Exhaustive interleaving sweep: one reload cycle (read R, apply A) and one
// teardown completion (result+deleteCmd T, deletion apply D) in every legal
// order (R before A; T before D), across both view modes, with tick rebuilds
// after each step. Verifies grouping invariants at every intermediate state
// and that the deletion always lands (directly or via requeue).
// ---------------------------------------------------------------------------

func TestTeardown_InterleavingSweep(t *testing.T) {
	type step byte // 'R' begin reload, 'A' apply reload, 'T' teardown result, 'D' apply delete
	schedules := [][]step{
		{'R', 'A', 'T', 'D'},
		{'R', 'T', 'A', 'D'},
		{'R', 'T', 'D', 'A'},
		{'T', 'R', 'A', 'D'},
		{'T', 'R', 'D', 'A'},
		{'T', 'D', 'R', 'A'},
	}
	for _, mode := range []session.GroupViewMode{session.GroupViewNormal, session.GroupViewActiveTop} {
		for _, sched := range schedules {
			name := fmt.Sprintf("mode%d_%s", mode, string(func() []byte {
				b := make([]byte, len(sched))
				for i, s := range sched {
					b[i] = byte(s)
				}
				return b
			}()))
			t.Run(name, func(t *testing.T) {
				h := buildTeardownFixture(t)
				h.groupViewMode = mode
				h.rebuildFlatItems()
				cursorTo(t, h, "act1")
				startTeardown(t, h)

				var pendingReload tea.Msg
				var pendingDelete tea.Msg
				var pendingRequeue tea.Cmd
				for _, s := range sched {
					switch s {
					case 'R':
						pendingReload = beginReload(t, h)
					case 'A':
						apply(t, h, pendingReload)
					case 'T':
						pendingDelete = runTeardownResult(t, h, "act1", "Act One")
					case 'D':
						pendingRequeue = apply(t, h, pendingDelete)
					}
					tickRebuild(h)
					checkGroupingIntegrity(t, h, "step "+string(s))
				}
				// Any requeued deletion converges once the reload has settled.
				chase(t, h, pendingRequeue)

				if _, stillThere := h.instanceByID["act1"]; stillThere {
					t.Errorf("act1 still present after schedule completed — deletion lost")
				}
				checkGroupingIntegrity(t, h, "final", "act1")
			})
		}
	}
}
