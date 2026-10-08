package netbox

// These tests run the client against local HTTP servers that send only
// status codes, headers and delays, and, where a body is needed, responses
// recorded from the lab (ADR-0023). NetBox's own behavior is tested against
// the lab, in netbox_integration_test.go.

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/httpclient"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/tracing"
)

// recorded returns a response recorded from the lab's NetBox 4.7.
func recorded(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "netbox-47", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A step is one response of a scripted server.
type step struct {
	status int
	header map[string]string
	delay  time.Duration
}

// scripted returns a server that answers its nth request with steps[n], or
// with the last step once they run out. A 200 carries the recorded status
// response. It counts the requests it gets.
func scripted(t *testing.T, steps ...step) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	body := recorded(t, "status.json")
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := steps[min(int(n.Add(1)), len(steps))-1]
		select {
		case <-time.After(s.delay):
		case <-r.Context().Done():
			return
		}
		for k, v := range s.header {
			w.Header().Set(k, v)
		}
		w.WriteHeader(s.status)
		if s.status == http.StatusOK {
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

// testClient returns a client for url with o's settings, whose retries
// record their delays instead of sleeping.
func testClient(t *testing.T, url string, o Options) (*Client, *[]time.Duration) {
	t.Helper()
	var delays []time.Duration
	o.URL = url
	if !o.Token.IsSet() {
		o.Token = config.NewSecret("nbt_testkey.testsecret")
	}
	if o.Timeout == 0 {
		o.Timeout = 5 * time.Second
	}
	o.retry = httpclient.Retry{Attempts: 4, Base: 100 * time.Millisecond, Cap: time.Second,
		Sleep: func(_ context.Context, d time.Duration) error { delays = append(delays, d); return nil }}
	c, err := New(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c, &delays
}

func TestRetries(t *testing.T) {
	soon := time.Now().Add(20 * time.Second).UTC().Format(http.TimeFormat)
	tests := []struct {
		name       string
		steps      []step
		wantStatus int // of the APIError, or 0 for success
		wantTries  int32
		wantDelays []time.Duration // exact delays; nil to check the backoff range
	}{
		{"503 then success", []step{{status: 503}, {status: 200}}, 0, 2, nil},
		{"each retried status", []step{{status: 429}, {status: 502}, {status: 504}, {status: 200}}, 0, 4, nil},
		{"gives up after four tries", []step{{status: 503}}, 503, 4, nil},
		{"500 isn't retried", []step{{status: 500}}, 500, 1, nil},
		{"404 isn't retried", []step{{status: 404}}, 404, 1, nil},
		{"Retry-After in seconds", []step{{status: 429, header: map[string]string{"Retry-After": "7"}}, {status: 200}},
			0, 2, []time.Duration{7 * time.Second}},
		{"Retry-After is capped", []step{{status: 503, header: map[string]string{"Retry-After": "3600"}}, {status: 200}},
			0, 2, []time.Duration{time.Minute}},
		{"redirects aren't followed", []step{{status: 302, header: map[string]string{"Location": "http://elsewhere.example/"}}},
			302, 1, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, tries := scripted(t, tt.steps...)
			c, delays := testClient(t, srv.URL, Options{})
			_, err := c.Status(t.Context())
			var api *APIError
			switch {
			case tt.wantStatus == 0 && err != nil:
				t.Errorf("Status: %v", err)
			case tt.wantStatus != 0 && (!errors.As(err, &api) || api.Status != tt.wantStatus):
				t.Errorf("Status: %v, want an APIError with status %d", err, tt.wantStatus)
			}
			if tries.Load() != tt.wantTries {
				t.Errorf("%d requests, want %d", tries.Load(), tt.wantTries)
			}
			if tt.wantDelays != nil && !slices.Equal(*delays, tt.wantDelays) {
				t.Errorf("delays %v, want %v", *delays, tt.wantDelays)
			}
			for i, d := range *delays {
				if tt.wantDelays == nil && (d < 50*time.Millisecond<<i || d > 100*time.Millisecond<<i) {
					t.Errorf("delay %d is %v, outside the backoff for its attempt", i+1, d)
				}
			}
			if api != nil && api.Status == 302 && !strings.Contains(api.Detail, "http://elsewhere.example/") {
				t.Errorf("redirect detail = %q", api.Detail)
			}
		})
	}

	t.Run("Retry-After as a date", func(t *testing.T) {
		srv, _ := scripted(t, step{status: 503, header: map[string]string{"Retry-After": soon}}, step{status: 200})
		c, delays := testClient(t, srv.URL, Options{})
		if _, err := c.Status(t.Context()); err != nil {
			t.Fatal(err)
		}
		if len(*delays) != 1 || (*delays)[0] < 15*time.Second || (*delays)[0] > 20*time.Second {
			t.Errorf("delays %v, want about 20s", *delays)
		}
	})
}

// TestBodyFailureIsRetried stalls partway through the first answer's body,
// past the timeout, and checks the request is tried again.
func TestBodyFailureIsRetried(t *testing.T) {
	body := recorded(t, "status.json")
	var tries atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tries.Add(1) == 1 {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			_, _ = w.Write(body[:len(body)/2])
			w.(http.Flusher).Flush()
			select {
			case <-time.After(time.Second):
			case <-r.Context().Done():
			}
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	c, delays := testClient(t, srv.URL, Options{Timeout: 200 * time.Millisecond})
	if _, err := c.Status(t.Context()); err != nil || tries.Load() != 2 || len(*delays) != 1 {
		t.Errorf("Status after %d requests: %v", tries.Load(), err)
	}
}

func TestTimeout(t *testing.T) {
	t.Run("a slow answer is retried", func(t *testing.T) {
		srv, tries := scripted(t, step{status: 200, delay: time.Second}, step{status: 200})
		c, _ := testClient(t, srv.URL, Options{Timeout: 100 * time.Millisecond})
		if _, err := c.Status(t.Context()); err != nil || tries.Load() != 2 {
			t.Errorf("Status after %d requests: %v", tries.Load(), err)
		}
	})
	t.Run("always slow", func(t *testing.T) {
		srv, tries := scripted(t, step{status: 200, delay: time.Second})
		c, _ := testClient(t, srv.URL, Options{Timeout: 50 * time.Millisecond})
		_, err := c.Status(t.Context())
		var ue *UnreachableError
		var ne net.Error
		if !errors.As(err, &ue) || !errors.As(err, &ne) || !ne.Timeout() || tries.Load() != 4 {
			t.Errorf("Status after %d requests: %v, want a timeout after 4", tries.Load(), err)
		}
	})
	t.Run("nothing listening", func(t *testing.T) {
		l, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := l.Addr().String()
		_ = l.Close()
		c, delays := testClient(t, "http://"+addr, Options{})
		var ue *UnreachableError
		if _, err := c.Status(t.Context()); !errors.As(err, &ue) || len(*delays) != 3 {
			t.Errorf("Status after %d retries: %v, want an UnreachableError after 3", len(*delays), err)
		}
	})
}

func TestCanceledContextStopsRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var tries atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		tries.Add(1)
		cancel()
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	c, err := New(t.Context(), Options{URL: srv.URL, Token: config.NewSecret("nbt_k.s"), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	start := time.Now()
	if _, err := c.Status(ctx); err == nil {
		t.Error("Status succeeded")
	}
	if tries.Load() != 1 || time.Since(start) > 2*time.Second {
		t.Errorf("%d requests in %v; a canceled context should stop the retries", tries.Load(), time.Since(start))
	}
}

func TestTLS(t *testing.T) {
	body := recorded(t, "status.json")
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) })
	// The servers' own logs of the failed handshakes are expected.
	quiet := log.New(io.Discard, "", 0)
	srv := httptest.NewUnstartedServer(ok)
	srv.Config.ErrorLog = quiet
	srv.StartTLS()
	t.Cleanup(srv.Close)
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("an untrusted certificate isn't retried", func(t *testing.T) {
		c, delays := testClient(t, srv.URL, Options{})
		_, err := c.Status(t.Context())
		var cve *tls.CertificateVerificationError
		if !errors.As(err, &cve) || len(*delays) != 0 {
			t.Errorf("Status after %d retries: %v, want a certificate error and no retry", len(*delays), err)
		}
	})
	t.Run("the CA file is trusted", func(t *testing.T) {
		c, _ := testClient(t, srv.URL, Options{CAFile: caFile})
		if !c.Encrypted() {
			t.Error("Encrypted() = false for https://")
		}
		if _, err := c.Status(t.Context()); err != nil {
			t.Error(err)
		}
	})
	t.Run("TLS 1.1 is refused", func(t *testing.T) {
		old := httptest.NewUnstartedServer(ok)
		old.Config.ErrorLog = quiet
		old.TLS = &tls.Config{MinVersion: tls.VersionTLS10, MaxVersion: tls.VersionTLS11} //nolint:gosec // The server nbpdns must refuse.
		old.StartTLS()
		t.Cleanup(old.Close)
		c, delays := testClient(t, old.URL, Options{CAFile: caFile})
		if _, err := c.Status(t.Context()); err == nil || len(*delays) != 0 {
			t.Errorf("Status after %d retries: %v, want a failure and no retry", len(*delays), err)
		}
	})
	t.Run("https:// to a plain HTTP server isn't retried", func(t *testing.T) {
		plain, _ := scripted(t, step{status: 200})
		c, delays := testClient(t, strings.Replace(plain.URL, "http://", "https://", 1), Options{})
		if _, err := c.Status(t.Context()); err == nil || !strings.Contains(err.Error(), "HTTP response to HTTPS client") || len(*delays) != 0 {
			t.Errorf("Status after %d retries: %v, want a failure and no retry", len(*delays), err)
		}
	})
	t.Run("a bad CA file", func(t *testing.T) {
		notPEM := filepath.Join(t.TempDir(), "ca.pem")
		if err := os.WriteFile(notPEM, []byte("not a certificate"), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, file := range []string{notPEM, filepath.Join(t.TempDir(), "missing.pem")} {
			_, err := New(t.Context(), Options{URL: srv.URL, Token: config.NewSecret("nbt_k.s"), CAFile: file})
			if err == nil || !strings.Contains(err.Error(), "netbox.ca_file") {
				t.Errorf("New with CA file %s: %v", filepath.Base(file), err)
			}
		}
	})
}

func TestDenied(t *testing.T) {
	tests := []struct {
		name         string
		statusAnswer int // what /api/status/ answers; everything else gets 403
		wantAuth     bool
	}{
		{"a valid token without permission", http.StatusOK, false},
		{"a token NetBox rejects", http.StatusForbidden, true},
		{"no token at all", http.StatusUnauthorized, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := recorded(t, "status.json")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/status/" {
					w.WriteHeader(tt.statusAnswer)
					if tt.statusAnswer == http.StatusOK {
						_, _ = w.Write(body)
					}
					return
				}
				w.WriteHeader(http.StatusForbidden)
			}))
			t.Cleanup(srv.Close)
			c, _ := testClient(t, srv.URL, Options{})
			for _, typ := range ObjectTypes {
				_, err := c.Count(t.Context(), typ)
				var pe *PermissionError
				var ae *AuthError
				switch {
				case tt.wantAuth && !errors.As(err, &ae):
					t.Errorf("Count(%s): %v, want an AuthError", typ, err)
				case !tt.wantAuth && (!errors.As(err, &pe) || pe.ObjectType != typ):
					t.Errorf("Count(%s): %v, want a PermissionError naming it", typ, err)
				}
			}
		})
	}
}

