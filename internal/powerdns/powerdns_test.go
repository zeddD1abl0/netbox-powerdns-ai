package powerdns

// These tests run the client against local HTTP servers that send only
// status codes, headers and delays, and, where a body is needed, responses
// recorded from the lab (ADR-0026). PowerDNS's own behavior is tested against
// the lab, in powerdns_integration_test.go. Transport behavior, such as
// retries and TLS, is tested in internal/httpclient.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/httpclient"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/tracing"
)

// recorded returns a response recorded from the lab server p (TestRecord).
func recorded(t *testing.T, p lab.PowerDNS, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", p.Name, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// testClient returns a client for url with o's settings, whose retries
// don't sleep.
func testClient(t *testing.T, url string, o Options) *Client {
	t.Helper()
	if o.Group == "" {
		o.Group = "test"
	}
	o.Primary.URL = url
	if !o.Primary.APIKey.IsSet() {
		o.Primary.APIKey = config.NewSecret("the-test-key")
	}
	if o.Timeout == 0 {
		o.Timeout = 5 * time.Second
	}
	o.retry = httpclient.Retry{Attempts: 2, Base: time.Millisecond, Cap: time.Millisecond}
	c, err := New(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

// TestRecordedFixture maps the responses recorded from each lab server into
// the normalized model, and checks the result against the fixture they were
// recorded from.
func TestRecordedFixture(t *testing.T) {
	f := lab.DescribePowerDNSFixture("rec")
	for _, p := range lab.PowerDNSes {
		t.Run(p.Name, func(t *testing.T) {
			var (
				server Server
				zones  []Zone
				zone   ZoneData
			)
			for name, v := range map[string]any{"server.json": &server, "zones.json": &zones, "zone.json": &zone} {
				if err := json.Unmarshal(recorded(t, p, name), v); err != nil {
					t.Fatalf("%s: %v", name, err)
				}
			}
			if err := server.check(p.Group, Supported); err != nil || !strings.HasPrefix(server.Version, p.Version+".") {
				t.Errorf("server %+v: %v", server, err)
			}
			names := make([]string, len(zones))
			for i, z := range zones {
				names[i] = z.Name
			}
			if want := append([]string{f.Zone}, f.Others...); !slices.Equal(names, want) {
				t.Errorf("zones %v, want %v; record them again", names, want)
			}
			z, probs := zone.DNS()
			checkFixtureZone(t, f, z, probs)
		})
	}
}

// TestSupportedMatchesLab checks that the lab runs every supported release,
// and nothing else, so the integration tests cover exactly them.
func TestSupportedMatchesLab(t *testing.T) {
	var releases []string
	for _, p := range lab.PowerDNSes {
		releases = append(releases, p.Version)
	}
	if !slices.Equal(Supported, releases) {
		t.Errorf("Supported = %v, but the lab runs %v", Supported, releases)
	}
}

func TestServerCheck(t *testing.T) {
	tests := []struct {
		daemon, version string
		want            string // part of the error, or "" for supported
	}{
		{"authoritative", "5.1.4", ""},
		{"authoritative", "v5.1.0", ""},
		{"authoritative", "5.0.7", "runs PowerDNS 5.0.7, which isn't supported; nbpdns supports PowerDNS 5.1.x"},
		{"authoritative", "4.9.5", "isn't supported"},
		{"authoritative", "5.2.0-alpha1", "isn't supported"},
		{"recursor", "5.1.4", "is a PowerDNS recursor, not an authoritative server"},
	}
	for _, tt := range tests {
		s := &Server{DaemonType: tt.daemon, Version: tt.version}
		err := s.check("site-a", Supported)
		if (err == nil) != (tt.want == "") || (err != nil && !strings.Contains(err.Error(), tt.want)) {
			t.Errorf("%s %s: %v, want an error containing %q", tt.daemon, tt.version, err, tt.want)
		}
	}
}

// TestServerReleases checks a server against a list of several releases, as
// Supported was and will be again (ADR-0026).
func TestServerReleases(t *testing.T) {
	releases := []string{"5.1", "5.0"}
	for _, tt := range []struct {
		version string
		want    bool
	}{{"5.1.4", true}, {"5.0.7", true}, {"4.9.5", false}} {
		if got := (&Server{DaemonType: "authoritative", Version: tt.version}).supported(releases); got != tt.want {
			t.Errorf("%s: supported = %v, want %v", tt.version, got, tt.want)
		}
	}
	if got := seriesText(releases); got != "5.1.x or 5.0.x" {
		t.Errorf("seriesText = %q", got)
	}
}

// scripted returns a server that answers each request with answer(path).
func scripted(t *testing.T, answer func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(answer))
	t.Cleanup(srv.Close)
	return srv
}

func TestErrors(t *testing.T) {
	p := lab.PowerDNSes[0]
	server, zone := recorded(t, p, "server.json"), recorded(t, p, "zone.json")
	recursor := bytes.Replace(server, []byte(`"authoritative"`), []byte(`"recursor"`), 1)
	tests := []struct {
		name   string
		answer func(w http.ResponseWriter, r *http.Request)
		call   func(c *Client) error
		check  func(err error) bool
	}{
		{"a rejected key", status(http.StatusUnauthorized, "Unauthorized"),
			func(c *Client) error { _, err := c.Server(t.Context()); return err },
			func(err error) bool { var e *AuthError; return errors.As(err, &e) && e.Detail == "Unauthorized" }},
		{"no such server, in JSON", status(http.StatusNotFound, `{"error": "Not Found"}`),
			func(c *Client) error { _, err := c.Connect(t.Context()); return err },
			func(err error) bool {
				var e *ServerNotFoundError
				return errors.As(err, &e) && e.ServerID == "localhost" && strings.HasSuffix(e.URL, "/api/v1/servers/localhost") &&
					strings.Contains(err.Error(), "check powerdns.groups.test.primary.server_id") &&
					strings.Contains(err.Error(), "powerdns.groups.test.primary.url is the address in front of /api/v1")
			}},
		{"no such server, in plain text", status(http.StatusNotFound, "Not Found"),
			func(c *Client) error { _, err := c.Server(t.Context()); return err },
			func(err error) bool { var e *ServerNotFoundError; return errors.As(err, &e) }},
		{"no such zone", status(http.StatusNotFound, `{"error": "Could not find domain 'x.'"}`),
			func(c *Client) error { _, err := c.ZoneData(t.Context(), Zone{ID: "x.", Name: "x."}); return err },
			func(err error) bool { var e *ZoneNotFoundError; return errors.As(err, &e) && e.Zone == "x." }},
		{"an error with PowerDNS's detail", status(http.StatusUnprocessableEntity, `{"error": "something is wrong"}`),
			func(c *Client) error { _, err := c.Zones(t.Context()); return err },
			func(err error) bool {
				var e *APIError
				return errors.As(err, &e) && e.Status == 422 && e.Detail == "something is wrong" && e.Path == "/api/v1/servers/localhost/zones?dnssec=false"
			}},
		{"a redirect", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", "http://elsewhere.example/")
			w.WriteHeader(http.StatusFound)
		}, func(c *Client) error { _, err := c.Zones(t.Context()); return err },
			func(err error) bool {
				var e *APIError
				return errors.As(err, &e) && strings.Contains(e.Detail, "http://elsewhere.example/") && strings.Contains(e.Detail, "powerdns.groups.test.primary.url")
			}},
		{"a recursor", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(recursor) },
			func(c *Client) error { _, err := c.Connect(t.Context()); return err },
			func(err error) bool {
				var e *NotAuthoritativeError
				return errors.As(err, &e) && e.DaemonType == "recursor"
			}},
		{"no zone of that name", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("[]")) },
			func(c *Client) error { _, err := c.FindZone(t.Context(), "missing.example."); return err },
			func(err error) bool { var e *ZoneNotFoundError; return errors.As(err, &e) }},
		{"a zone, found", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("zone") != "rec.nbpdns.example." {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = fmt.Fprint(w, `[{"id": "rec.nbpdns.example.", "name": "rec.nbpdns.example.", "kind": "Native"}]`)
		}, func(c *Client) error {
			z, err := c.FindZone(t.Context(), "REC.nbpdns.example")
			if err == nil && z.ID != "rec.nbpdns.example." {
				err = fmt.Errorf("found %+v", z)
			}
			return err
		}, func(err error) bool { return err == nil }},
		{"the root zone's data", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.EscapedPath() != "/api/v1/servers/localhost/zones/." {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(zone)
		}, func(c *Client) error {
			_, err := c.ZoneData(t.Context(), Zone{ID: ".", Name: "."})
			return err
		}, func(err error) bool { return err == nil }},
		{"a zone's data", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/servers/localhost/zones/rec.nbpdns.example." {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(zone)
		}, func(c *Client) error {
			_, err := c.ZoneData(t.Context(), Zone{ID: "rec.nbpdns.example.", Name: "rec.nbpdns.example."})
			return err
		}, func(err error) bool { return err == nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := scripted(t, tt.answer)
			if err := tt.call(testClient(t, srv.URL, Options{})); !tt.check(err) {
				t.Errorf("got %v", err)
			}
		})
	}
}

