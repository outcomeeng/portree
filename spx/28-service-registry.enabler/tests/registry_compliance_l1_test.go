package serviceregistry_test

import (
	"testing"

	"github.com/fairy-pitta/portree/internal/config"
	"github.com/fairy-pitta/portree/internal/port"
	"github.com/fairy-pitta/portree/internal/state"
)

// TestPortAssignmentsSurviveRestart verifies that port assignments are persisted
// to disk and survive a Registry restart (simulated by creating a new Registry
// instance backed by the same state directory).
func TestPortAssignmentsSurviveRestart(t *testing.T) {
	dir := t.TempDir()
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

	// First registry instance: assign a port.
	store1, err := state.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reg1 := port.NewRegistry(store1, cfg)
	assigned, err := reg1.AssignPort("main", "web")
	if err != nil {
		t.Fatalf("first registry AssignPort() error: %v", err)
	}

	// Second registry instance backed by the same directory: simulates process restart.
	store2, err := state.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reg2 := port.NewRegistry(store2, cfg)
	restored, err := reg2.GetPort("main", "web")
	if err != nil {
		t.Fatalf("second registry GetPort() error: %v", err)
	}

	if restored != assigned {
		t.Errorf("port assignment did not survive restart: assigned=%d, restored=%d", assigned, restored)
	}
}
