---
name: resolve-rebase
description: Resolve merge conflicts during rebase of local branch onto upstream main
---

# Resolve Rebase Conflicts

You are resolving merge conflicts that occurred while rebasing the `local` branch onto `main`. The `local` branch contains a small number of custom feature commits (hotkeys, UI additions, new methods) applied on top of the upstream project.

## Context

- The `local` branch has custom patches: hard restart (H key), MRU session cycling (Ctrl+W), and possibly other small features
- Upstream (`main`) is the actively developed original project
- Conflicts typically happen because upstream modified the same files our patches touch (hotkeys.go, home.go, help.go, instance.go)
- Our changes are usually **additive** — new constants, new methods, new case blocks, new help entries

## Steps

1. Run `git status` to identify conflicting files
2. For each conflicting file, read the conflict markers and assess:
   - **Simple/additive**: Our code adds new entries (hotkey constants, case handlers, methods) alongside upstream's changes. Resolution: keep both sides, adapting our additions to match any upstream renames or restructuring.
   - **Overlapping**: Both sides modified the same lines of code. Resolution depends on whether our change is still valid against the new upstream code.
   - **Complex**: Upstream fundamentally restructured the area our patch touches, making mechanical resolution risky.

3. For simple/additive conflicts:
   - Resolve by keeping upstream's version of shared code and re-adding our custom additions
   - Watch for upstream renames (e.g. `hotkeyToggleGeminiYolo` → `hotkeyToggleYolo`) and update our code to match
   - Match upstream's formatting (alignment, spacing)
   - After resolving each file, run `git add <file>`

4. After all files are resolved:
   - Run `git rebase --continue` (this will use the existing commit message)
   - If more conflicts appear (multiple commits), repeat from step 1
   - Run `make build` to verify everything compiles
   - If build fails, fix the issue and amend the commit

5. If you encounter a **complex** conflict that you're not confident resolving:
   - Run `git rebase --abort`
   - Print `ESCALATE: <description of the conflict>` as the final output
   - Do NOT attempt a guess — it's better to escalate than to break the build

## Rules

- NEVER delete our custom code during resolution — the whole point is to preserve our patches
- ALWAYS preserve upstream's changes to shared/existing code
- If upstream removed something our code depends on, that's a complex conflict — escalate
- After resolution, the build MUST pass (`make build`)
- Do not create new commits — use `git rebase --continue`
