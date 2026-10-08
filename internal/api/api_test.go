package api

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api/contract"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
)

// checker checks every response the tests get against the API's OpenAPI
// document, and TestMain fails if any operation or listed status code went
// unchecked.
var checker *contract.Checker

func TestMain(m *testing.M) {
	var err error
	if checker, err = contract.New(Spec(), "/api"); err != nil {
		fmt.Fprintln(os.Stderr, "the API's OpenAPI document:", err)
		os.Exit(1)
	}
	code := m.Run()
	// Only a run of every test can cover every operation.
	if code == 0 && flag.Lookup("test.run").Value.String() == "" {
		if missing := checker.Missing(); len(missing) > 0 {
			fmt.Fprintf(os.Stderr, "no test checks these responses against the OpenAPI document:\n  %s\n", strings.Join(missing, "\n  "))
			code = 1
		}
	}
	os.Exit(code)
}

// fakeSource serves fixed state.
type fakeSource struct{ status service.Status }

func (f fakeSource) Status() service.Status { return f.status }

// A reply is a response's status code, header and body.
type reply struct {
	code   int
	header http.Header
	body   []byte
}

// get sends a request to the API over src, checks the response against the
// OpenAPI document, and returns it.
func get(t *testing.T, src Source, method, target string, header http.Header) reply {
	t.Helper()
	h := New(Options{Source: src, Log: slog.New(slog.DiscardHandler)})
	req := httptest.NewRequestWithContext(t.Context(), method, target, nil)
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()
	if strings.HasPrefix(req.URL.Path, "/api/") && req.URL.Path != "/api/openapi.yaml" {
		checker.Check(t, req, resp)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return reply{resp.StatusCode, resp.Header, body}
}

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr[T any](v T) *T { return &v }

func TestStatus(t *testing.T) {
	started := at("2026-10-08T01:00:00Z")
	before := service.Status{
		Version: "v0.2.0", Revision: "ec048d8", Started: started, UptimeSeconds: 1.5,
		Schedule: service.Schedule{IntervalSeconds: 300, TimeoutSeconds: 600},
		NetBox:   service.NetBox{URL: "https://netbox.example.com/"},
		Groups:   []service.GroupInfo{},
	}
	failed := before
	failed.Ready, failed.Live = true, true
	failed.Schedule.LastRefresh = &service.Refresh{
		Started: at("2026-10-08T01:05:00Z"), Finished: at("2026-10-08T01:05:01Z"), DurationSeconds: 1,
		Outcome: "failed", Error: "reading NetBox: connection refused",
	}
	failed.Schedule.NextRefresh = ptr(at("2026-10-08T01:10:00Z"))
	failed.Schedule.Refreshes = service.Refreshes{Failed: 1}
	failed.NetBox.Up, failed.NetBox.Error = ptr(false), "connection refused"
	exporting := failed
	exporting.Tracing = service.Tracing{Exported: true, Endpoint: "https://otel.example.com:4318", Protocol: "grpc"}

	tests := []struct {
		name   string
		status service.Status
		want   []string // fragments of the JSON, compacted with its keys sorted
	}{
		{"before the first refresh", before, []string{
			`"live":false`, `"ready":false`, `"netbox":{"error":null,"up":null,"url":"https://netbox.example.com/"}`,
			`"schedule":{"interval_seconds":300,"last_complete_refresh":null,"last_refresh":null,"next_refresh":null,"refreshes":{"complete":0,"failed":0,"incomplete":0},"timeout_seconds":600}`,
		}},
		{"after a failed refresh", failed, []string{
			`"live":true`, `"ready":true`, `"netbox":{"error":"connection refused","up":false,"url":"https://netbox.example.com/"}`,
			`"last_refresh":{"duration_seconds":1,"error":"reading NetBox: connection refused","finished":"2026-10-08T01:05:01Z","outcome":"failed","started":"2026-10-08T01:05:00Z"}`,
			`"next_refresh":"2026-10-08T01:10:00Z"`, `"refreshes":{"complete":0,"failed":1,"incomplete":0}`,
		}},
		{"not exporting spans", failed, []string{`"tracing":{"endpoint":null,"exported":false,"protocol":null}`}},
		{"exporting spans", exporting, []string{`"tracing":{"endpoint":"https://otel.example.com:4318","exported":true,"protocol":"grpc"}`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := get(t, fakeSource{tt.status}, http.MethodGet, "/api/status", nil)
			if r.code != http.StatusOK || r.header.Get("Content-Type") != "application/json" {
				t.Fatalf("%d %s: %s", r.code, r.header.Get("Content-Type"), r.body)
			}
			got := compact(t, r.body)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("status has no\n%s\nin\n%s", w, got)
				}
			}
		})
	}
}

// compact returns the JSON in body with its keys sorted and no spaces.
func compact(t *testing.T, body []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSpec(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		r := get(t, fakeSource{}, method, "/api/openapi.yaml", nil)
		if r.code != http.StatusOK || r.header.Get("Content-Type") != "application/yaml" {
			t.Fatalf("%s: %d %s", method, r.code, r.header.Get("Content-Type"))
		}
		if method == http.MethodGet && string(r.body) != string(Spec()) {
			t.Error("the served spec isn't the embedded one")
		}
	}
	if !strings.Contains(string(Spec()), "x-api-id: f4c4a283-c96e-410e-a606-bdda49d9af95") {
		t.Error("the embedded spec has another API ID")
	}
}

func TestNoStore(t *testing.T) {
	for _, path := range []string{"/api/status", "/api/openapi.yaml"} {
		if r := get(t, fakeSource{}, http.MethodGet, path, nil); r.header.Get("Cache-Control") != "no-store" {
			t.Errorf("%s: Cache-Control %q", path, r.header.Get("Cache-Control"))
		}
	}
}