func TestRequestHeaders(t *testing.T) {
	traceparent := regexp.MustCompile(`^00-[0-9a-f]{32}-[0-9a-f]{16}-01$`)
	tests := []struct {
		name, token, wantAuth string
		traced                bool
	}{
		{"v2 token", "nbt_abc.def", "Bearer nbt_abc.def", true},
		{"v1 token", "notarealv1token", "Token notarealv1token", true},
		{"no tracer", "nbt_abc.def", "Bearer nbt_abc.def", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := recorded(t, "status.json")
			var got http.Header
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Clone()
				_, _ = w.Write(body)
			}))
			t.Cleanup(srv.Close)
			o := Options{Token: config.NewSecret(tt.token)}
			if tt.traced {
				tp := tracing.NewProvider("test")
				t.Cleanup(func() { _ = tp.Shutdown(context.WithoutCancel(t.Context())) })
				o.Tracer = tracing.Tracer(tp)
			}
			c, _ := testClient(t, srv.URL, o)
			if _, err := c.Status(t.Context()); err != nil {
				t.Fatal(err)
			}
			if a := got.Get("Authorization"); a != tt.wantAuth {
				t.Errorf("Authorization = %q, want %q", a, tt.wantAuth)
			}
			if a := got.Get("Accept"); a != "application/json" {
				t.Errorf("Accept = %q", a)
			}
			if tp := got.Get("Traceparent"); traceparent.MatchString(tp) != tt.traced {
				t.Errorf("traceparent = %q, want one: %v", tp, tt.traced)
			}
		})
	}
}

