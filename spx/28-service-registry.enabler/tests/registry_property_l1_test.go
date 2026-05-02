package serviceregistry_test

import (
	"testing"

	"github.com/fairy-pitta/portree/internal/config"
	"github.com/fairy-pitta/portree/internal/port"
	"github.com/fairy-pitta/portree/internal/state"
)

func newTwoServiceRegistry(t *testing.T) *port.Registry {
	t.Helper()
	store, err := state.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Services: map[string]config.ServiceConfig{
			"web": {
				Command:   "npm start",
				PortRange: config.PortRange{Min: 3100, Max: 3199},
				ProxyPort: 3000,
			},
			"api": {
				Command:   "go run .",
				PortRange: config.PortRange{Min: 8100, Max: 8199},
				ProxyPort: 8000,
			},
		},
		Env:       map[string]string{},
		Worktrees: map[string]config.WTOverride{},
	}
	return port.NewRegistry(store, cfg)
}

// TestAssignPortIdempotencyProperty verifies that AssignPort returns the same port
// for the same pair across multiple calls — across the branches listed below.
func TestAssignPortIdempotencyProperty(t *testing.T) {
	branches := []string{"main", "feature/auth", "fix/bug-123", "release/v2"}
	for _, branch := range branches {
		reg := newTestRegistry(t)
		first, err := reg.AssignPort(branch, "web")
		if err != nil {
			t.Fatalf("branch %q: AssignPort error: %v", branch, err)
		}
		for i := 0; i < 5; i++ {
			repeated, err := reg.AssignPort(branch, "web")
			if err != nil {
				t.Fatalf("branch %q call %d: AssignPort error: %v", branch, i, err)
			}
			if repeated != first {
				t.Errorf("branch %q: AssignPort not idempotent: first=%d repeated[%d]=%d", branch, first, i, repeated)
			}
		}
	}
}

// TestDifferentPairsGetDistinctPorts verifies that different worktree-service pairs
// do not share ports within their respective service ranges.
func TestDifferentPairsGetDistinctPorts(t *testing.T) {
	reg := newTwoServiceRegistry(t)
	branches := []string{"main", "feature/auth", "fix/bug"}

	portsSeen := map[int]string{}
	for _, branch := range branches {
		for _, svc := range []string{"web", "api"} {
			p, err := reg.AssignPort(branch, svc)
			if err != nil {
				t.Fatalf("AssignPort(%q, %q) error: %v", branch, svc, err)
			}
			key := branch + ":" + svc
			if existing, ok := portsSeen[p]; ok {
				t.Errorf("port %d assigned to both %q and %q", p, existing, key)
			}
			portsSeen[p] = key
		}
	}
}

// TestAssignedPortsAreInRange verifies that all assigned ports satisfy
// port_range.min ≤ port ≤ port_range.max for the corresponding service.
func TestAssignedPortsAreInRange(t *testing.T) {
	ranges := map[string]config.PortRange{
		"web": {Min: 3100, Max: 3199},
		"api": {Min: 8100, Max: 8199},
	}
	reg := newTwoServiceRegistry(t)
	for svc, r := range ranges {
		p, err := reg.AssignPort("main", svc)
		if err != nil {
			t.Fatalf("AssignPort(main, %q) error: %v", svc, err)
		}
		if p < r.Min || p > r.Max {
			t.Errorf("service %q: port %d not in range [%d, %d]", svc, p, r.Min, r.Max)
		}
	}
}
