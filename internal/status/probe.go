package status

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// DefaultProbeTimeout is the per-probe deadline used by Probe when the caller
// passes a zero or negative timeout.
const DefaultProbeTimeout = 500 * time.Millisecond

// Probe issues parallel reachability checks against every ServiceStatus.
// Direct URLs are checked via TCP dial to 127.0.0.1:port; proxy URLs are
// checked via HTTP HEAD with a short timeout. The proxy block's Healthy
// field is populated from a TCP probe to the first proxy port.
//
// A direct probe returns true iff a TCP connection succeeds, regardless of
// any HTTP response. A proxy probe returns true iff the HEAD request returns
// any status code below 500 — anything 5xx (502 Bad Gateway from the proxy
// when the upstream service is dead, 504 Gateway Timeout, etc.) leaves the
// service marked unreachable via the proxy.
//
// Probe mutates the supplied slice in place. A timeout of zero or less is
// replaced with DefaultProbeTimeout.
func Probe(statuses []WorktreeStatus, timeout time.Duration) {
	if timeout <= 0 {
		timeout = DefaultProbeTimeout
	}

	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			// Probes target dev servers on `*.localhost` addresses with
			// auto-generated self-signed certs (per ADR-003). The probe is
			// loopback-only and never reaches the network — InsecureSkipVerify
			// is intentional. //nolint:gosec // G402: dev-server probe over loopback
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	var wg sync.WaitGroup
	for i := range statuses {
		ws := &statuses[i]
		// Per-worktree probe of proxy health (only once across services).
		if ws.Proxy.Running && len(ws.Proxy.Ports) > 0 {
			i := i
			port := ws.Proxy.Ports[0]
			wg.Add(1)
			go func() {
				defer wg.Done()
				statuses[i].Proxy.Healthy = probeTCP(port, timeout)
			}()
		}
		for j := range ws.Services {
			svc := &ws.Services[j]
			if svc.DirectURL != "" {
				i, j := i, j
				port := svc.Port
				wg.Add(1)
				go func() {
					defer wg.Done()
					statuses[i].Services[j].DirectReachable = probeTCP(port, timeout)
				}()
			}
			if svc.ProxyURL != "" {
				i, j := i, j
				url := svc.ProxyURL
				wg.Add(1)
				go func() {
					defer wg.Done()
					statuses[i].Services[j].ProxyReachable = probeHTTP(httpClient, url)
				}()
			}
		}
	}
	wg.Wait()
}

// probeTCP attempts a TCP connect to 127.0.0.1:port. Returns true iff the
// connection succeeded within timeout.
func probeTCP(port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// probeHTTP issues a HEAD request and returns true iff the response status is
// below 500. 5xx (typically 502 from the proxy when upstream is dead) returns
// false — the URL isn't useful to a caller in that state.
func probeHTTP(client *http.Client, url string) bool {
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	return err == nil && resp.StatusCode < 500
}

// formatURL is shared between status.go and probe.go for URL composition.
// Pulled into probe.go to avoid an import cycle when tests import status.
func formatURL(scheme, host string, port int) string {
	return fmt.Sprintf("%s://%s:%d", scheme, host, port)
}