func TestWarnings(t *testing.T) {
	tests := []struct {
		name, url, token string
		want             []string
	}{
		{"https and v2", "https://netbox.example.com", "nbt_abc.def", nil},
		{"http", "http://netbox.example.com", "nbt_abc.def", []string{"crosses the network unencrypted"}},
		{"v1 token", "https://netbox.example.com", "notarealv1token", []string{"v1 token"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			c, err := New(t.Context(), Options{URL: tt.url, Token: config.NewSecret(tt.token),
				Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
			if err != nil {
				t.Fatal(err)
			}
			c.Close()
			lines := strings.Count(logs.String(), `"level":"WARN"`)
			if lines != len(tt.want) {
				t.Errorf("%d warnings, want %d:\n%s", lines, len(tt.want), logs.String())
			}
			for _, w := range tt.want {
				if !strings.Contains(logs.String(), w) {
					t.Errorf("no warning containing %q:\n%s", w, logs.String())
				}
			}
		})
	}
}

// TestTokenNeverLogged makes requests fail every way the client logs, at
// debug level, and checks the token never appears.
func TestTokenNeverLogged(t *testing.T) {
	const token = "nbt_logkey.thesecretpartofthetoken"
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	for _, steps := range [][]step{{{status: 503}}, {{status: 403}}, {{status: 200, delay: time.Second}}} {
		srv, _ := scripted(t, steps...)
		c, _ := testClient(t, srv.URL, Options{Token: config.NewSecret(token), Logger: log, Timeout: 50 * time.Millisecond})
		_, _ = c.Views(t.Context())
	}
	if logs.Len() == 0 {
		t.Fatal("nothing was logged")
	}
	if strings.Contains(logs.String(), "thesecretpart") {
		t.Errorf("the token is in the log:\n%s", logs.String())
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		o    Options
		want string
	}{
		{"no URL", Options{Token: config.NewSecret("nbt_k.s")}, "netbox.url isn't set"},
		{"no token", Options{URL: "https://netbox.example.com"}, "netbox.token isn't set"},
	}
	for _, tt := range tests {
		if _, err := New(t.Context(), tt.o); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: %v, want %q", tt.name, err, tt.want)
		}
	}
}

func TestEndpointAndRebase(t *testing.T) {
	c, _ := testClient(t, "https://netbox.example.com/netbox", Options{})
	if got := c.endpoint("api/status/", nil).String(); got != "https://netbox.example.com/netbox/api/status/" {
		t.Errorf("endpoint = %s", got)
	}
	// NetBox behind a proxy may build its links with another scheme, host or
	// path; the token only ever goes to the configured one, and the proxy's
	// path prefix stays.
	current := c.endpoint(pluginAPI+"records/", url.Values{"limit": {"10"}})
	next, err := c.rebase(current, "http://internal:8080/api/plugins/netbox-dns/records/?limit=10&offset=10")
	if err != nil || next.String() != "https://netbox.example.com/netbox/api/plugins/netbox-dns/records/?limit=10&offset=10" {
		t.Errorf("rebase = %v, %v", next, err)
	}
}

func TestConsistent(t *testing.T) {
	tests := []struct {
		counts []int
		n      int
		want   bool
	}{
		{[]int{39}, 39, true},
		{[]int{39, 39, 39, 39}, 39, true},
		{[]int{0}, 0, true},
		{[]int{39, 38, 38, 38}, 38, false}, // one deleted while reading
		{[]int{39, 40, 40, 40}, 40, false}, // one added while reading
		{[]int{39, 39}, 38, false},
		{[]int{-1}, 39, false},
		{nil, 0, false},
	}
	for _, tt := range tests {
		if got := consistent(tt.counts, tt.n); got != tt.want {
			t.Errorf("consistent(%v, %d) = %v, want %v", tt.counts, tt.n, got, tt.want)
		}
	}
}

// TestListChecksCounts serves the recorded page of 39 records with its
// count rewritten, as a list that changes, or a broken answer, would give.
func TestListChecksCounts(t *testing.T) {
	var page map[string]json.RawMessage
	if err := json.Unmarshal(recorded(t, "records.json"), &page); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		counts    []string // the count each request gets; the last repeats
		wantErr   bool
		wantTries int32
	}{
		{"unchanged", []string{"39"}, false, 1},
		{"changed while read, then not", []string{"40", "39"}, false, 2},
		{"keeps changing", []string{"40"}, true, 3},
		{"a negative count", []string{"-1"}, true, 3},
		{"a huge count", []string{"10000000000"}, true, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tries atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				p := maps.Clone(page)
				p["count"] = json.RawMessage(tt.counts[min(int(tries.Add(1)), len(tt.counts))-1])
				b, _ := json.Marshal(p)
				_, _ = w.Write(b)
			}))
			t.Cleanup(srv.Close)
			c, _ := testClient(t, srv.URL, Options{})
			records, err := c.Records(t.Context(), 1)
			if (err != nil) != tt.wantErr || tries.Load() != tt.wantTries {
				t.Errorf("Records: %d records after %d reads, %v", len(records), tries.Load(), err)
			}
			if err == nil && len(records) != 39 {
				t.Errorf("%d records, want 39", len(records))
			}
		})
	}
}

