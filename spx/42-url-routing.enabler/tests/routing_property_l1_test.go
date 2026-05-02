package urlrouting_test

import (
	"testing"

	"github.com/fairy-pitta/portree/internal/config"
	"github.com/fairy-pitta/portree/internal/proxy"
	"github.com/fairy-pitta/portree/internal/state"
)

// TestSchemeIsConsistentWithinSession verifies that Scheme() returns the same
// value throughout the lifetime of a ProxyServer — either always "http" or
// always "https", never switching mid-session.
func TestSchemeIsConsistentWithinSession(t *testing.T) {
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

	resolver := proxy.NewResolver(cfg, store)

	// HTTP proxy: scheme must be "http" consistently.
	httpSrv := proxy.NewProxyServer(resolver, nil)
	first := httpSrv.Scheme()
	for i := 0; i < 10; i++ {
		if got := httpSrv.Scheme(); got != first {
			t.Errorf("HTTP proxy Scheme() changed from %q to %q at call %d", first, got, i)
		}
	}
	if first != "http" {
		t.Errorf("HTTP proxy Scheme() = %q, want 'http'", first)
	}
}
