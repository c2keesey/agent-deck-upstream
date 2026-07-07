package ui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const (
	hotkeyQuit             = "quit"
	hotkeyNewSession       = "new_session"
	hotkeyQuickCreate      = "quick_create"
	hotkeyRename           = "rename"
	hotkeyRestart          = "restart"
	hotkeyHardRestart      = "hard_restart"
	hotkeyRestartFresh     = "restart_fresh"
	hotkeyDelete           = "delete"
	hotkeyCloseSession     = "close_session"
	hotkeyArchiveSession   = "archive_session"
	hotkeyUnarchiveSession = "unarchive_session"
	hotkeyViewArchived     = "view_archived"
	hotkeyUndoDelete       = "undo_delete"
	hotkeyMoveToGroup      = "move_to_group"
	hotkeyMCPManager       = "mcp_manager"
	hotkeyPluginManager    = "plugin_manager"
	hotkeySkillsManager    = "skills_manager"
	hotkeyTogglePreview    = "toggle_preview"
	hotkeyCycleGroupView   = "cycle_group_view"
	hotkeyMarkUnread       = "mark_unread"
	hotkeyQuickApprove     = "quick_approve"
	hotkeyPromptSession    = "prompt_session" // #1410: prompt the highlighted session without attaching
	hotkeyToggleYolo       = "toggle_yolo"
	hotkeyQuickFork        = "quick_fork"
	hotkeyForkWithOptions  = "fork_with_options"
	hotkeyCopyOutput       = "copy_output"
	hotkeySendOutput       = "send_output"
	hotkeyExecShell        = "exec_shell"
	hotkeyEditNotes        = "edit_notes"
	hotkeyEditPaths        = "edit_paths"
	hotkeyEditSession      = "edit_session"
	hotkeyWorktreeSetup    = "worktree_setup"
	hotkeyWorktreeFinish   = "worktree_finish"
	hotkeyCreateGroup      = "create_group"
	hotkeySearch           = "search"
	hotkeyHelp             = "help"
	hotkeySettings         = "settings"
	hotkeyImport           = "import"
	hotkeyReload           = "reload"
	hotkeyDetach           = "detach"
	hotkeyWatcherPanel     = "watcher_panel"
	// Local fork additions.
	hotkeyMRUCycle       = "mru_cycle"
	hotkeyAttentionCycle = "attention_cycle"
	// Session switcher. While attached it is intercepted in the tmux attach
	// loop (see internal/tmux/pty.go AttachOptions); on the home screen it is
	// dispatched like any other hotkey. Must resolve to a "ctrl+<letter>" chord.
	//
	// LOCAL fork: bound to Ctrl+W by default (see defaultHotkeyBindings), so the
	// canonical dispatch token is "ctrl+w" and the home-screen arm is
	// `case "ctrl+w"`. Upstream ships this unbound/opt-in on Ctrl+S; the fork
	// keeps it always-on because the MRU-ordered switcher is the primary switch
	// UX. Ctrl+S stayed problematic (XOFF / Cmd+Right on some terminals).
	hotkeySwitchSession = "switch_session" // canonical "ctrl+w" (local remap)
)

var hotkeyActionOrder = []string{
	hotkeyQuit,
	hotkeyNewSession,
	hotkeyQuickCreate,
	hotkeyRename,
	hotkeyRestart,
	hotkeyHardRestart,
	hotkeyRestartFresh,
	hotkeyDelete,
	hotkeyCloseSession,
	hotkeyArchiveSession,
	hotkeyUnarchiveSession,
	hotkeyViewArchived,
	hotkeyUndoDelete,
	hotkeyMoveToGroup,
	hotkeyMCPManager,
	hotkeyPluginManager,
	hotkeySkillsManager,
	hotkeyTogglePreview,
	hotkeyCycleGroupView,
	hotkeyMarkUnread,
	hotkeyQuickApprove,
	hotkeyPromptSession,
	hotkeyToggleYolo,
	hotkeyQuickFork,
	hotkeyForkWithOptions,
	hotkeyCopyOutput,
	hotkeySendOutput,
	hotkeyExecShell,
	hotkeyEditNotes,
	hotkeyEditPaths,
	hotkeyEditSession,
	hotkeyWorktreeSetup,
	hotkeyWorktreeFinish,
	hotkeyCreateGroup,
	hotkeySearch,
	hotkeyHelp,
	hotkeySettings,
	hotkeyImport,
	hotkeyReload,
	hotkeyDetach,
	hotkeyMRUCycle,
	hotkeyAttentionCycle,
	hotkeyWatcherPanel,
	hotkeySwitchSession,
}

