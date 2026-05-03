# URL Routing

PROVIDES subdomain-based HTTP(S) reverse proxy routing from `{branch}.localhost:{proxy_port}` to worktree service ports
SO THAT developers
CAN access services by branch name without knowing port numbers

## Assertions

### Scenarios

- Given a service running on `localhost:{port}`, when a request arrives at the proxy on `{proxy_port}`, then the proxy forwards the request and returns the upstream response ([test](tests/routing_scenario_l1_test.go))
- Given an HTTPS proxy with a trusted TLS certificate, when an HTTPS request arrives, then the connection is established without a TLS error ([test](tests/routing_scenario_l1_test.go))
- Given a running proxy, when `portree proxy stop` is called, then all listeners stop within the 5-second shutdown timeout ([test](tests/routing_scenario_l1_test.go))

### Properties

- The proxy scheme is consistent within a session: all traffic under the same `portree proxy` invocation uses either HTTP or HTTPS, never a mix ([test](tests/routing_property_l1_test.go))

### Compliance

- NEVER: the proxy sets a `WriteTimeout` on HTTP listeners — SSE and chunked streaming (e.g., Vite HMR, webpack dev server) require unlimited write deadlines ([test](tests/routing_compliance_l1_test.go))
