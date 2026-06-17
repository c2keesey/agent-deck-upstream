// Priority field (local fork): conductor-assigned session priority that
// drives the Ctrl+E attention cycle. 0 = unset, 1 (highest) .. 3 (lowest).
//
// Persistence mirrors the #1143 idle-timeout pattern: the value rides in the
// tool_data extras zone so legacy binaries round-trip it untouched.
package session

import (
	"strings"
	"testing"
)

func TestPriority_PersistenceRoundTrip(t *testing.T) {
	td := WritePriorityToToolData(nil, 1)
	if got := ReadPriorityFromToolData(td); got != 1 {
		t.Fatalf("ReadPriorityFromToolData after Write = %d, want 1", got)
	}

	// Setting back to 0 removes the key (blob identical to a legacy row).
	cleared := WritePriorityToToolData(td, 0)
	if got := ReadPriorityFromToolData(cleared); got != 0 {
		t.Fatalf("Write(td, 0) should clear, got %d", got)
	}
	if strings.Contains(string(cleared), "priority") {
		t.Fatalf("cleared blob still contains priority key: %s", string(cleared))
	}

	// Round-trip preserves unrelated fields.
	mixed := []byte(`{"color":"#ff00aa","idle_timeout_secs":600}`)
	out := WritePriorityToToolData(mixed, 2)
	if got := ReadPriorityFromToolData(out); got != 2 {
		t.Fatalf("round-trip with extras lost priority: got %d", got)
	}
	if !strings.Contains(string(out), `"color":"#ff00aa"`) {
		t.Fatalf("round-trip dropped color: %s", string(out))
	}
	if got := ReadIdleTimeoutSecsFromToolData(out); got != 600 {
		t.Fatalf("round-trip dropped idle_timeout_secs: got %d", got)
	}
}

func TestPriority_SQLiteRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage := newTestStorage(t)

	inst := NewInstance("priority-roundtrip", "/tmp")
	inst.Tool = "shell"
	inst.Priority = 1

	groupTree := NewGroupTreeWithGroups([]*Instance{inst}, nil)
	if err := storage.SaveWithGroups([]*Instance{inst}, groupTree); err != nil {
		t.Fatalf("SaveWithGroups: %v", err)
	}

	loaded, _, err := storage.LoadWithGroups()
	if err != nil {
		t.Fatalf("LoadWithGroups: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(loaded))
	}
	if loaded[0].Priority != 1 {
		t.Fatalf("Priority not preserved across SQLite round-trip: got %d, want 1", loaded[0].Priority)
	}
}

// Regression: clearing priority (set 0) must actually remove it from the DB.
// The tool_data extras-merge layer carries forward keys absent from the typed
// schema; without "priority" being a known key, a cleared value got re-added
// from the old row. priority is modeled in statedb.toolDataBlob to make the
// clear authoritative.
func TestPriority_SQLiteClearRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage := newTestStorage(t)

	inst := NewInstance("priority-clear", "/tmp")
	inst.Tool = "shell"
	inst.Priority = 2

	groupTree := NewGroupTreeWithGroups([]*Instance{inst}, nil)
	if err := storage.SaveWithGroups([]*Instance{inst}, groupTree); err != nil {
		t.Fatalf("SaveWithGroups (set): %v", err)
	}

	// Now clear it and save again over the existing row.
	inst.Priority = 0
	if err := storage.SaveWithGroups([]*Instance{inst}, groupTree); err != nil {
		t.Fatalf("SaveWithGroups (clear): %v", err)
	}

	loaded, _, err := storage.LoadWithGroups()
	if err != nil {
		t.Fatalf("LoadWithGroups: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(loaded))
	}
	if loaded[0].Priority != 0 {
		t.Fatalf("Priority not cleared across SQLite round-trip: got %d, want 0", loaded[0].Priority)
	}
}

func TestPriority_SetField(t *testing.T) {
	inst := NewInstance("priority-set", "/tmp")
	inst.Tool = "shell"

	old, post, err := SetField(inst, FieldPriority, "2", nil)
	if err != nil {
		t.Fatalf("SetField(priority, 2): %v", err)
	}
	if post != nil {
		t.Fatalf("priority should not need a postCommit hook")
	}
	if old != "0" {
		t.Fatalf("old value = %q, want \"0\"", old)
	}
	if inst.Priority != 2 {
		t.Fatalf("Priority = %d, want 2", inst.Priority)
	}

	// Empty string and "0" both clear.
	if _, _, err := SetField(inst, FieldPriority, "", nil); err != nil {
		t.Fatalf("SetField(priority, \"\"): %v", err)
	}
	if inst.Priority != 0 {
		t.Fatalf("Priority after clear = %d, want 0", inst.Priority)
	}

	// Out-of-range and junk rejected.
	for _, bad := range []string{"4", "-1", "high", "1.5"} {
		if _, _, err := SetField(inst, FieldPriority, bad, nil); err == nil {
			t.Fatalf("SetField(priority, %q) should fail", bad)
		}
	}

	// Live field — no restart required.
	if RestartPolicyFor(FieldPriority) != FieldLive {
		t.Fatalf("priority should be a live field")
	}
}