// defaultHotkeyBindings keeps the local fork keymap as the source of truth
// (the user relies on this muscle memory: attention=ctrl+e, mru=ctrl+w,
// hard_restart=ctrl+t, restart=t, settings=p, quick_create=a, …).
// Upstream's v1.9.x keybinding overhaul (restart→R, close→D, move→M, settings→S,
// toggle_yolo→y, quick_create→N, etc.) is intentionally NOT adopted. Upstream's
// genuinely-new actions are grafted onto free keys when their upstream default
// collides with a local binding: cycle_group_view→V (upstream t=restart),
// worktree_setup→B (upstream b=exec_shell), quick_approve→ctrl+a (upstream
// a=quick_create). toggle_yolo stays unbound (the former teardown key "y" is
// deliberately left free — teardown now rides the delete flow on "d" for MAIA
// worktree sessions, and giving "y" a new destructive meaning would fight
// muscle memory).
var defaultHotkeyBindings = map[string]string{
	hotkeyQuit:             "q",
	hotkeyNewSession:       "n",
	hotkeyQuickCreate:      "a",
	hotkeyRename:           "r",
	hotkeyRestart:          "t",
	hotkeyHardRestart:      "ctrl+t",
	hotkeyRestartFresh:     "T",
	hotkeyDelete:           "d",
	hotkeyCloseSession:     "ctrl+x",
	hotkeyArchiveSession:   "A",
	hotkeyUnarchiveSession: "shift+u",
	hotkeyViewArchived:     "^",
	hotkeyUndoDelete:       "ctrl+z",
	hotkeyMoveToGroup:      "o",
	hotkeyMCPManager:       "m",
	hotkeyPluginManager:    "L",
	hotkeySkillsManager:    "s",
	hotkeyTogglePreview:    "v",
	hotkeyCycleGroupView:   "V", // upstream "t" collides with local restart; use shift+v (View)
	hotkeyMarkUnread:       "u",
	hotkeyQuickApprove:     "ctrl+a", // local: 'a' is quick-create; user runs bypass-permissions so approve is parked off the home row
	hotkeyPromptSession:    "O",      // upstream #1410 defaults this to "o", which collides with local move_to_group; remapped to shift+O (pairs with lowercase o)
	hotkeyToggleYolo:       "",       // stays unbound; "y" is intentionally left free (ex-teardown key)
	hotkeyQuickFork:        "f",
	hotkeyForkWithOptions:  "z",
	hotkeyCopyOutput:       "c",
	hotkeySendOutput:       "x",
	hotkeyExecShell:        "b",
	hotkeyEditNotes:        "e",
	hotkeyEditPaths:        "",
	hotkeyEditSession:      "",
	hotkeyWorktreeSetup:    "B", // upstream "b" collides with local exec_shell; use shift+b
	hotkeyWorktreeFinish:   "w",
	hotkeyCreateGroup:      "g",
	hotkeySearch:           "/",
	hotkeyHelp:             "?",
	hotkeySettings:         "p",
	hotkeyImport:           "i",
	hotkeyReload:           "ctrl+r",
	hotkeyDetach:           "ctrl+q",
	// mru_cycle is UNBOUND: the local blind MRU cursor-cycle is superseded by
	// upstream's MRU-ordered session switcher, bound to ctrl+w below.
	hotkeyMRUCycle:       "",
	hotkeyAttentionCycle: "ctrl+e",
	hotkeyWatcherPanel:   "",
	// switch_session = ctrl+w (local remap). Upstream defaults this to Ctrl+S,
	// but Ctrl+S is XOFF / what some terminals send for Cmd+Right, so it kept
	// triggering unexpectedly. Ctrl+W is the user's old MRU key and the switcher
	// is MRU-ordered, so it cleanly replaces the local MRU cursor-cycle. The old
	// overview Ctrl+S handler stays removed from handleMainKey.
	hotkeySwitchSession: "ctrl+w",
}

var hotkeyActionDefaultTriggers = map[string][]string{
	hotkeyQuit:            {"q", "ctrl+c"},
	hotkeyForkWithOptions: {"z"},
	hotkeyMoveToGroup:     {"o"},
	hotkeyWorktreeFinish:  {"w"},
}

// renamedHotkeys maps old action names to new names for backward compatibility.
var renamedHotkeys = map[string]string{
	"toggle_gemini_yolo": hotkeyToggleYolo,
}

