package tracing

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

var traceparentRE = regexp.MustCompile(`^00-([0-9a-f]{32})-([0-9a-f]{16})-01$`)

// TestTransport sends requests through the Transport to a local test server
// and checks the traceparent header it receives, and the client spans.
func TestTransport(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		withParent bool
		wantError  bool // the span's status
	}{
		{"child of the command's span", http.StatusOK, true, false},
		{"root span without a parent", http.StatusOK, false, false},
		{"server error", http.StatusServiceUnavailable, true, true},
		{"client error isn't a span error", http.StatusNotFound, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Get("traceparent")
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			rec := tracetest.NewSpanRecorder()
			tp := NewProvider("test", sdktrace.WithSpanProcessor(rec))
			tracer := Tracer(tp)
			ctx := t.Context()
			var parent trace.Span
			if tt.withParent {
				ctx, parent = tracer.Start(ctx, "nbpdns netbox zones")
				defer parent.End()
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/status/?limit=1", http.NoBody)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := (&Transport{Tracer: tracer}).RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()

			m := traceparentRE.FindStringSubmatch(got)
			if m == nil {
				t.Fatalf("traceparent = %q, want W3C form", got)
			}
			if req.Header.Get("traceparent") != "" {
				t.Error("the Transport changed the caller's request")
			}
			spans := rec.Ended()
			if len(spans) != 1 {
				t.Fatalf("got %d ended spans, want the client span", len(spans))
			}
			client := spans[0]
			if client.SpanKind() != trace.SpanKindClient || client.Name() != http.MethodGet {
				t.Errorf("span = %s %v, want a GET client span", client.Name(), client.SpanKind())
			}
			if m[1] != client.SpanContext().TraceID().String() || m[2] != client.SpanContext().SpanID().String() {
				t.Errorf("traceparent %s doesn't name the client span %v", got, client.SpanContext())
			}
			if tt.withParent {
				if client.Parent().SpanID() != parent.SpanContext().SpanID() || m[1] != parent.SpanContext().TraceID().String() {
					t.Errorf("the client span isn't a child of the command's span")
				}
			}
			attrs := map[attribute.Key]attribute.Value{}
			for _, a := range client.Attributes() {
				attrs[a.Key] = a.Value
			}
			if attrs["http.request.method"].AsString() != http.MethodGet ||
				attrs["http.response.status_code"].AsInt64() != int64(tt.status) ||
				attrs["url.full"].AsString() != srv.URL+"/api/status/?limit=1" {
				t.Errorf("span attributes = %v", attrs)
			}
			if gotError := client.Status().Code == codes.Error; gotError != tt.wantError {
				t.Errorf("span status = %v, want error %v", client.Status(), tt.wantError)
			}
		})
	}
}

// TestTransportNetworkError checks that a failed request ends its span with
// an error.
func TestTransportNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	rec := tracetest.NewSpanRecorder()
	tracer := Tracer(NewProvider("test", sdktrace.WithSpanProcessor(rec)))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&Transport{Tracer: tracer}).RoundTrip(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("RoundTrip succeeded against a closed server")
	}
	if spans := rec.Ended(); len(spans) != 1 || spans[0].Status().Code != codes.Error {
		t.Errorf("want one ended span with an error status, got %d", len(spans))
	}
}

// TestProviderIgnoresOTELVariables checks that the standard variables can't
// change sampling: nbpdns has its own keys.
func TestProviderIgnoresOTELVariables(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "always_off")
	t.Setenv("OTEL_SERVICE_NAME", "someone-else")
	rec := tracetest.NewSpanRecorder()
	_, span := Tracer(NewProvider("test", sdktrace.WithSpanProcessor(rec))).Start(t.Context(), "s")
	span.End()
	spans := rec.Ended()
	if len(spans) != 1 || !spans[0].SpanContext().IsSampled() {
		t.Fatal("OTEL_TRACES_SAMPLER turned sampling off")
	}
	for _, a := range spans[0].Resource().Attributes() {
		if a.Key == "service.name" && a.Value.AsString() != "nbpdns" {
			t.Errorf("service.name = %s, want nbpdns", a.Value.AsString())
		}
	}
}
