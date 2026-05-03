// Package status assembles per-worktree health reports for the `portree
// status` command and the TUI dashboard. It builds structured snapshots from
// existing state — service runtime, allocated ports, proxy state — and
// optionally probes both the direct (`localhost:port`) and proxy
// (`{slug}.localhost:{proxy_port}`) URLs to populate reachability flags.
//
// The package is consumed by:
//   - cmd/status.go (CLI; both human-readable and JSON output)
//   - internal/tui (dashboard; URL column with reachability indicator)
package status

import (
	"sort"

	"github.com/fairy-pitta/portree/internal/config"
	"github.com/fairy-pitta/portree/internal/git"
	"github.com/fairy-pitta/portree/internal/process"
	"github.com/fairy-pitta/portree/internal/state"
)

// WorktreeStatus is the top-level shape of a per-worktree status report.
// Designed for direct JSON marshaling.
type WorktreeStatus struct {
	Worktree string          `json:"worktree"`
	Slug     string          `json:"slug"`
	Services []ServiceStatus `json:"services"`
	Proxy    ProxyStatus     `json:"proxy"`
}

// ServiceStatus reports a single service's runtime and reachability.
//
// Reachability has two independent dimensions:
//
//   - direct: a TCP connection (or HEAD request) to localhost:port — succeeds
//     iff the service itself is responding regardless of proxy state.
//   - proxy: a HEAD request to the proxy URL — succeeds iff the proxy is
//     running AND can reach the service. A 502 from the proxy means proxy is
//     alive but upstream isn't.
type ServiceStatus struct {
	Name            string `json:"name"`
	Port            int    `json:"port"`
	Status          string `json:"status"`
	PID             int    `json:"pid,omitempty"`
	DirectURL       string `json:"direct_url,omitempty"`
	DirectReachable bool   `json:"direct_reachable"`
	ProxyURL        string `json:"proxy_url,omitempty"`
	ProxyReachable  bool   `json:"proxy_reachable"`
}

// ProxyStatus reports the shared reverse proxy's state and health.
//
// Healthy is the result of a probe to one of the proxy ports — it reflects
// whether the listener is accepting connections, not whether any specific
// upstream is reachable (that lives on each ServiceStatus.ProxyReachable).
type ProxyStatus struct {
	Running bool  `json:"running"`
	PID     int   `json:"pid,omitempty"`
	Ports   []int `json:"ports,omitempty"`
	HTTPS   bool  `json:"https"`
	Healthy bool  `json:"healthy"`
}

// Build assembles a WorktreeStatus per non-bare worktree from the loaded
// state and config. Reachability fields are left at their zero value
// (unreachable); call Probe to populate them.
func Build(trees []git.Worktree, cfg *config.Config, st *state.State) []WorktreeStatus {
	if cfg == nil {
		return nil
	}

	serviceNames := make([]string, 0, len(cfg.Services))
	for name := range cfg.Services {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)

	proxyRunning := false
	proxyHTTPS := false
	if st != nil {
		proxyRunning = st.Proxy.Status == state.StatusRunning && st.Proxy.PID > 0 && process.IsProcessRunning(st.Proxy.PID)
		proxyHTTPS = st.Proxy.HTTPS
	}
	scheme := "http"
	if proxyHTTPS {
		scheme = "https"
	}

	// Collect proxy ports from the config, deduped and sorted.
	proxyPortSet := map[int]bool{}
	for _, svc := range cfg.Services {
		if svc.ProxyPort > 0 {
			proxyPortSet[svc.ProxyPort] = true
		}
	}
	proxyPorts := make([]int, 0, len(proxyPortSet))
	for p := range proxyPortSet {
		proxyPorts = append(proxyPorts, p)
	}
	sort.Ints(proxyPorts)

	proxyStatus := ProxyStatus{
		Running: proxyRunning,
		HTTPS:   proxyHTTPS,
		Ports:   proxyPorts,
	}
	if proxyRunning && st != nil {
		proxyStatus.PID = st.Proxy.PID
	}

	out := make([]WorktreeStatus, 0, len(trees))
	for _, tree := range trees {
		if tree.IsBare {
			continue
		}
		ws := WorktreeStatus{
			Worktree: tree.Branch,
			Slug:     tree.Slug(),
			Proxy:    proxyStatus,
		}
		for _, svcName := range serviceNames {
			svc := ServiceStatus{
				Name:   svcName,
				Status: state.StatusStopped,
			}
			if st != nil {
				if ss := state.GetServiceState(st, tree.Branch, svcName); ss != nil {
					svc.Port = ss.Port
					svc.PID = ss.PID
					if ss.PID > 0 && process.IsProcessRunning(ss.PID) {
						svc.Status = state.StatusRunning
					}
				}
			}
			if svc.Port > 0 {
				svc.DirectURL = directURL(svc.Port)
			}
			if proxyRunning {
				if cfgSvc, ok := cfg.Services[svcName]; ok {
					svc.ProxyURL = proxyURLFor(scheme, ws.Slug, cfgSvc.ProxyPort)
				}
			}
			ws.Services = append(ws.Services, svc)
		}
		out = append(out, ws)
	}
	return out
}

func directURL(port int) string {
	return formatURL("http", "localhost", port)
}

func proxyURLFor(scheme, slug string, port int) string {
	return formatURL(scheme, slug+".localhost", port)
}
