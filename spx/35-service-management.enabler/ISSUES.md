# Known Issues — service-management

## Grandchild orphans escape `Kill(-pgid, ...)` termination

`internal/process/runner.go` starts each service with `Setpgid: true`, putting the `sh -c "..."` wrapper in a new process group with `pgid == wrapper PID`. `Stop()` sends `Kill(-pgid, SIGTERM)`, which is supposed to signal every process in the group.

In practice, complex command chains can produce descendants that re-set their own process group and escape the kill. The pattern observed during xiperinc integration: `sh -c "pnpm run dev:service"` → `pnpm` → `next dev` → Node.js workers. pnpm and/or Node's child-process management spawns descendants outside the wrapper's pgid, so when `Stop()` kills the wrapper group, `next-server` keeps running, holds its allocated port, and breaks subsequent `portree up` invocations with `EADDRINUSE`.

**Workaround in place:**

- `portree reset` (v0.5.0) hunts processes by *port* via `lsof` and kills them, sidestepping the pgid problem entirely. This is the user-facing escape hatch.
- `portree reset --proxy-port` (v0.5.0) extends that to the proxy port, skipping the legitimate proxy PID recorded in state.

**Proper fix would require:**

1. Tracking descendants explicitly (e.g., walking `/proc/<pid>/task/.../children` on Linux, `pgrep -P` on macOS) before `Stop()` and signalling them individually.
2. Or: invoking the user's command directly via `exec.Command(parsedCmd[0], parsedCmd[1:]...)` instead of `sh -c "<string>"`, so the runner has direct PID handles to all spawned children. Loses shell quoting/expansion features.
3. Or: wrapping `cmd.Cancel` or platform-specific `prctl(PR_SET_PDEATHSIG)` so the kernel sends SIGKILL when the parent dies.

Tradeoffs and platform compatibility need analysis. Defer until `portree reset --proxy-port` proves insufficient in real workflows.