// defaultDisabledHotkeys are actions that keep a canonical key in
// defaultHotkeyBindings (so the home-screen dispatch case and help/status
// labels resolve) but ship UNBOUND: resolveHotkeys drops them unless the user
// binds them explicitly. Upstream lists switch_session here (opt-in Ctrl+S); the
// LOCAL fork instead remaps switch_session to Ctrl+W and keeps it BOUND by
// default (the MRU-ordered switcher is the fork's primary switch UX), so it is
// deliberately NOT disabled here. The map stays for any future upstream opt-in
// actions.
var defaultDisabledHotkeys = map[string]bool{}

func resolveHotkeys(overrides map[string]string) map[string]string {
	bindings := make(map[string]string, len(defaultHotkeyBindings))
	for action, key := range defaultHotkeyBindings {
		bindings[action] = key
	}

	overrideActions := make([]string, 0, len(overrides))
	for action := range overrides {
		overrideActions = append(overrideActions, action)
	}
	sort.Strings(overrideActions)

	canonicalOverrides := make(map[string]string, len(overrides))
	for _, action := range overrideActions {
		key := overrides[action]
		normalizedAction := strings.TrimSpace(strings.ToLower(action))
		normalizedKey := strings.TrimSpace(key)

		if _, ok := defaultHotkeyBindings[normalizedAction]; ok {
			canonicalOverrides[normalizedAction] = normalizedKey
		}
	}
	for _, action := range overrideActions {
		key := overrides[action]
		normalizedAction := strings.TrimSpace(strings.ToLower(action))
		newName, ok := renamedHotkeys[normalizedAction]
		if !ok {
			continue
		}
		if _, exists := canonicalOverrides[newName]; exists {
			continue
		}
		canonicalOverrides[newName] = strings.TrimSpace(key)
	}

	for action, key := range canonicalOverrides {
		if key == "" {
			delete(bindings, action)
			continue
		}
		bindings[action] = key
	}

	// Opt-in actions ship unbound: drop them unless the user set them
	// explicitly. The canonical default stays in defaultHotkeyBindings so the
	// dispatch case and labels resolve once a user binds it.
	for action := range defaultDisabledHotkeys {
		if _, overridden := canonicalOverrides[action]; !overridden {
			delete(bindings, action)
		}
	}

	return bindings
}

func buildHotkeyLookup(bindings map[string]string) (map[string]string, map[string]bool) {
	keyToCanonical := make(map[string]string, len(bindings))
	blockedCanonical := make(map[string]bool)

	for _, action := range hotkeyActionOrder {
		canonical := defaultHotkeyBindings[action]
		bound := strings.TrimSpace(bindings[action])
		defaultTriggers := defaultTriggersForAction(action)
		if bound == "" {
			for _, trigger := range defaultTriggers {
				blockedCanonical[trigger] = true
			}
			continue
		}
		if bound != canonical {
			for _, trigger := range defaultTriggers {
				blockedCanonical[trigger] = true
			}
		}
		for _, alias := range hotkeyAliases(bound) {
			if _, exists := keyToCanonical[alias]; !exists {
				keyToCanonical[alias] = canonical
			}
		}
	}

	return keyToCanonical, blockedCanonical
}

func defaultTriggersForAction(action string) []string {
	if triggers, ok := hotkeyActionDefaultTriggers[action]; ok {
		return triggers
	}
	return hotkeyAliases(defaultHotkeyBindings[action])
}

func hotkeyAliases(key string) []string {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return nil
	}

	aliases := []string{trimmed}
	seen := map[string]bool{trimmed: true}
	add := func(alias string) {
		alias = strings.TrimSpace(alias)
		if alias == "" || seen[alias] {
			return
		}
		seen[alias] = true
		aliases = append(aliases, alias)
	}

	if shiftAlias := shiftedAliasFor(trimmed); shiftAlias != "" {
		add(shiftAlias)
	}
	if unshiftedAlias := unshiftedAliasFor(trimmed); unshiftedAlias != "" {
		add(unshiftedAlias)
	}

	return aliases
}

func shiftedAliasFor(key string) string {
	runes := []rune(key)
	if len(runes) != 1 {
		return ""
	}

	r := runes[0]
	if unicode.IsUpper(r) {
		return "shift+" + strings.ToLower(string(r))
	}

	switch r {
	case '!':
		return "shift+1"
	case '@':
		return "shift+2"
	case '#':
		return "shift+3"
	case '$':
		return "shift+4"
	case '%':
		return "shift+5"
	case '^':
		return "shift+6"
	case '&':
		return "shift+7"
	case '*':
		return "shift+8"
	case '(':
		return "shift+9"
	case ')':
		return "shift+0"
	}

	return ""
}