// status returns an answer with code and body.
func status(code int, body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}
}

func TestRequests(t *testing.T) {
	traceparent := regexp.MustCompile(`^00-[0-9a-f]{32}-[0-9a-f]{16}-01$`)
	server := recorded(t, lab.PowerDNSes[0], "server.json")
	var got *http.Request
	srv := scripted(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(context.WithoutCancel(r.Context()))
		_, _ = w.Write(server)
	})
	tp := tracing.NewProvider("test")
	t.Cleanup(func() { _ = tp.Shutdown(context.WithoutCancel(t.Context())) })
	c := testClient(t, srv.URL+"/pdns", Options{Primary: config.Primary{APIKey: config.NewSecret("k1"), ServerID: "srv1"}, Tracer: tracing.Tracer(tp)})
	if _, err := c.Server(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got.URL.Path != "/pdns/api/v1/servers/srv1" {
		t.Errorf("path = %s", got.URL.Path)
	}
	if got.Header.Get("X-API-Key") != "k1" || got.Header.Get("Accept") != "application/json" {
		t.Errorf("headers = %v", got.Header)
	}
	if !traceparent.MatchString(got.Header.Get("Traceparent")) {
		t.Errorf("traceparent = %q", got.Header.Get("Traceparent"))
	}
	if c.URL() != srv.URL+"/pdns/api/v1/servers/srv1" || c.Encrypted() || c.Group() != "test" {
		t.Errorf("URL %s, Encrypted %v, Group %s", c.URL(), c.Encrypted(), c.Group())
	}
}

