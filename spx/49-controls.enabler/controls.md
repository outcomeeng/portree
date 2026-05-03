# Controls

PROVIDES CLI commands (`up`, `down`, `ls`, `open`, `proxy`, `doctor`, `init`, `reset`, `dash`) and TUI dashboard
SO THAT developers
CAN manage portree services from the terminal and discover service endpoints programmatically

## Assertions

### Scenarios

- Given running services, when `portree ls --json` is run, then output is valid JSON where each entry contains `worktree`, `service`, `port`, `status`, `pid`, `url`, and `direct_url` fields ([test](tests/controls_scenario_l2_test.go))
- Given running services, when `portree ls` is run without `--json`, then output is a human-readable table with the same fields ([test](tests/controls_scenario_l2_test.go))
- Given a `.portree.toml` with a validation error, when `portree doctor` is run, then each configuration error is reported with the field name and reason ([test](tests/controls_scenario_l2_test.go))
- Given a valid `.portree.toml` and all services reachable, when `portree doctor` is run, then the command exits with status 0 ([test](tests/controls_scenario_l2_test.go))
- Given no `.portree.toml` in the repo, when any command requiring config is run, then the error message instructs the user to run `portree init` ([test](tests/controls_scenario_l2_test.go))
- Given multiple worktrees exist, when `portree up` runs (without `--all`) from any worktree, then only that worktree's services start; services in other worktrees are untouched ([test](tests/controls_multiworktree_l2_test.go))
- Given multiple worktrees exist, when `portree up --all` runs from any worktree, then services start for every non-bare worktree ([test](tests/controls_flags_l2_test.go))
- Given services are already running for a worktree, when `portree up` runs again from any worktree, then those services are not respawned, the original PIDs remain in state, and the existing processes are untouched ([test](tests/controls_multiworktree_l2_test.go))
- Given a configured service name, when `portree up --service <name>` runs, then only that service starts ([test](tests/controls_flags_l2_test.go))
- Given orphaned state entries for removed worktrees, when `portree down --prune` runs, then those entries are removed and the command exits 0 ([test](tests/controls_flags_l2_test.go))
- Given stale state entries (status running, recorded PID no longer alive), when `portree down --prune` runs, then those entries are marked stopped without disturbing any actually-running services ([test](tests/controls_multiworktree_l2_test.go))
- Given stale state entries detected by `portree doctor`, when the diagnostic output is rendered, then the stale-state row names `portree down --prune` as the command that clears the entries ([test](tests/controls_multiworktree_l2_test.go))
- Given running services across all worktrees and orphaned state entries, when `portree down --all --prune` runs from any worktree, then every worktree's services stop and orphaned entries are removed in a single invocation ([test](tests/controls_multiworktree_l2_test.go))
- Given the proxy is running and reachable, when `portree ls` is run, then each entry surfaces its proxy URL (`http://{slug}.localhost:{proxy_port}`) before the port column with a reachability indicator ([test](tests/controls_multiworktree_l2_test.go))
- Given a process holds a port allocated to one of the current worktree's services, when `portree reset` runs, then that process is terminated and the port is freed ([test](tests/controls_multiworktree_l2_test.go))
- Given a process holds the configured proxy port and no legitimate proxy is recorded in state, when `portree reset --proxy-port` runs, then that process is terminated and the port is freed ([test](tests/controls_multiworktree_l2_test.go))
- Given the proxy is running and recorded in state, when `portree reset --proxy-port` runs, then the legitimate proxy PID is preserved and only unrelated listeners are killed ([test](tests/controls_multiworktree_l2_test.go))
- Given the proxy is not running, when `portree up --ensure-proxy` runs, then the proxy starts in the background, registers in state, and accepts connections on its configured port ([test](tests/controls_multiworktree_l2_test.go))
- Given the proxy is already running, when `portree up --ensure-proxy` runs, then the existing proxy is left alone (same PID in state) ([test](tests/controls_multiworktree_l2_test.go))
- Given no other worktree has running services, when `portree down --release-proxy` runs, then the proxy is stopped and its port is freed ([test](tests/controls_multiworktree_l2_test.go))
- Given at least one other worktree has running services, when `portree down --release-proxy` runs, then the proxy is left alone and its PID is unchanged ([test](tests/controls_multiworktree_l2_test.go))
- Given a `.portree.toml` exists in a linked worktree's checkout, when any portree command runs from that worktree, then the file is ignored and the main worktree's config is used ([test](tests/controls_multiworktree_l2_test.go))

### Compliance

- ALWAYS: every command exits with status 0 on success and non-zero on error — required for agent and script compatibility ([test](tests/controls_compliance_l2_test.go))
- ALWAYS: every command resolves the state directory as `filepath.Join(git.MainWorktreeRoot(cwd), ".portree")` so all worktrees read and write a single shared state file ([test](tests/controls_multiworktree_l2_test.go))
- ALWAYS: `portree down --all --prune` runs the stop loop for every non-bare worktree before pruning orphaned state — `--prune` does not short-circuit the stop loop ([test](tests/controls_multiworktree_l2_test.go))