func unshiftedAliasFor(key string) string {
	lower := strings.ToLower(strings.TrimSpace(key))
	if !strings.HasPrefix(lower, "shift+") {
		return ""
	}

	base := strings.TrimSpace(lower[len("shift+"):])
	runes := []rune(base)
	if len(runes) != 1 {
		return ""
	}

	r := runes[0]
	if unicode.IsLetter(r) {
		return strings.ToUpper(string(r))
	}

	switch r {
	case '1':
		return "!"
	case '2':
		return "@"
	case '3':
		return "#"
	case '4':
		return "$"
	case '5':
		return "%"
	case '6':
		return "^"
	case '7':
		return "&"
	case '8':
		return "*"
	case '9':
		return "("
	case '0':
		return ")"
	}

	return ""
}

func actionHotkey(bindings map[string]string, action string) string {
	if bindings == nil {
		return ""
	}
	return strings.TrimSpace(bindings[action])
}

func joinHotkeyLabels(keys ...string) string {
	filtered := make([]string, 0, len(keys))
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return strings.Join(filtered, "/")
}

// DetachByteFromBinding converts a hotkey binding string (e.g. "ctrl+q") to the
// corresponding ASCII byte used by the PTY attach loop. Returns 0x11 (Ctrl+Q) as
// the default when the binding cannot be mapped.
func DetachByteFromBinding(binding string) byte {
	binding = strings.ToLower(strings.TrimSpace(binding))
	if !strings.HasPrefix(binding, "ctrl+") {
		return 17 // default Ctrl+Q
	}
	ch := binding[len("ctrl+"):]
	if len(ch) == 1 && ch[0] >= 'a' && ch[0] <= 'z' {
		return ch[0] - 'a' + 1
	}
	switch ch {
	case "\\":
		return 0x1C
	case "]":
		return 0x1D
	case "^":
		return 0x1E
	case "_":
		return 0x1F
	}
	return 17 // default Ctrl+Q
}

// DetachByteLabel returns a human-readable label for a detach byte (e.g. "Ctrl+Q").
func DetachByteLabel(b byte) string {
	if b >= 1 && b <= 26 {
		return fmt.Sprintf("Ctrl+%c", 'A'+b-1)
	}
	switch b {
	case 0x1C:
		return "Ctrl+\\"
	case 0x1D:
		return "Ctrl+]"
	case 0x1E:
		return "Ctrl+^"
	case 0x1F:
		return "Ctrl+_"
	}
	return "Ctrl+Q"
}

// ResolvedDetachByte returns the detach byte for the current hotkey configuration.
func ResolvedDetachByte(overrides map[string]string) byte {
	bindings := resolveHotkeys(overrides)
	key := actionHotkey(bindings, hotkeyDetach)
	if key == "" {
		return 17 // default Ctrl+Q
	}
	return DetachByteFromBinding(key)
}

// ctrlByteFromBinding converts a "ctrl+<letter>" binding to its control byte, or
// returns 0 when the binding is not a single-control-key chord. Unlike
// DetachByteFromBinding it does not fall back to Ctrl+Q, so callers can treat 0
// as "no portable byte for this key" (e.g. "ctrl+tab" / "ctrl+shift+tab", which
// have no legacy control byte).
func ctrlByteFromBinding(binding string) byte {
	binding = strings.ToLower(strings.TrimSpace(binding))
	if !strings.HasPrefix(binding, "ctrl+") {
		return 0
	}
	ch := binding[len("ctrl+"):]
	if len(ch) == 1 && ch[0] >= 'a' && ch[0] <= 'z' {
		return ch[0] - 'a' + 1
	}
	switch ch {
	case "\\":
		return 0x1C
	case "]":
		return 0x1D
	case "^":
		return 0x1E
	case "_":
		return 0x1F
	}
	return 0
}

// ResolvedSwitchByte returns the control byte that opens the in-attach session
// switcher for the current hotkey overrides, or 0 when it is unbound or not a
// ctrl+<letter> chord. The switcher's forward/backward cycling and commit are
// handled in the TUI, so only this single opener byte reaches the attach loop.
func ResolvedSwitchByte(overrides map[string]string) byte {
	bindings := resolveHotkeys(overrides)
	return ctrlByteFromBinding(actionHotkey(bindings, hotkeySwitchSession))
}
