# Known Issues — service-registry

## IPv4-only `IsPortAvailable` masks IPv6-only listeners

`internal/process/runner.go`'s `IsPortAvailable(port)` binds `127.0.0.1:port` and immediately releases. A listener bound only on IPv6 (`[::]:port` with `IPV6_V6ONLY=true`) leaves `127.0.0.1:port` free, so the check returns `true` and portree spawns a service that then fails to bind with `EADDRINUSE :::port`.

The v0.3.4 idempotency guard (`StartServices` skips already-running services) masks this for the common case — most users will never hit it because portree won't try to spawn a duplicate. The bug bites when state has been wiped (e.g., `.portree/state.json` deleted, or a stale state.json persists from before a process crash) while a real listener still holds the port.

**Fix sketch:** probe both stacks. Bind `[::]:port` (IPv6 wildcard, dual-stack on systems where `IPV6_V6ONLY=false` defaults) AND `0.0.0.0:port`. If either fails, the port is in use. Or use `lsof -ti :port` parity with `cmd/reset.go`.

**Test reproduction:** spawn a Python process that does `socket.socket(AF_INET6, SOCK_STREAM)` with `IPV6_V6ONLY=1`, bind it to a service's allocated port, then run `portree up`. The wrapper succeeds; the service crashes on `EADDRINUSE`.

Defer until a real user hits the wiped-state scenario in v0.3.4+ or until automated coverage is wanted.
