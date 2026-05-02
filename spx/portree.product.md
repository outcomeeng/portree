# portree

## Why this product exists

portree provides automatic per-worktree dev server management — port allocation, process lifecycle, and subdomain routing — so that Outcome Engineering platform developers can switch between git worktrees and run parallel environments without managing ports, processes, or URLs by hand.

## Product hypothesis

WE BELIEVE THAT providing automatic per-worktree dev server management (port allocation, process lifecycle, subdomain routing)
WILL cause platform developers to switch between worktrees and run parallel dev environments without manual intervention
CONTRIBUTING TO faster iteration cycles on the Outcome Engineering platform (spx, plugins, outcome.engineering)

### Evidence of success

| Metric                           | Current                      | Target                     | Measurement approach                             |
| -------------------------------- | ---------------------------- | -------------------------- | ------------------------------------------------ |
| Manual port management incidents | Frequent (developer-managed) | Zero (portree-managed)     | Count port conflicts and manual overrides weekly |
| Worktree context-switch time     | Minutes (start/stop by hand) | Seconds (`portree up`)     | Benchmark `portree up` execution time            |
| Concurrent worktrees in use      | 1 (sequential by necessity)  | 2+ (concurrent by default) | Developer workflow observation                   |

## Scope

### What's included

- Project setup — service topology configuration and project initialization
- Service registry — deterministic port assignment per worktree-service pair
- Service management — per-worktree service lifecycle (start, stop, state persistence)
- URL routing — subdomain HTTP(S) reverse proxy to worktree services
- Controls — CLI commands and TUI dashboard

### What's excluded

| Excluded                  | Rationale                                                |
| ------------------------- | -------------------------------------------------------- |
| GUI / web interface       | portree is a CLI tool; integrations can add UI           |
| Third-party plugin API    | Requires stable domain interfaces first                  |
| Multi-machine setups      | Local development tool only                              |
| CI/CD environment support | Remote environments have dedicated port management tools |

## Product-level assertions

### Compliance

- ALWAYS: CLI commands exit with non-zero status on error — required for agent and script compatibility ([review])
- NEVER: portree modifies `/etc/hosts` or requires elevated privileges for core functionality — developer ergonomics ([review])

## Open decisions

| Decision topic          | Key question                                           | Options                                 | Triggers ADR/PDR? |
| ----------------------- | ------------------------------------------------------ | --------------------------------------- | ----------------- |
| State persistence scope | Should process state survive a machine restart?        | File-based persistence / in-memory only | yes               |
| HTTPS trust model       | How should the auto-generated TLS cert be distributed? | mkcert / manual trust / HTTP-only mode  | yes               |
