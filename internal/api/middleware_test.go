package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api/gen"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/logging"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
)

// instrumented is the API with its log, spans and metrics recorded.
type instrumented struct {
	h       http.Handler
	log     *bytes.Buffer
	spans   *tracetest.SpanRecorder
	metrics *metrics.Metrics
}

func newInstrumented(t *testing.T) *instrumented {
	t.Helper()
	in := &instrumented{log: &bytes.Buffer{}, spans: tracetest.NewSpanRecorder(), metrics: metrics.New(version.Info{})}
	log, err := logging.New(in.log, "json", "info")
	if err != nil {
		t.Fatal(err)
	}
	in.h = New(Options{
		Source: fakeSource{}, Log: log, Metrics: in.metrics,
		Tracer: sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(in.spans)).Tracer("test"),
	})
	return in
}

// do sends a request, checks the response against the OpenAPI document if
// it's for one of its operations, and returns it.
func (in *instrumented) do(t *testing.T, method, target string, header http.Header) reply {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, target, nil)
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	in.h.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()
	if _, ok := in.h.(*handler).ops[method+" "+strings.SplitN(req.URL.Path, "?", 2)[0]]; ok {
		checker.Check(t, req, resp)
	}
	return reply{rec.Code, rec.Header(), rec.Body.Bytes()}
}

func TestFlowID(t *testing.T) {
	tests := []struct {
		name, sent string
		kept       bool
	}{
		{"kept", "flow-1.2:3+4/5=", true},
		{"none sent", "", false},
		{"too long", strings.Repeat("a", 129), false},
		{"bad characters", "flow id", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := newInstrumented(t)
			h := http.Header{}
			if tt.sent != "" {
				h.Set("X-Flow-ID", tt.sent)
			}
			r := in.do(t, http.MethodGet, "/api/status", h)
			got := r.header.Get("X-Flow-ID")
			if tt.kept && got != tt.sent || !tt.kept && (got == tt.sent || !flowIDRE.MatchString(got)) {
				t.Errorf("X-Flow-ID %q for %q", got, tt.sent)
			}
			var line struct {
				Msg       string `json:"msg"`
				RequestID string `json:"request_id"`
				Operation string `json:"operation"`
				Status    int    `json:"status"`
			}
			if err := json.Unmarshal(in.log.Bytes(), &line); err != nil || line.Msg != "API request" ||
				line.RequestID != got || line.Operation != "getStatus" || line.Status != http.StatusOK {
				t.Errorf("log line %+v, %v, want request_id %q:\n%s", line, err, got, in.log)
			}
		})
	}
}

func TestTraceparent(t *testing.T) {
	in := newInstrumented(t)
	const traceID, parent = "4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7"
	in.do(t, http.MethodGet, "/api/status", http.Header{"Traceparent": {"00-" + traceID + "-" + parent + "-01"}})
	spans := in.spans.Ended()
	if len(spans) != 1 {
		t.Fatalf("%d spans", len(spans))
	}
	s := spans[0]
	if s.Name() != "GET /api/status" || s.SpanContext().TraceID().String() != traceID || s.Parent().SpanID().String() != parent {
		t.Errorf("span %q in trace %s, parent %s", s.Name(), s.SpanContext().TraceID(), s.Parent().SpanID())
	}
	attrs := map[attribute.Key]attribute.Value{}
	for _, a := range s.Attributes() {
		attrs[a.Key] = a.Value
	}
	if attrs["http.route"].AsString() != "/api/status" || attrs["http.response.status_code"].AsInt64() != http.StatusOK {
		t.Errorf("span attributes %v", attrs)
	}
}

// problemOf decodes a problem response, and fails t unless it is one.
func problemOf(t *testing.T, r reply, status int) gen.Problem {
	t.Helper()
	var p gen.Problem
	d := json.NewDecoder(bytes.NewReader(r.body))
	d.DisallowUnknownFields()
	if r.code != status || r.header.Get("Content-Type") != "application/problem+json" || d.Decode(&p) != nil ||
		p.Type != "about:blank" || p.Title != http.StatusText(status) || int(p.Status) != status || p.Detail == nil || p.Instance == nil {
		t.Fatalf("%d %s: %s, want a %d problem", r.code, r.header.Get("Content-Type"), r.body, status)
	}
	return p
}

