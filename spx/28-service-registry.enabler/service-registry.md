# Service Registry

PROVIDES stable, deterministic port assignments per worktree-service pair using FNV32 hash-based allocation, backed by persistent state
SO THAT service-management and url-routing
CAN address services at predictable ports without conflicts across worktrees

## Assertions

### Scenarios

- Given a worktree-service pair, when a port is assigned, then the assigned port falls within the service's configured `port_range` ([test](tests/registry_scenario_l1_test.go))
- Given the same worktree-service pair is assigned twice without releasing, when the second assignment is requested, then the same port is returned ([test](tests/registry_scenario_l1_test.go))
- Given a port assignment exists, when `Release` is called for that pair, then `GetPort` returns 0 for that pair ([test](tests/registry_scenario_l1_test.go))
- Given a released assignment, when a new assignment is requested for the same pair, then a port within the configured range is returned ([test](tests/registry_scenario_l1_test.go))

### Properties

- Port assignment is idempotent: multiple `AssignPort` calls for the same worktree-service pair always return the same port ([test](tests/registry_property_l1_test.go))
- Different worktree-service pairs receive distinct ports within their respective service port ranges ([test](tests/registry_property_l1_test.go))
- All assigned ports satisfy `port_range.min ≤ port ≤ port_range.max` for the corresponding service ([test](tests/registry_property_l1_test.go))

### Compliance

- ALWAYS: port assignments are persisted to disk — assignments survive a process restart and are restored on next load ([test](tests/registry_compliance_l1_test.go))
