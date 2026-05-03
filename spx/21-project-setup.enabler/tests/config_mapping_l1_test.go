package projectsetup_test

import (
	"strings"
	"testing"

	"github.com/fairy-pitta/portree/internal/config"
)

func TestValidationMappings(t *testing.T) {
	base := func() *config.Config {
		return &config.Config{
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
	}

	cases := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr string
	}{
		{
			name:    "empty command maps to validation error naming the service",
			mutate:  func(c *config.Config) { s := c.Services["web"]; s.Command = ""; c.Services["web"] = s },
			wantErr: "web",
		},
		{
			name: "port_range.min > port_range.max maps to validation error",
			mutate: func(c *config.Config) {
				s := c.Services["web"]
				s.PortRange = config.PortRange{Min: 4000, Max: 3000}
				c.Services["web"] = s
			},
			wantErr: "must be <=",
		},
		{
			name: "port_range.min zero maps to validation error",
			mutate: func(c *config.Config) {
				s := c.Services["web"]
				s.PortRange = config.PortRange{Min: 0, Max: 3199}
				c.Services["web"] = s
			},
			wantErr: "positive",
		},
		{
			name: "port_range.max zero maps to validation error",
			mutate: func(c *config.Config) {
				s := c.Services["web"]
				s.PortRange = config.PortRange{Min: 3100, Max: 0}
				c.Services["web"] = s
			},
			wantErr: "positive",
		},
		{
			name: "proxy_port zero maps to validation error",
			mutate: func(c *config.Config) {
				s := c.Services["web"]
				s.ProxyPort = 0
				c.Services["web"] = s
			},
			wantErr: "proxy_port",
		},
		{
			name: "duplicate proxy_port maps to validation error naming both services",
			mutate: func(c *config.Config) {
				c.Services["api"] = config.ServiceConfig{
					Command:   "go run .",
					PortRange: config.PortRange{Min: 8100, Max: 8199},
					ProxyPort: 3000, // same as web
				}
			},
			wantErr: "proxy_port",
		},
		{
			name: "worktree override for unknown service maps to validation error",
			mutate: func(c *config.Config) {
				c.Worktrees["main"] = config.WTOverride{
					Services: map[string]config.WTServiceOverride{
						"nonexistent": {Port: 3100},
					},
				}
			},
			wantErr: "nonexistent",
		},
		{
			name: "worktree override port outside range maps to validation error",
			mutate: func(c *config.Config) {
				c.Worktrees["main"] = config.WTOverride{
					Services: map[string]config.WTServiceOverride{
						"web": {Port: 9999}, // outside [3100, 3199]
					},
				}
			},
			wantErr: "outside range",
		},
		{
			name: "overlapping port ranges between services maps to validation error",
			mutate: func(c *config.Config) {
				c.Services["api"] = config.ServiceConfig{
					Command:   "go run .",
					PortRange: config.PortRange{Min: 3150, Max: 3250}, // overlaps web [3100-3199]
					ProxyPort: 8000,
				}
			},
			wantErr: "overlapping",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base()
			tc.mutate(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("Validate() expected an error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate() error = %q, expected it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}