// TestKeyNeverLogged makes requests fail every way the client logs, at debug
// level, and checks the key never appears.
func TestKeyNeverLogged(t *testing.T) {
	const key = "the-secret-part-of-the-api-key"
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	for _, code := range []int{http.StatusServiceUnavailable, http.StatusUnauthorized, http.StatusInternalServerError} {
		srv := scripted(t, status(code, `{"error": "no"}`))
		c := testClient(t, srv.URL, Options{Primary: config.Primary{APIKey: config.NewSecret(key)}, Logger: log})
		_, err := c.Zones(t.Context())
		if err == nil || strings.Contains(err.Error(), key) {
			t.Errorf("error %v", err)
		}
	}
	if logs.Len() == 0 {
		t.Fatal("nothing was logged")
	}
	if strings.Contains(logs.String(), key) {
		t.Errorf("the key is in the log:\n%s", logs.String())
	}
}

func TestNew(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	for _, url := range []string{"https://pdns.example.com", "http://pdns.example.com"} {
		logs.Reset()
		c, err := New(t.Context(), Options{Group: "site-a", Logger: log,
			Primary: config.Primary{URL: url, APIKey: config.NewSecret("k")}})
		if err != nil {
			t.Fatal(err)
		}
		c.Close()
		warned := strings.Contains(logs.String(), "crosses the network unencrypted") && strings.Contains(logs.String(), `"group":"site-a"`)
		if warned != strings.HasPrefix(url, "http://") {
			t.Errorf("%s: warnings:\n%s", url, logs.String())
		}
	}
	tests := []struct {
		name    string
		primary config.Primary
		want    string
	}{
		{"no URL", config.Primary{APIKey: config.NewSecret("k")}, "server group site-a needs powerdns.groups.site-a.primary.url"},
		{"no key", config.Primary{URL: "https://pdns.example.com"}, "api_key_file"},
		{"a bad CA file", config.Primary{URL: "https://pdns.example.com", APIKey: config.NewSecret("k"), CAFile: "/does/not/exist"},
			"powerdns.groups.site-a.primary.ca_file"},
	}
	for _, tt := range tests {
		if _, err := New(t.Context(), Options{Group: "site-a", Primary: tt.primary}); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: %v, want %q", tt.name, err, tt.want)
		}
	}
}

