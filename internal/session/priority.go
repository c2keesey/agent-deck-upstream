// Priority field (local fork): conductor-assigned session priority driving
// the Ctrl+E attention cycle. A strict rank — 1 = highest, unique per session,
// no upper bound. 0 = unset (sorts last). Ranks express a strict total order,
// not capped buckets.
//
// Persistence mirrors the #1143 idle-timeout pattern: the value rides in the
// tool_data extras zone, so the positional MarshalToolData signature stays
// untouched and legacy binaries preserve the key via MergeToolDataExtras.
package session

import (
	"encoding/json"
	"fmt"
	"strconv"
)

const toolDataPriorityKey = "priority"

// WritePriorityToToolData merges priority into the given tool_data JSON blob.
// Passing prio == 0 removes the key, keeping the blob identical to a
// pre-priority row so downgrades stay clean.
func WritePriorityToToolData(td json.RawMessage, prio int) json.RawMessage {
	m := map[string]json.RawMessage{}
	if len(td) > 0 {
		_ = json.Unmarshal(td, &m)
	}
	if prio > 0 {
		raw, _ := json.Marshal(prio)
		m[toolDataPriorityKey] = raw
	} else {
		delete(m, toolDataPriorityKey)
	}
	out, _ := json.Marshal(m)
	return out
}

// ReadPriorityFromToolData extracts priority from the blob.
// Returns 0 (unset) for missing/malformed/legacy rows.
func ReadPriorityFromToolData(td json.RawMessage) int {
	if len(td) == 0 {
		return 0
	}
	var blob struct {
		Priority int `json:"priority"`
	}
	_ = json.Unmarshal(td, &blob)
	return blob.Priority
}

// ParsePriorityFlag parses a priority value from CLI/TUI input.
// Accepts "" / "0" (clear) and any positive integer rank (1 = highest, unique
// per session, no upper bound); negatives and junk error.
func ParsePriorityFlag(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid priority %q — expected 0 (clear) or a strict rank ≥ 1 (1 = highest, unique per session)", value)
	}
	return n, nil
}