func TestProblems(t *testing.T) {
	in := newInstrumented(t)
	for _, path := range []string{"/api/nope", "/api/", "/api/status/more"} {
		p := problemOf(t, in.do(t, http.MethodGet, path, nil), http.StatusNotFound)
		if *p.Instance != path {
			t.Errorf("%s: instance %q", path, *p.Instance)
		}
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		r := in.do(t, method, "/api/status", nil)
		problemOf(t, r, http.StatusMethodNotAllowed)
		if r.header.Get("Allow") != "GET, HEAD" || r.header.Get("X-Flow-ID") == "" {
			t.Errorf("%s: Allow %q, X-Flow-ID %q", method, r.header.Get("Allow"), r.header.Get("X-Flow-ID"))
		}
	}
	// The mux's redirect to a cleaned path isn't a problem.
	if r := in.do(t, http.MethodGet, "/api//status", nil); r.code/100 != 3 || r.header.Get("Location") != "/api/status" {
		t.Errorf("/api//status: %d to %q", r.code, r.header.Get("Location"))
	}
}

func TestRequestMetrics(t *testing.T) {
	in := newInstrumented(t)
	in.do(t, http.MethodGet, "/api/status", nil)
	in.do(t, http.MethodGet, "/api/status", nil)
	in.do(t, http.MethodGet, "/api/openapi.yaml", nil)
	in.do(t, http.MethodGet, "/api/nope", nil)
	in.do(t, http.MethodPost, "/api/status", nil)
	in.do(t, http.MethodGet, "/api/docs", nil)
	in.do(t, http.MethodGet, "/api/docs/init.js", nil)
	want := `
# HELP nbpdns_api_requests_total Requests to the API. The operation is the request's ` + "`operationId`" + ` in ` + "`api/openapi.yaml`" + `, such as ` + "`getStatus`" + `, or ` + "`openapi`" + ` for the OpenAPI document, ` + "`docs`" + ` for the API reference, or ` + "`unmatched`" + ` for any other path or method. The code is the answer's status.
# TYPE nbpdns_api_requests_total counter
nbpdns_api_requests_total{code="200",operation="docs"} 2
nbpdns_api_requests_total{code="200",operation="getStatus"} 2
nbpdns_api_requests_total{code="200",operation="openapi"} 1
nbpdns_api_requests_total{code="404",operation="unmatched"} 1
nbpdns_api_requests_total{code="405",operation="unmatched"} 1
`
	if err := testutil.GatherAndCompare(in.metrics.Registry, strings.NewReader(want), "nbpdns_api_requests_total"); err != nil {
		t.Error(err)
	}
	if n := testutil.CollectAndCount(in.metrics.Registry, "nbpdns_api_request_duration_seconds"); n != 4 {
		t.Errorf("%d duration series, want one for each operation", n)
	}
}

func TestOperations(t *testing.T) {
	ops, err := operations(Spec(), base)
	if err != nil {
		t.Fatal(err)
	}
	if ops["GET /api/status"] != "getStatus" {
		t.Errorf("operations %v", ops)
	}
	// Every route of the generated server has its operationId.
	for pattern := range ops {
		if !strings.HasPrefix(pattern, "GET "+base+"/") {
			t.Errorf("route %q", pattern)
		}
	}
}

func TestLink(t *testing.T) {
	q := url.Values{"cursor": {"a b"}, "limit": {"2"}}
	tests := []struct {
		public, host, want string
	}{
		{"", "nbpdns.example.com:8080", "http://nbpdns.example.com:8080/api/server-groups?cursor=a+b&limit=2"},
		{"https://dns.example.com", "10.0.0.5:8080", "https://dns.example.com/api/server-groups?cursor=a+b&limit=2"},
		{"https://example.com/nbpdns/", "10.0.0.5:8080", "https://example.com/nbpdns/api/server-groups?cursor=a+b&limit=2"},
	}
	for _, tt := range tests {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/server-groups", nil)
		r.Host = tt.host
		if got := (Options{PublicURL: tt.public}).link(r, "/api/server-groups", q); got != tt.want {
			t.Errorf("link with %q, %q = %q, want %q", tt.public, tt.host, got, tt.want)
		}
	}
}

func TestErrorHandlers(t *testing.T) {
	var log bytes.Buffer
	o := Options{Log: slog.New(slog.NewJSONHandler(&log, nil))}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/status", nil)

	rec := httptest.NewRecorder()
	badRequest(rec, req, errors.New(`invalid format for parameter limit: "x" isn't a number`))
	p := problemOf(t, reply{rec.Code, rec.Header(), rec.Body.Bytes()}, http.StatusBadRequest)
	if !strings.Contains(*p.Detail, "parameter limit") {
		t.Errorf("400 detail %q", *p.Detail)
	}

	rec = httptest.NewRecorder()
	o.failed(rec, req, errors.New("the secret internals"))
	p = problemOf(t, reply{rec.Code, rec.Header(), rec.Body.Bytes()}, http.StatusInternalServerError)
	// The error is logged, and isn't sent to the client.
	if *p.Detail == "" || strings.Contains(*p.Detail, "secret") || !strings.Contains(log.String(), "the secret internals") {
		t.Errorf("500 detail %q, log:\n%s", *p.Detail, log.String())
	}
}
