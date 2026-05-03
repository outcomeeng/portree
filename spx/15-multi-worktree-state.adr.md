# Multi-Worktree Shared State

## Purpose

This decision governs where portree's runtime state and configuration live when multiple git worktrees of the same repository are active concurrently. Both reside at a single canonical location addressable identically from every worktree.

## Context

**Business impact:** portree's product hypothesis targets 2+ concurrent worktrees as the default development mode. A per-worktree state file fragments the model: `portree up` from worktree A is invisible to `portree ls` from worktree B, ports allocated by one worktree collide with services started by another, and `--all` operations produce inconsistent views depending on which worktree invoked them. Per-worktree config loading is similarly fragile: feature branches typically don't carry `.portree.toml` in their checkout, so commands invoked from a linked worktree fail with a missing-config error even when the main worktree's config is fully valid.

**Technical constraints:**

- Git exposes the calling worktree's root via `git rev-parse --show-toplevel` and the shared common directory via `git rev-parse --git-common-dir`. The latter resolves to the same path from every worktree of the same repository.
- The `internal/git` package exposes both functions: `git.FindRepoRoot` (per-worktree) and `git.MainWorktreeRoot` (shared via common-dir resolution).
- `.portree/state.json` uses `flock` for concurrent access. Concurrent invocations from sibling worktrees must read and write the same file for the lock to convey mutual exclusion.
- The `--all` flag iterates worktrees from `git worktree list --porcelain`. Iteration is correct regardless of caller; the writes converge only when the file path is identical across callers.
- The `--prune` flag removes state entries for branches whose worktrees no longer exist. Composition with `--all` requires both effects in one invocation.

## Decision

Every command resolves both the state directory (`git.MainWorktreeRoot(cwd)/.portree`) and the config file (`git.MainWorktreeRoot(cwd)/.portree.toml`) from the main worktree root regardless of which worktree invoked it, and `portree down --all --prune` runs the stop loop for every non-bare worktree before pruning orphaned state entries.

## Rationale

Three sub-rules follow from one principle: multi-worktree concurrency requires a single canonical address for state.

1. **State directory and config both resolve from `MainWorktreeRoot`, not `FindRepoRoot`.** The main worktree root is the only path identical from every worktree. `FindRepoRoot` returns the caller's worktree root, which differs per worktree and would fragment the state file. The same logic applies to config: linked worktrees rarely carry `.portree.toml` in their checkout, and per-worktree overrides are already expressed as `[worktrees.<branch>]` sections inside the single canonical config — there is no real workflow that benefits from a separate `.portree.toml` per worktree.

2. **`--all --prune` composes stop and prune.** `--prune` enhances `down` rather than replacing it. The composition `down --all --prune` runs both effects: stop services for every non-bare worktree, then remove state entries whose branches are absent from `git worktree list`. Short-circuiting on `--prune` makes the composition silently lossy and surprises the caller.

3. **Process-group termination handles child cleanup.** Service processes start with `Setpgid: true`, making each `sh -c` invocation a process-group leader. `Kill(-pgid, SIGTERM)` followed by `Kill(-pgid, SIGKILL)` after the timeout terminates the wrapper shell and every direct child. Services that re-detach by setting their own pgid are out of scope — that rare case falls to consumers.

**Alternatives rejected:**

- **Per-worktree state files with cross-worktree synchronization.** Doubles the source of truth and makes `--all` semantics dependent on the calling worktree's state. The same reason a database has one master, not N replicas with conflict resolution.
- **State under the user's home directory** (`~/.portree/state.json`). Loses per-repository isolation; multiple repositories on the same machine collide.

## Trade-offs accepted

| Trade-off                                                             | Mitigation / reasoning                                                                                                                    |
| --------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| State location depends on git's notion of the main worktree           | `git rev-parse --git-common-dir` is stable across all supported git versions                                                              |
| Stale `.portree/` directories under linked worktrees become invisible | Migration is `rm -rf <linkedWorktree>/.portree`; the directory carries no role after fix                                                  |
| `.portree.toml` files in linked worktrees are silently ignored        | Linked worktrees rarely carry the file in their checkout; per-branch overrides live in `[worktrees.<branch>]` sections of the main config |
| `down --all --prune` runs longer than `--prune` alone                 | Composition's purpose is both effects; "prune only" remains available as `down --prune`                                                   |

## Compliance

### Recognized by

Every command's state I/O and config loading construct paths through `git.MainWorktreeRoot`. No call site resolves `.portree/state.json` or `.portree.toml` from `git.FindRepoRoot` or `os.Getwd()`. The `down` command runs its stop loop before any prune step.

### MUST

- Every command resolves the state directory as `filepath.Join(git.MainWorktreeRoot(cwd), ".portree")` ([test](49-controls.enabler/tests/controls_multiworktree_l2_test.go))
- Every command loads config from `filepath.Join(git.MainWorktreeRoot(cwd), ".portree.toml")` ([test](49-controls.enabler/tests/controls_multiworktree_l2_test.go))
- `portree down --all --prune` runs the stop loop for every non-bare worktree before pruning orphaned state entries ([test](49-controls.enabler/tests/controls_multiworktree_l2_test.go))
- Service processes start with `Setpgid: true` and stop via `Kill(-pgid, SIGTERM)` followed by `Kill(-pgid, SIGKILL)` after the configured timeout ([test](35-service-management.enabler/tests/lifecycle_compliance_l2_test.go))
- `portree up` is idempotent: a service whose state record holds a live PID is not respawned, and its PID remains in state ([test](49-controls.enabler/tests/controls_multiworktree_l2_test.go))

### NEVER

- A command resolves the state directory or config path from `git.FindRepoRoot` or `os.Getwd()` — both return per-worktree paths that fragment state and break commands invoked from linked worktrees without a local config ([review])
- `--prune` short-circuits the stop loop in `down` — composition `--all --prune` performs both effects in one invocation ([review])
