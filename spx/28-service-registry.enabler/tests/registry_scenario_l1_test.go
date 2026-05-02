package serviceregistry_test

import (
	"testing"

	"github.com/fairy-pitta/portree/internal/config"
	"github.com/fairy-pitta/portree/internal/port"
	"github.com/fairy-pitta/portree/internal/state"
)

func newTestRegistry(t *testing.T) *port.Registry {
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
		},
		Env:       map[string]string{},
		Worktrees: map[string]config.WTOverride{},
	}
	return port.NewRegistry(store, cfg)
}

func TestAssignPortInRange(t *testing.T) {
	reg := newTestRegistry(t)
	p, err := reg.AssignPort("main", "web")
	if err != nil {
		t.Fatalf("AssignPort() error: %v", err)
	}
	if p < 3100 || p > 3199 {
		t.Errorf("AssignPort() = %d, not in configured range [3100, 3199]", p)
	}
}

func TestAssignPortIsIdempotent(t *testing.T) {
	reg := newTestRegistry(t)
	first, err := reg.AssignPort("main", "web")
	if err != nil {
		t.Fatal(err)
	}
	second, err := reg.AssignPort("main", "web")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Errorf("AssignPort() not idempotent: first call returned %d, second returned %d", first, second)
	}
}

func TestReleaseClears(t *testing.T) {
	reg := newTestRegistry(t)
	if _, err := reg.AssignPort("main", "web"); err != nil {
		t.Fatal(err)
	}
	if err := reg.Release("main", "web"); err != nil {
		t.Fatalf("Release() error: %v", err)
	}
	p, err := reg.GetPort("main", "web")
	if err != nil {
		t.Fatal(err)
	}
	if p != 0 {
		t.Errorf("GetPort() after Release() = %d, want 0", p)
	}
}

func TestReassignAfterRelease(t *testing.T) {
	reg := newTestRegistry(t)
	if _, err := reg.AssignPort("main", "web"); err != nil {
		t.Fatal(err)
	}
	if err := reg.Release("main", "web"); err != nil {
		t.Fatal(err)
	}
	p, err := reg.AssignPort("main", "web")
	if err != nil {
		t.Fatalf("AssignPort() after Release() error: %v", err)
	}
	if p < 3100 || p > 3199 {
		t.Errorf("AssignPort() after Release() = %d, not in range [3100, 3199]", p)
	}
}
