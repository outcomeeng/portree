package urlrouting_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fairy-pitta/portree/internal/config"
	"github.com/fairy-pitta/portree/internal/proxy"
	"github.com/fairy-pitta/portree/internal/state"
)

const testProxyPort = 19800

func setupProxy(t *testing.T, backendPort int) (*proxy.ProxyServer, *state.FileStore) {
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
				ProxyPort: testProxyPort,
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
	return proxy.NewProxyServer(resolver, nil), store
}

// TestProxyForwardsRequestToUpstream verifies that the proxy forwards an incoming
// HTTP request to the upstream service and returns the upstream response.
func TestProxyForwardsRequestToUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "hello from upstream")
	}))
	defer upstream.Close()

	var backendPort int
	_, _ = fmt.Sscanf(upstream.Listener.Addr().String(), "127.0.0.1:%d", &backendPort)

	srv, _ := setupProxy(t, backendPort)
	if err := srv.Start(map[string]int{"web": testProxyPort}); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer func() { _ = srv.Stop() }()
	time.Sleep(20 * time.Millisecond)

	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/", testProxyPort), nil)
	req.Host = fmt.Sprintf("main.localhost:%d", testProxyPort)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET proxy error: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("proxy response status = %d, want 200", resp.StatusCode)
	}
	if string(body) != "hello from upstream" {
		t.Errorf("proxy response body = %q, want 'hello from upstream'", body)
	}
}

// generateTestCert creates a self-signed ECDSA certificate for 127.0.0.1/localhost,
// valid for 24 hours. Used only in tests — not trusted outside the test client.
func generateTestCert(t *testing.T) (tls.Certificate, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generateTestCert: key generation: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("generateTestCert: CreateCertificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("generateTestCert: MarshalECPrivateKey: %v", err)
	}
	tlsCert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
	)
	if err != nil {
		t.Fatalf("generateTestCert: X509KeyPair: %v", err)
	}
	x509Cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("generateTestCert: ParseCertificate: %v", err)
	}
	return tlsCert, x509Cert
}

// TestHTTPSProxyEstablishesWithoutTLSError verifies that an HTTPS proxy with a
// trusted TLS certificate accepts connections without a TLS error.
func TestHTTPSProxyEstablishesWithoutTLSError(t *testing.T) {
	const httpsProxyPort = 19803

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer upstream.Close()

	var backendPort int
	_, _ = fmt.Sscanf(upstream.Listener.Addr().String(), "127.0.0.1:%d", &backendPort)

	store, err := state.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Services: map[string]config.ServiceConfig{
			"web": {
				Command:   "npm start",
				PortRange: config.PortRange{Min: 3100, Max: 3199},
				ProxyPort: httpsProxyPort,
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

	tlsCert, x509Cert := generateTestCert(t)
	proxyCfg := &tls.Config{Certificates: []tls.Certificate{tlsCert}}
	resolver := proxy.NewResolver(cfg, store)
	srv := proxy.NewProxyServer(resolver, proxyCfg)
	if err := srv.Start(map[string]int{"web": httpsProxyPort}); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer func() { _ = srv.Stop() }()
	time.Sleep(20 * time.Millisecond)

	pool := x509.NewCertPool()
	pool.AddCert(x509Cert)
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
	}

	req, _ := http.NewRequest("GET", fmt.Sprintf("https://127.0.0.1:%d/", httpsProxyPort), nil)
	req.Host = fmt.Sprintf("main.localhost:%d", httpsProxyPort)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("HTTPS proxy request error (TLS handshake failure would appear here): %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("HTTPS proxy status = %d, want 200", resp.StatusCode)
	}
}

// TestProxyStopsWithinShutdownTimeout verifies that Stop() completes within 5 seconds.
func TestProxyStopsWithinShutdownTimeout(t *testing.T) {
	srv, _ := setupProxy(t, 1) // upstream port 1 — listener binds but routing fails; enough for shutdown test
	if err := srv.Start(map[string]int{"web": 19801}); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	done := make(chan error, 1)
	go func() { done <- srv.Stop() }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Stop() error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("proxy.Stop() did not complete within the 5-second shutdown timeout")
	}
}
