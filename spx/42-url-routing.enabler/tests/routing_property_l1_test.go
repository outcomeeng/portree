package urlrouting_test

import (
	"crypto/tls"
	"testing"

	"github.com/fairy-pitta/portree/internal/config"
	"github.com/fairy-pitta/portree/internal/proxy"
	"github.com/fairy-pitta/portree/internal/state"
)

// TestSchemeIsConsistentWithinSession verifies that Scheme() returns the same
// value throughout the lifetime of a ProxyServer — either always "http" or
// always "https", never switching mid-session. The property holds for both
// the HTTP server (constructed with a nil TLS config) and the HTTPS server
// (constructed with a non-nil TLS config).
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

	cases := []struct {
		name      string
		tlsConfig *tls.Config
		want      string
	}{
		{"HTTP", nil, "http"},
		{"HTTPS", &tls.Config{}, "https"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := proxy.NewProxyServer(resolver, tc.tlsConfig)
			first := srv.Scheme()
			for i := 0; i < 10; i++ {
				if got := srv.Scheme(); got != first {
					t.Errorf("%s proxy Scheme() changed from %q to %q at call %d", tc.name, first, got, i)
				}
			}
			if first != tc.want {
				t.Errorf("%s proxy Scheme() = %q, want %q", tc.name, first, tc.want)
			}
		})
	}
}
