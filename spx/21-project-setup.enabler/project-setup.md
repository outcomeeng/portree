# Project Setup

PROVIDES the service topology for a project (service commands, port ranges, env vars, per-worktree overrides parsed from `.portree.toml`)
SO THAT service-registry, service-management, url-routing, and controls
CAN operate with consistent, validated project configuration across all worktrees

## Assertions

### Scenarios

- Given a valid `.portree.toml` in the repo root, when config is loaded, then all services, port ranges, env vars, and worktree overrides are available ([test](tests/config_scenario_l1_test.go))
- Given `.portree.toml` is absent, when config is loaded, then an error names the missing file and instructs the user to run `portree init` ([test](tests/config_scenario_l1_test.go))
- Given no `.portree.toml` exists in the repo, when `portree init` is run, then a `.portree.toml` with the default template is created ([test](tests/init_scenario_l1_test.go))
- Given `.portree.toml` already exists, when `portree init` is run, then the existing file is preserved and a non-zero exit status is returned ([test](tests/init_scenario_l1_test.go))

### Mappings

- Missing or empty `command` for a service maps to a validation error naming that service ([test](tests/config_mapping_l1_test.go))
- `port_range.min > port_range.max` maps to a validation error ([test](tests/config_mapping_l1_test.go))
- `port_range.min` or `port_range.max` of 0 maps to a validation error ([test](tests/config_mapping_l1_test.go))
- `proxy_port` of 0 maps to a validation error ([test](tests/config_mapping_l1_test.go))
- Two services sharing the same `proxy_port` maps to a validation error naming both services ([test](tests/config_mapping_l1_test.go))
- A worktree override referencing an undefined service name maps to a validation error ([test](tests/config_mapping_l1_test.go))
- A worktree override specifying a fixed port outside the service's `port_range` maps to a validation error ([test](tests/config_mapping_l1_test.go))
- Overlapping `port_range` bounds between two services maps to a validation error naming both services ([test](tests/config_mapping_l1_test.go))

### Properties

- Config loading is deterministic: the same `.portree.toml` content always produces the same `Config` value ([test](tests/config_property_l1_test.go))
- Env merging is deterministic: worktree service env vars override global env vars for the same key, with no ordering dependence ([test](tests/config_property_l1_test.go))
