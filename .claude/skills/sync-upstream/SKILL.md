---
name: sync-upstream
description: Sync the agent-deck local branch with upstream/main, skipping the main branch entirely. Use when the user says "sync upstream", "pull upstream", "update agent-deck from upstream", "/sync-upstream", or any variant. Performs a direct merge of upstream/main into local with conflict resolution, an auto-run test gate, a feature summary, and an auto-push.
---

# Sync agent-deck local branch with upstream

This is a personal fork (`c2keesey/agent-deck`) of `asheshgoplani/agent-deck`. The user works only on the `local` branch and does not maintain a separate `main` branch — sync `upstream/main` directly into `local`.

## Workflow

1. **Verify state** — Must be on `local` branch with a clean working tree. If dirty, stop and ask the user how to proceed (`git stash` vs. commit vs. abort). If on a different branch, ask before switching.

2. **Fetch and survey** — `git fetch upstream`. Show divergence: how many commits in `upstream/main..local` (your stuff) and `local..upstream/main` (new from upstream). If `local..upstream/main` is empty, report "already current" and stop.

3. **Merge** — `git merge upstream/main`. Do not rebase; the existing history pattern is `Merge branch 'main' into local` / similar.

4. **Resolve conflicts** — For each conflicted file:
   - **ALWAYS PREFER LOCAL FEATURES.** On any conflict where local has a deliberate customization, take the local side — even when upstream ships its own "official" version of a similar feature. Only take upstream for shared code that has genuinely evolved and where local has no intentional divergence. (Memory: `prefer-local-over-upstream`.)
   - **Additive conflicts are unions, not either/or.** Most enum/const/slice/switch-arm conflicts (e.g. `FieldPriority` vs `FieldPin`, the `hotkeyActionOrder` tail, struct fields like `primaryLabels` vs `groupViewMode`) are two independent additions colliding on the same line — keep BOTH sides, don't pick one.
   - If an upstream test enforces upstream's behavior and contradicts a kept local feature, **rewrite that test to pin the local behavior** (don't just delete coverage) and add a comment noting the divergence.
   - **CLAUDE.md**: it is gitignored AND untracked on `local` (not present in either branch) — it never actually conflicts. Ignore it; the on-disk file is the user's private copy. (Historical: upstream untracked it in PR #1002.)
   - After resolving, verify no markers remain: `grep -rn "^<<<<<<<\|^>>>>>>>\|^=======" --include='*.go' .`

   ### The keymap is the #1 recurring conflict — treat the LOCAL keymap as source of truth

   Upstream periodically does keybinding overhauls (v1.9.x remapped restart→R, close→D, move→M, settings→S, quick_create→N, exec_shell→E, worktree_finish→W, toggle_yolo→y, …). **Do NOT adopt upstream's keymap.** Local keeps its own (teardown=y, attention=ctrl+e, mru=ctrl+w, hard_restart=ctrl+t, restart=t, settings=p, exec_shell=b, worktree_finish=w, move=o, close=ctrl+x, quick_approve=a). `internal/ui/hotkeys.go` conflicts in three zones every time:
   - **const block** — union it: take upstream's full set (it's a superset of names) and re-add local-only consts (`hotkeyMRUCycle`, `hotkeyAttentionCycle`, `hotkeyTeardown`).
   - **`defaultHotkeyBindings` map** — keep LOCAL key values. Graft upstream's NEW actions onto FREE keys; for any whose upstream default collides with a local binding, pick a free key (or leave `""` unbound) and **add a `// upstream X collides with local Y` comment**.
   - **`hotkeyActionOrder` slice** — usually auto-merges; if the tail conflicts, keep both local + upstream entries.

   **Current local key grafts (free keys chosen for upstream-new actions; keep these stable across syncs):** `worktree_setup=B` (upstream b=exec_shell), `cycle_group_view=V` (upstream t=restart), `quick_create=a` (local keeps this), `quick_approve=ctrl+a` (upstream defaults it to `a`, which is local quick_create; the user runs bypass-permissions so approve is parked off the home row — keep it bound so the upstream feature+tests survive rather than deleting and re-conflicting), `archive=A`, `unarchive=shift+u`, `view_archived=^`, `toggle_yolo=""` unbound (upstream y=teardown). **Session switcher remapped to Ctrl+W:** upstream defaults `switch_session=ctrl+s`, but Ctrl+S is XOFF / what some terminals send for Cmd+Right, so it kept opening unexpectedly. The switcher is MRU-ordered, so the fork sets `switch_session=ctrl+w`, **unbinds `mru_cycle`** (`""`) — the old blind MRU cursor-cycle it replaces — repurposes the overview `case "ctrl+w"` in `handleMainKey` to `openSessionSwitcher`, and adds `ctrl+w` to the picker's cycle-forward case (`handleSessionSwitcherKey`) for tap-to-advance. Keep the overview `case "ctrl+s"` removed. Tests pinning this: `TestResolvedSwitchByte_Default` (ctrl+w/0x17), `TestMRUCycleDefaultBinding` (mru unbound, switch=ctrl+w).

   **The dispatch switch in `home.go` (`switch key` after `normalizeMainKey`) must match the keymap.** When you rebind an action, also change its literal `case "X":` arm to the new key. `go build` catches the most common failure: a **duplicate `case "X"`** when an upstream literal arm (its default key) collides with a local arm — rebind the upstream one to the documented free key (e.g. upstream `case "b"` worktree-setup → `case "B"`; upstream `case "t"` cycle-group-view → `case "V"`). A latent local double-bind can also surface via a new upstream test (e.g. `quick_create` and `quick_approve` both on `a`: the literal `case "a"` shadowed the resolved `quick_approve` arm — resolved by keeping local `quick_create=a` and parking `quick_approve=ctrl+a`, updating the `#1369` tests to press ctrl+a).

   **Known fork features that collide with upstream — watch these areas:**
   - `y` teardown hotkey + `teardownSession()` in `internal/ui/home.go` (runs `gr; make down` directly via `zsh -ic`, tool-agnostic — not typed into the agent).
   - **Attach loop (`internal/tmux/pty.go`) is now UNIFIED on upstream's mechanism** — local's MRU/attention switch keys ride `AttachWithOptions(opts AttachOptions) (SwitchIntent, error)`: `AttachOptions` carries `MRUSwitchKey`/`AttentionSwitchKey`, the loop keeps the local pre-checks that send `switchMRU`/`switchAttention` on `detachCh` (→ `ErrMRUSwitch`/`ErrAttentionSwitch`), and upstream's `SwitchKeyByte`→`SwitchIntent` (Ctrl+S switcher) path coexists. `AttachWithOpts(AttachOpts)` is a thin compat shim. Take upstream's `attachCmd{opts,result}` struct + `Run`, and `Home.attachOptions()` (extended to include `mruSwitchByte()`/`attentionSwitchByte()`). This removes the parallel mechanism that used to conflict every sync — future conflicts here should be small; re-unify rather than re-fork if upstream reshapes it.
   - **Row render in `home.go`** — local owns the primary-label engine (`computeSessionLabels`/`primaryLabels`, dynamic precedence: name > branch > folder > broadcast > auto) + priority chip + always-on broadcast trailer (vs upstream's `show_pane_titles` toggle #1343 — keep local always-on). Graft upstream's per-row markers INTO `primaryText` (pin `📌` when `inst.Pin != PinNone`, maestro `⬢` when `isMaestro`) and ADOPT upstream's `leftGutterWidth` 2-space gutter as the Sprintf's first arg (group rows use it for root-hotkey numbers; omitting it on session rows misaligns them). Match the format verb count to the union of args.
   - **Personal-fork curated footer** (right-pads to width, shows `n/a new`) vs upstream's opt-in `[ui] footer` curated mode (#1300/#1289). The footer right-pad makes substring tests like `LastIndex("p ")` false-match inside "hel**p** " — anchor footer-order tests on label tokens (`"p settings"`, `"? help"`), not `key+" "`.
   - **Title-lock on rename** (`SetField(FieldTitle)` sets `TitleLocked`) — upstream #1355 is the canonical version; adopt upstream here.
   - **Start-path / restart session-id binding** (`ensureClaudeSessionIDFromDisk*` in `instance.go`) — adjacent to upstream #1324 (codex history scan) and #1349 (rebind guard). Re-run `TestStartPath_*` and `TestIssue1147_*` after.
   - **MAIA new-session picker** (`internal/ui/maia_worker_picker.go`, opened by `case "n"`) — Claude/Codex tool switcher (`c`/`Tab` toggles, default Claude), `Enter`=worker, `r`=ro-dev, `s`=shell, `~`=home, all honor the active tool. Replaces upstream's full NewDialog on `n` (keep upstream's `pendingRemoteName=""` reset for the shared remote path). Overlaps upstream new-session dialog nav (#1295), tool-visibility denylist (#1346), shell quick-create (#1307). When taking local's picker body, upstream's NewDialog block (uses `pathMap`/`pathInfo`) drops cleanly — those vars live only inside that block.
   - **happy-wrapper fork support** — local has RED TDD stubs (`*UseHappy*` tests) for a feature not yet implemented; these fail pre- and post-merge. Do NOT treat as merge regressions.

5. **Build verification** — `go build ./...` (broader than just `./cmd/...` — catches conflicts in any package). (Do NOT pin `GOTOOLCHAIN=go1.24.0` — go.mod tracks upstream's required version, currently ≥1.25.11; the Makefile already pins the right one. The CLAUDE.md prose may lag.) The build is your fastest correctness gate after resolving markers — it surfaces **duplicate `case "X"`** (keymap collisions), orphaned vars (e.g. `title`/`pathMap` left after taking local's side), and unused symbols. Fix them in the working tree BEFORE committing; then `gofmt -l` and `go vet ./...` clean. Do not commit a broken merge.

6. **Commit** — Use a merge commit:
   ```
   Merge upstream/main into local

   Sync upstream v1.X.Y (~N commits) into local fork branch.

   Conflict resolutions: <one bullet per file>
   <one line on pre-existing vs merge-regression failures>
   ```
   No `Co-Authored-By` line, no `--no-verify` on the **commit** (per CLAUDE.md mandate). Pre-commit runs gofmt + vet only (fast), so it passes once the build/vet/gofmt are clean. If you committed the merge before realizing a fix was needed, `git add` the fix and `git commit --amend --no-edit` (hooks re-run) rather than a second commit.

7. **Run tests (auto-run) and classify failures.** Run the mandated gates plus the full suite:
   ```
   go test -run TestPersistence_ ./internal/session/... -race -count=1
   go test ./internal/feedback/... ./internal/ui/... ./cmd/agent-deck/... -run "Feedback|Sender_" -race -count=1
   go test ./internal/watcher/... -race -count=1 -timeout 120s
   go test ./... -count=1     # broad sweep to catch merge regressions
   ```
   **macOS `-race` caveat:** the broad `-race` sweep can crash the ThreadSanitizer runtime itself (`ThreadSanitizer: CHECK failed: tsan_rtl.cpp`) — that's a tooling crash, not a data race. Run the broad sweep as `go test ./... -count=1` (no `-race`); use `-race` for the targeted gates and for any specific test you suspect.

   For EACH failing test, classify it before deciding whether it blocks the push:
   - **Pre-existing local failure** — also fails on the pre-merge commit. Check with a throwaway worktree: `git worktree add -q /tmp/ad-premerge HEAD^1 && (cd /tmp/ad-premerge && go test -run <Test> ...)`. Not a blocker.
   - **Environmental / upstream-side failure** — also fails on clean `upstream/main`: `git worktree add -q /tmp/ad-upstream upstream/main && (cd /tmp/ad-upstream && go test -run <Test> ...)`. Not a blocker. (Common: macOS `/var/folders`→`/private/var/folders` symlink tests like `GetJSONLPathChecked_*`, `*CustomCommandResumes*`; and the `*UseHappy*` local stubs.)
   - **Parallel-load flake** — fails only in the broad `go test ./...` sweep (resource contention), passes when run isolated: `go test ./internal/<pkg>/ -run <Test> -count=1`. Not a blocker. Known: `TestStartMaintenanceWorkerCallback` (2s timing), and tmux-server contention (`TestPipeManager_ConnectPreservesLiveSibling`, `TestTestMainDoesNotLeakBootstrapServer`) — multiple packages spin up tmux servers at once. **Always re-run a suspected regression isolated before calling it a blocker.**
   - **Merge regression** — passes on BOTH parents (and isolated) but fails on the merge. THIS BLOCKS. Fix it before pushing; if you can't, stop and report.
   - Clean up: `git worktree remove /tmp/ad-premerge; git worktree remove /tmp/ad-upstream; git worktree prune`. (Never use `--force` — Safety Net blocks it.)

8. **Feature summary** — Produce a summary of what landed, grouped: (a) features/fixes that touch or overlap the user's fork customizations (from the list in step 4) — call these out first and explicitly, (b) other notable user-facing features, (c) infra/CI/deps in one line. Source it from `git log --no-merges --pretty='%s' <newtag> --not <pre-merge-commit>^ | grep -iE '^(feat|fix|perf)'`.

9. **Push (auto).** The user is pre-authorized to push the `local` branch to origin (memory: `push-local-freely`). After step 7 shows no merge regressions, **`git push --no-verify origin local`** automatically. The `--no-verify` is REQUIRED here: the pre-push hook runs the full `go test -race ./...`, which ALWAYS fails on the `*UseHappy*` RED stubs (and the macOS `-race` tsan crash) — so a verified push is impossible for this fork. `--no-verify` on **push** is allowed; the CLAUDE.md mandate only forbids it on commits. You've already run the real gates manually in step 7. Report the commit count pushed and the new `origin/local` position. If there IS a merge regression, do NOT push — report and stop.

## Notes

- `main` branch may be arbitrarily stale; that is intentional. Do not try to fast-forward or sync it.
- The deprecated `ad-sync` script and its launchd job were removed 2026-05-18. Do not reference them.
- If conflicts are deep (>5 files with >10 hunks each), pause and ask the user how they want to proceed rather than guessing.
