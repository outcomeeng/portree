# Controls

PROVIDES CLI commands (`up`, `down`, `ls`, `open`, `proxy`, `doctor`, `init`, `dash`) and TUI dashboard
SO THAT developers
CAN manage portree services from the terminal and discover service endpoints programmatically

## Assertions

### Scenarios

- Given running services, when `portree ls --json` is run, then output is valid JSON where each entry contains `worktree`, `service`, `port`, `status`, `pid`, `url`, and `direct_url` fields ([test](tests/controls_scenario_l2_test.go))
- Given running services, when `portree ls` is run without `--json`, then output is a human-readable table with the same fields ([test](tests/controls_scenario_l2_test.go))
- Given a `.portree.toml` with a validation error, when `portree doctor` is run, then each configuration error is reported with the field name and reason ([test](tests/controls_scenario_l2_test.go))
- Given a valid `.portree.toml` and all services reachable, when `portree doctor` is run, then the command exits with status 0 ([test](tests/controls_scenario_l2_test.go))
- Given no `.portree.toml` in the repo, when any command requiring config is run, then the error message instructs the user to run `portree init` ([test](tests/controls_scenario_l2_test.go))

### Compliance

- ALWAYS: every command exits with status 0 on success and non-zero on error — required for agent and script compatibility ([test](tests/controls_compliance_l2_test.go))
