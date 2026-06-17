# Quick Note / Idea Capture — Design Doc

**Status:** Implemented (v1) — Ctrl+Alt+I capture from inside a session
**Date:** 2026-06-14
**Owner:** c2k

## Problem

Whenever an idea strikes mid-work, it's *too easy* to act on it immediately: spin
up a new agent-deck session and start building. That forks attention. Every idea
becomes a new process in working memory — a thread to start, babysit, and clean
up — which disrupts the primary work in flight.

Concrete example: while doing main work, an idea to improve this local fork of
agent-deck appears. Today that means launching a new session right then, adding
cognitive overhead and context-switching cost.

The goal is the opposite: **capture the idea in seconds and immediately return to
the main task.** No new session, no forked attention. The idea lands in a durable
backlog to revisit deliberately later.

## Goals

- **Non-interruptive** — capturing an idea must not pull focus from current work.
- **Fast** — near-zero friction; a few keystrokes, no navigation, no context loss.
- **Durable** — stored somewhere stable that survives across sessions/restarts.
- **Reviewable later** — a backlog I can pull up, scan, and process deliberately.

Mental model: a **backlog of tasks/ideas** to track and triage on my own schedule.

## Non-goals (for v1)

- Not a full project/issue tracker (beads/Linear already exist for that).
- Not auto-execution — capture ≠ start work. Acting on an idea is a later, explicit step.

## Design directions under consideration

1. **agent-deck integrated** — a quick-capture hotkey/command inside the
   agent-deck TUI that appends an idea to a backlog store, plus a view to review it.
2. **Claude Code integrated ("by the way" style)** — capture an idea inline in the
   current chat stream without breaking flow, more like a side-channel note that
   doesn't interrupt the active conversation.

v1 leans toward whichever is fastest to reach from where ideas actually strike.
Decision pending interview.

## Decisions (locked after interview)

| Dimension | Decision |
|-----------|----------|
| Capture surface | **From inside a live Claude Code session** (the "by the way" experience). Triggered via a **tmux root-table key binding + `display-popup`**, since while attached the keystrokes go to tmux/Claude — not agent-deck's TUI. Scoped to agentdeck-managed sessions via `if-shell`, exactly like the existing `C-q` detach bind (`internal/tmux/tmux.go:1988`). |
| Hotkey | **`Ctrl+Alt+I`** (tmux: `C-M-i`) — "I for Idea." Ctrl+Alt is untouched by Claude Code, zsh, and tmux's default tables. One caveat handled in impl: `Ctrl+I` legacy-aliases to Tab, so agent-deck sessions enable tmux **extended-keys** so the combo is reported unambiguously (Ghostty + tmux HEAD both support it). Fallback if it ever collides: `Ctrl+Alt+N`. |
| Storage | **Standalone flat file** at `~/.agent-deck/ideas.md`, append-only markdown, **global across all profiles** (one backlog). |
| Review | **Just open the file.** Optional later: an `agent-deck ideas` subcommand that prints/opens it. |
| Context depth | **Last Claude user message** from the session's transcript, plus always: timestamp, session name, project path, tool/agent type, `ClaudeSessionID`. Non-Claude or unresolved → metadata only, no message line. |
| Non-interruptive guarantee | The popup floats over Claude without sending it any input; Claude keeps running underneath. Enter appends + closes; Esc closes with nothing written. Either way the cursor lands back exactly where it was in Claude. |

## Capture flow

1. While attached to (working inside) a Claude session, press **`Ctrl+Alt+I`**.
2. tmux (root table) intercepts it before Claude sees it and opens a small
   `display-popup` running `agent-deck idea --session <name>`. Session metadata is
   resolved at this instant from the tmux env + transcript.
3. Type the idea. **Enter** appends to `~/.agent-deck/ideas.md` and the popup
   closes. **Esc** closes with nothing written. Claude is exactly as you left it.

## Entry format (`~/.agent-deck/ideas.md`)

Append-only markdown; each idea is one section, human-scannable on open:

```markdown
## 2026-06-14 15:42 — improve fork sync conflict handling

auto-resolve trivial conflicts during local-branch upstream sync

- **session:** local-branch-work (claude)
- **project:** ~/repos/agent-deck
- **group:** projects/devops
- **claude session:** 0c2f…e91
- **last message:** "can you remap ctrl+w to the MRU switcher instead of…"
```

The first line after `##` is the idea text (truncated for the heading); the full
idea text and metadata follow. Context lines are omitted when unavailable.

## Implementation plan (grounded in current code)

**New code**
- `internal/ideas/ideas.go` — `AppendIdea(IdeaEntry) error`: formats an entry and
  appends to `~/.agent-deck/ideas.md` (`O_APPEND|O_CREATE`). Pure + unit-testable.
- `cmd/agent-deck/idea_cmd.go` — new `agent-deck idea [--session NAME]` subcommand,
  meant to run inside the popup:
  1. Resolve metadata: session name (arg / `tmux display-message -p '#{session_name}'`),
     project path (`#{pane_current_path}`), `CLAUDE_SESSION_ID`
     (`tmux show-environment -t <name> CLAUDE_SESSION_ID`), tool type, and the last
     Claude user message from the transcript.
  2. Prompt for the idea text (single-line; a minimal Bubble Tea `textinput` or
     `bufio` read — popup is a real TTY).
  3. `ideas.AppendIdea(...)`. Esc / empty input → no write.

**tmux wiring (the trigger)**
- In `internal/tmux/tmux.go`, next to the `C-q` bind (`:1988`), add a root-table
  bind scoped by `if-shell` to this session:
  `bind-key -n -T root C-M-i if-shell '[ "#{session_name}" = "<name>" ]'
   'display-popup -E -w 60% -h 40% "agent-deck idea --session <name>"'`.
- Ensure unambiguous Ctrl+Alt: set `extended-keys on` +
  `terminal-features ...:extkeys` on agentdeck-managed sessions (near the existing
  `escape-time`/`allow-passthrough` set-options at `:1958–1960`).

**Reuse**
- Instance metadata fields: `Title`, `ProjectPath`, `Tool`, `ClaudeSessionID`
  (`internal/session/instance.go:90–151`).
- Last Claude user message: existing transcript path resolution + tail parse
  (`internal/session/instance.go:5150` `parseClaudeLatestUserPrompt`, `:5225`
  `readJSONLTail`, `:7468–7550` path resolution). Expose a small exported helper
  if one isn't already (callable from the `idea` subcommand given a session ID +
  project path).

**Optional parity (secondary, not required for v1)**
- Also bind the same capture from the agent-deck home TUI (when *not* attached),
  reusing the `FeedbackDialog` overlay pattern (`internal/ui/feedback_dialog.go:56`)
  and `getSelectedSession()` (`internal/ui/home.go:3403`). Deferred unless wanted.

## Testing (per repo mandates)

- **TDD**: tests land before the feature.
- Unit: `AppendIdea` formatting + append semantics (idempotent create, ordering).
- Unit: metadata snapshot from an `Instance`, including last-message extraction
  against a fixture JSONL transcript (Claude) and graceful fallback (non-Claude).
- **Eval mandate** (`CLAUDE.md`): this adds a new interactive prompt/overlay, so an
  `eval_smoke` case under `tests/eval/` is required (capture box appears on `` ` ``,
  Enter writes an entry, Esc writes nothing).

## Deferred (not v1)

- Claude-Code-inline "by the way" capture (v2 direction).
- `agent-deck ideas` review/triage CLI; promote-to-beads bridge.
- Editing/deleting entries from within the TUI.
