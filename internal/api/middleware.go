package api

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.yaml.in/yaml/v3"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/logging"
)

// flowIDRE matches a valid X-Flow-ID, as api/openapi.yaml defines it.
var flowIDRE = regexp.MustCompile(`^[A-Za-z0-9._:+/=-]{1,128}$`)

// The operation label of a request that isn't one of the spec's operations.
const (
	// opSpec is a request for the OpenAPI document itself.
	opSpec = "openapi"
	// opDocs is a request for the API reference, or one of its files.
	opDocs = "docs"
	// opUnmatched is a request for no operation: an unknown path or method.
	opUnmatched = "unmatched"
)

// handler wraps the API's mux with what every request shares (ADR-0033):
// its flow ID, its span, its problem for an unknown path or method, its log
// line, and its metrics.
type handler struct {
	o   Options
	mux *http.ServeMux
	// ops maps each route, as the mux's pattern names it, such as
	// "GET /api/status", to its operationId in the spec.
	ops map[string]string
}

// operations returns the operationId of each route in the OpenAPI document
// spec, whose paths start at base.
func operations(spec []byte, base string) (map[string]string, error) {
	var doc struct {
		Paths map[string]map[string]struct {
			OperationID string `yaml:"operationId"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(spec, &doc); err != nil {
		return nil, err
	}
	ops := map[string]string{}
	for path, methods := range doc.Paths {
		for method, op := range methods {
			if op.OperationID != "" {
				ops[strings.ToUpper(method)+" "+base+path] = op.OperationID
			}
		}
	}
	return ops, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	id := r.Header.Get("X-Flow-ID")
	if !flowIDRE.MatchString(id) {
		id = rand.Text()
	}
	ctx := logging.WithRequestID(r.Context(), id)
	ctx = propagation.TraceContext{}.Extract(ctx, propagation.HeaderCarrier(r.Header))
	r = r.WithContext(ctx)
	// The handlers see the flow ID the response returns.
	r.Header = r.Header.Clone()
	r.Header.Set("X-Flow-ID", id)

	next, pattern := h.mux.Handler(r)
	op, route := opUnmatched, ""
	if pattern != "" {
		// A GET pattern answers HEAD too, so the route is the pattern without
		// whatever method it names.
		_, route, _ = strings.Cut(pattern, " ")
		op = h.ops[pattern]
		switch {
		case op != "":
		case strings.HasPrefix(route, base+"/docs"):
			op = opDocs
		default:
			op = opSpec
		}
	}
	name := r.Method
	if route != "" {
		name += " " + route
	}
	ctx, span := h.o.Tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("http.request.method", r.Method),
			attribute.String("url.path", r.URL.Path),
		))
	defer span.End()
	if route != "" {
		span.SetAttributes(attribute.String("http.route", route))
	}
	r = r.WithContext(ctx)

	w.Header().Set("X-Flow-ID", id)
	w.Header().Set("Cache-Control", "no-store")
	sw := &statusWriter{ResponseWriter: w}
	if pattern == "" {
		unmatched(sw, r, next)
	} else {
		// Through the mux, not next itself, which sets the path's values.
		h.mux.ServeHTTP(sw, r)
	}

	code := sw.status()
	d := time.Since(start)
	span.SetAttributes(attribute.Int("http.response.status_code", code))
	if code >= http.StatusInternalServerError {
		span.SetStatus(codes.Error, http.StatusText(code))
	}
	if h.o.Metrics != nil {
		h.o.Metrics.APIRequest(op, code, d)
	}
	h.o.Log.InfoContext(ctx, "API request", "method", r.Method, "path", r.URL.Path,
		"operation", op, "status", code, "duration_seconds", d.Seconds())
}

// unmatched answers a request that matched no route: with a problem, for a
// path that the API doesn't have, or a method that it doesn't answer on the
// path. Anything else, such as the mux's redirect to a cleaned path, next
// answers itself.
func unmatched(w http.ResponseWriter, r *http.Request, next http.Handler) {
	probe := &probeWriter{header: http.Header{}}
	next.ServeHTTP(probe, r)
	switch probe.code {
	case http.StatusNotFound:
		writeProblem(w, r, http.StatusNotFound, "The API has no "+r.URL.Path+".")
	case http.StatusMethodNotAllowed:
		w.Header().Set("Allow", probe.header.Get("Allow"))
		writeProblem(w, r, http.StatusMethodNotAllowed,
			fmt.Sprintf("The API only reads: %s answers %s, not %s.", r.URL.Path, probe.header.Get("Allow"), r.Method))
	default:
		next.ServeHTTP(w, r)
	}
}

// statusWriter records the status code a handler writes.
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (s *statusWriter) WriteHeader(code int) {
	if s.code == 0 {
		s.code = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if s.code == 0 {
		s.code = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the writer underneath.
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// status returns the code written, or 200 if the handler wrote nothing.
func (s *statusWriter) status() int {
	if s.code == 0 {
		return http.StatusOK
	}
	return s.code
}

// probeWriter keeps the status and header a handler writes, and drops the
// body.
type probeWriter struct {
	header http.Header
	code   int
}

func (p *probeWriter) Header() http.Header { return p.header }

func (p *probeWriter) WriteHeader(code int) {
	if p.code == 0 {
		p.code = code
	}
}

func (p *probeWriter) Write(b []byte) (int, error) {
	if p.code == 0 {
		p.code = http.StatusOK
	}
	return len(b), nil
}
