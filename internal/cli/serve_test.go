package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// lockedBuffer is a bytes.Buffer that's safe for concurrent use.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// A served is `nbpdns serve` running in the test.
type served struct {
	t      *testing.T
	addr   string
	stderr *lockedBuffer
	cancel context.CancelFunc
	done   chan int
}

// startServe runs `nbpdns serve` with env, listening on a free port of
// 127.0.0.1, and returns once it's listening.
func startServe(t *testing.T, env map[string]string) *served {
	t.Helper()
	env["NBPDNS_SERVER_LISTEN"] = "127.0.0.1:0"
	env["NBPDNS_LOG_FORMAT"] = "json"
	for k, v := range env {
		t.Setenv(k, v)
	}
	ctx, cancel := context.WithCancel(t.Context())
	s := &served{t: t, stderr: &lockedBuffer{}, cancel: cancel, done: make(chan int, 1)}
	go func() { s.done <- Main(ctx, []string{"serve"}, io.Discard, s.stderr) }()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		for l := range strings.Lines(s.stderr.String()) {
			var rec struct {
				Msg     string `json:"msg"`
				Address string `json:"address"`
			}
			if json.Unmarshal([]byte(l), &rec) == nil && rec.Msg == "serving" {
				s.addr = rec.Address
				t.Cleanup(func() { s.stop() })
				return s
			}
		}
		select {
		case code := <-s.done:
			t.Fatalf("serve exited %d before listening:\n%s", code, s.stderr)
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatalf("serve didn't listen:\n%s", s.stderr)
	return nil
}

// get fetches path from the service, and returns its status and body.
func (s *served) get(path string) (int, string) {
	s.t.Helper()
	req, err := http.NewRequestWithContext(s.t.Context(), http.MethodGet, "http://"+s.addr+path, http.NoBody)
	if err != nil {
		s.t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatal(err)
	}
	return resp.StatusCode, string(b)
}

// waitReady waits until /readyz answers 200.
func (s *served) waitReady(timeout time.Duration) {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if code, _ := s.get("/readyz"); code == http.StatusOK {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	s.t.Fatalf("serve wasn't ready after %s:\n%s", timeout, s.stderr)
}

// stop cancels serve's context, as SIGTERM does, and returns its exit code.
func (s *served) stop() int {
	s.cancel()
	select {
	case code := <-s.done:
		s.done <- code
		return code
	case <-time.After(30 * time.Second):
		s.t.Fatalf("serve didn't stop:\n%s", s.stderr)
		return -1
	}
}

func TestServeFailures(t *testing.T) {
	inUse, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer inUse.Close()
	two := configFile(t, twoGroups)
	two["NBPDNS_NETBOX_URL"], two["NBPDNS_NETBOX_TOKEN"] = "http://127.0.0.1:1", "nbt_abc.def"
	two["NBPDNS_SERVER_LISTEN"] = inUse.Addr().String()
	for _, tt := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"no groups", configFile(t, "log: {level: info}\n"), "no PowerDNS server groups are declared"},
		{"an address in use", two, "server.listen: listen tcp " + inUse.Addr().String()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := run(t, tt.env, "serve")
			if code != exitError || !strings.Contains(stderr, tt.want) {
				t.Errorf("exit %d, want %d with %q:\n%s", code, exitError, tt.want, stderr)
			}
		})
	}
}

// TestServeWithoutNetBox runs serve with NetBox unreachable: its first
// refresh fails, which still makes it ready, and it stops cleanly.
func TestServeWithoutNetBox(t *testing.T) {
	env := configFile(t, twoGroups)
	env["NBPDNS_NETBOX_URL"], env["NBPDNS_NETBOX_TOKEN"] = "http://127.0.0.1:1", "nbt_abc.s3cret-token"
	// Spans go nowhere, but the headers must still not show.
	env["NBPDNS_OTLP_ENDPOINT"], env["NBPDNS_OTLP_HEADERS"], env["NBPDNS_OTLP_TIMEOUT"] = "http://127.0.0.1:1", "Authorization=s3cret-header", "1s"
	s := startServe(t, env)
	if code, body := s.get("/livez"); code != http.StatusOK || body != "ok\n" {
		t.Errorf("livez: %d %q", code, body)
	}
	s.waitReady(time.Minute)
	code, body := s.get("/metrics")
	for _, want := range []string{
		"nbpdns_netbox_up 0",
		`nbpdns_drift_refreshes_total{outcome="failed"} 1`,
		`nbpdns_http_client_requests_total{code="error",method="GET",service="NetBox",target="netbox"}`,
		"nbpdns_build_info{",
	} {
		if code != http.StatusOK || !strings.Contains(body, want) {
			t.Errorf("metrics have no %q:\n%s", want, body)
		}
	}
	for _, path := range []string{"/status", "/status?json=1"} {
		code, page := s.get(path)
		if code != http.StatusOK || !strings.Contains(page, "127.0.0.1:1") || strings.Contains(page, "s3cret") {
			t.Errorf("%s: %d, without the URLs or with a secret:\n%s", path, code, page)
		}
	}
	if code := s.stop(); code != exitOK {
		t.Errorf("exit %d, want 0:\n%s", code, s.stderr)
	}
	for _, want := range []string{"couldn't read NetBox", `"msg":"shutting down"`} {
		if !strings.Contains(s.stderr.String(), want) {
			t.Errorf("no %q in the logs:\n%s", want, s.stderr)
		}
	}
}