// TestZoneFilter checks the query that each filter sends.
func TestZoneFilter(t *testing.T) {
	tests := []struct {
		name string
		f    ZoneFilter
		want url.Values
	}{
		{"everything", ZoneFilter{}, url.Values{}},
		{"names, whatever their case, in views", ZoneFilter{Names: []string{"a.example", "b.example"}, Views: []string{"v", "w"}, Status: "active"},
			url.Values{"name__ie": {"a.example", "b.example"}, "view": {"v", "w"}, "status": {"active"}}},
		{"empty names and views", ZoneFilter{Names: []string{""}, Views: []string{""}}, url.Values{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got url.Values
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.URL.Query()
				_, _ = w.Write([]byte(`{"count": 0, "next": null, "results": []}`))
			}))
			t.Cleanup(srv.Close)
			c, _ := testClient(t, srv.URL, Options{})
			if _, err := c.Zones(t.Context(), tt.f); err != nil {
				t.Fatal(err)
			}
			// Every list pages in a stable order.
			for _, k := range []string{"limit", "offset", "ordering"} {
				got.Del(k)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("query %v, want %v", got, tt.want)
			}
		})
	}
}

// TestReadZonesConcurrency serves the recorded records page for every zone,
// slowly, and checks how many requests ReadZones has in flight.
func TestReadZonesConcurrency(t *testing.T) {
	body := recorded(t, "records.json")
	var mu sync.Mutex
	inFlight, most := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		inFlight++
		most = max(most, inFlight)
		mu.Unlock()
		defer func() { mu.Lock(); inFlight--; mu.Unlock() }()
		time.Sleep(20 * time.Millisecond)
		if r.URL.Query().Get("zone_id") == "13" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	c, _ := testClient(t, srv.URL, Options{Concurrency: 3})

	zones := make([]Zone, 12)
	for i := range zones {
		zones[i] = Zone{ID: i + 1, Name: "rec.nbpdns.example", DefaultTTL: 3600}
	}
	read, _, err := c.ReadZones(t.Context(), zones)
	if err != nil || len(read) != len(zones) {
		t.Fatalf("ReadZones: %d zones, %v", len(read), err)
	}
	if most != 3 {
		t.Errorf("at most %d requests in flight, want 3", most)
	}

	zones = append(zones, Zone{ID: 13, Name: "missing.example", View: View{Name: "v"}})
	var api *APIError
	if _, _, err := c.ReadZones(t.Context(), zones); !errors.As(err, &api) || !strings.Contains(err.Error(), "zone missing.example in view v") {
		t.Errorf("ReadZones with a failing zone: %v", err)
	}
}
