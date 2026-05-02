# Service Management

PROVIDES per-worktree service lifecycle control (start, stop, state persistence, and environment injection)
SO THAT url-routing and controls
CAN route to running services and present accurate service health

## Assertions

### Scenarios

- Given a configured project, when `StartServices` is called for a worktree, then each service process starts with `$PORT` set to its registry-assigned port ([test](tests/lifecycle_scenario_l1_test.go))
- Given a service with a per-worktree command override, when started in that worktree, then the override command is used instead of the default ([test](tests/lifecycle_scenario_l1_test.go))
- Given a service with global and worktree-specific env vars, when started, then the merged env exposes both, with worktree values overriding global values for the same key ([test](tests/lifecycle_scenario_l1_test.go))
- Given running services, when `StopServices` is called, then all service processes stop and state records are cleared ([test](tests/lifecycle_scenario_l1_test.go))

### Compliance

- ALWAYS: service processes receive `$PORT`, `$PT_BRANCH`, and `$PT_{SERVICE_NAME}_URL` for each configured service — required for cross-service URL construction ([test](tests/lifecycle_compliance_l1_test.go))
- ALWAYS: shutdown sends SIGTERM before SIGKILL, allowing services to flush state before forced termination ([test](tests/lifecycle_compliance_l2_test.go))