func TestDetail(t *testing.T) {
	long := strings.Repeat("é", 150) // 300 bytes, 2 to a character
	tests := []struct{ body, want string }{
		{`{"error": "Not Found"}`, "Not Found"},
		{"Not Found\n", "Not Found"},
		{"bad \xff byte", "bad byte"},
		{"two\nlines", "two lines"},
		{"<html>\n<head><title>400 No required SSL certificate was sent</title></head>\n<body>…</body></html>",
			"400 No required SSL certificate was sent"},
		{long, long[:200]},
		// 200 bytes would split a character, so the last half goes.
		{"x" + long, ("x" + long)[:199]},
	}
	for _, tt := range tests {
		if got := detail([]byte(tt.body)); got != tt.want {
			t.Errorf("detail(%.20q) = %.20q, want %.20q", tt.body, got, tt.want)
		}
	}
}

// TestReadZonesConcurrency serves the recorded zone for every zone, and
// checks that no more requests are in flight than the client allows, and
// that the zones come back in the order given.
func TestReadZonesConcurrency(t *testing.T) {
	zone := recorded(t, lab.PowerDNSes[0], "zone.json")
	var inFlight, peak atomic.Int32
	srv := scripted(t, func(w http.ResponseWriter, _ *http.Request) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write(zone)
	})
	c := testClient(t, srv.URL, Options{Concurrency: 3})
	zones := make([]Zone, 10)
	for i := range zones {
		zones[i] = Zone{ID: fmt.Sprintf("z%d.", i), Name: fmt.Sprintf("z%d.", i)}
	}
	read, _, err := c.ReadZones(t.Context(), zones)
	if err != nil {
		t.Fatal(err)
	}
	if len(read) != len(zones) || peak.Load() > 3 || peak.Load() < 2 {
		t.Errorf("read %d zones, with up to %d requests in flight, want 3", len(read), peak.Load())
	}

	t.Run("the first error stops it", func(t *testing.T) {
		var tries atomic.Int32
		srv := scripted(t, func(w http.ResponseWriter, _ *http.Request) {
			tries.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
		})
		c := testClient(t, srv.URL, Options{Concurrency: 1})
		var ae *AuthError
		if _, _, err := c.ReadZones(t.Context(), zones); !errors.As(err, &ae) || !strings.Contains(err.Error(), "reading zone z0.") || tries.Load() > 2 {
			t.Errorf("ReadZones after %d requests: %v", tries.Load(), err)
		}
	})
}
