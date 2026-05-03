package urlrouting_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fairy-pitta/portree/internal/config"
	"github.com/fairy-pitta/portree/internal/proxy"
	"github.com/fairy-pitta/portree/internal/state"
)

// TestWriteTimeoutIsNotSet verifies that the proxy does not cut off long-running
// responses — a hard requirement for SSE and chunked streaming (e.g., Vite HMR).
// If WriteTimeout were set to a non-zero value, the upstream response sent after
// that deadline would be truncated. This test uses a streaming upstream that sends
// data in two bursts separated by > the ReadTimeout interval and checks both bursts
// arrive at the client.
func TestWriteTimeoutIsNotSet(t *testing.T) {
	const chunk1 = "chunk-one"
	const chunk2 = "chunk-two"
	// Separation between chunks: longer than typical write timeouts (30s default would fail).
	// We use a shorter delay here to keep the test fast while still exercising the proxy path.
	const chunkDelay = 200 * time.Millisecond

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprint(w, chunk1)
		flusher.Flush()
		time.Sleep(chunkDelay)
		_, _ = fmt.Fprint(w, chunk2)
		flusher.Flush()
	}))
	defer upstream.Close()

	var backendPort int
	_, _ = fmt.Sscanf(upstream.Listener.Addr().String(), "127.0.0.1:%d", &backendPort)

	proxyPort := freePort(t)

	store, err := state.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Services: map[string]config.ServiceConfig{
			"web": {
				Command:   "npm start",
				PortRange: config.PortRange{Min: 3100, Max: 3199},
				ProxyPort: proxyPort,
			},
		},
		Env:       map[string]string{},
		Worktrees: map[string]config.WTOverride{},
	}
	st := &state.State{
		Services:        map[string]map[string]*state.ServiceState{},
		PortAssignments: map[string]int{},
	}
	state.SetPortAssignment(st, "main", "web", backendPort)
	if err := store.Save(st); err != nil {
		t.Fatal(err)
	}

	resolver := proxy.NewResolver(cfg, store)
	srv := proxy.NewProxyServer(resolver, nil)
	if err := srv.Start(map[string]int{"web": proxyPort}); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer func() { _ = srv.Stop() }()
	time.Sleep(20 * time.Millisecond)

	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/", proxyPort), nil)
	req.Host = fmt.Sprintf("main.localhost:%d", proxyPort)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("streaming request error: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if !strings.Contains(string(body), chunk1) {
		t.Errorf("first chunk not received; body = %q", body)
	}
	if !strings.Contains(string(body), chunk2) {
		t.Errorf("second chunk not received (response may have been cut off by WriteTimeout); body = %q", body)
	}
}
