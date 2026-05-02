# AGENTS.md

This file provides guidance to [Codex](https://openai.com/codex) and [Claude Code](https://claude.ai/code) when working with code in this repository.

## Commands

```bash
make build          # build ./portree binary (injects version/commit/date via ldflags)
make test           # go test ./... -race -count=1 -v
make test-short     # go test ./... -short -count=1 (what pre-commit runs)
make lint           # golangci-lint run ./...
make fmt            # gofmt + goimports
make vet            # go vet ./...
make all            # fmt vet lint test build

# Single test
go test ./cmd/... -run TestUpDownCommand -race -count=1 -v
go test ./internal/port/... -run TestAllocate -race -count=1 -v

# Activate git hooks (runs vet + lint + test-short on commit)
make setup-hooks
```

## Architecture

portree is a single-binary CLI (Cobra) that manages dev-server processes across git worktrees and routes them via a subdomain reverse proxy.

```
main.go → cmd.Execute()
cmd/root.go   PersistentPreRunE: detects repo root (git.FindRepoRoot), loads config.Load()
               sets package-level globals: repoRoot, cfg
cmd/<sub>.go  each subcommand; annotate Annotations["skipRepoDetection"]="true" to bypass PersistentPreRunE
```

**Cobra exit-code contract** — `SilenceErrors: true` is set on rootCmd. Sub-commands return a non-nil error from `RunE` on failure. `main.go` calls `os.Exit(1)` when `cmd.Execute()` returns an error. Commands that call `return nil` on failure exit 0 — indistinguishable from success.

**Port allocation** — `internal/port/allocator.go` derives ports via `FNV32(branch:service) % range`. Collisions fall back to linear probing within the range. Assignments are persisted in `.portree/state.json` so ports are stable across restarts.

**State** — `.portree/state.json` (relative to the main worktree root). File-level locking via `internal/state/store.go` (`flock`). Holds per-service `{port, pid, status}`, proxy PIDs, and port assignments.

**Process management** — `internal/process/runner.go` starts each service as `sh -c <command>` with a process group. Graceful shutdown: SIGTERM → SIGKILL. Logs: `.portree/logs/<branch-slug>.<service>.log`.

**Proxy** — `internal/proxy/server.go` runs one HTTP listener per `proxy_port`. `internal/proxy/resolver.go` extracts the subdomain slug from the `Host` header and maps it to the allocated port.

**TUI** — `internal/tui/` is a Bubble Tea application. `app.go` is the top-level model; `dashboard.go` renders the service table; `keys.go` defines key bindings.

## Testing conventions

Tests in `cmd/cmd_test.go` share process-level state (Cobra globals + `os.Chdir`). Use `resetRootCmd()` before each test and **do not call `t.Parallel()`** in tests that use `setupGitRepo`/`setupTestRepo` — those call `os.Chdir` which is process-wide.

Test level is encoded in the filename (spec-tree convention):

| Level | Pattern                           |
| ----- | --------------------------------- |
| L1    | `{subject}_{evidence}_l1_test.go` |
| L2    | `{subject}_{evidence}_l2_test.go` |
| L3    | `{subject}_{evidence}_l3_test.go` |

L2 tests that exercise the compiled binary build it in `TestMain` by walking up from the test file to `go.mod` then calling `go build`. See `spx/CLAUDE.md` for the scaffold.

## Spec tree (spx/)

Product specs live in `spx/`. Before any spec-tree work, read `spx/CLAUDE.md` — it documents the skill invocation sequence, node naming rules, sparse-integer ordering, and assertion-test linking contract.
