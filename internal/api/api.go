// Package api serves nbpdns's read-only API at /api (ADR-0033). The API's
// source of truth is api/openapi.yaml: `make generate` writes its server
// code into package gen, and the copy of it that the binary serves at
// /api/openapi.yaml. The handlers map the service's last-known state onto
// the generated types. Every error is an RFC 9457 problem.
package api

import (
	"context"
	_ "embed"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api/gen"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
)

// spec is the API's OpenAPI document, the copy of api/openapi.yaml that
// `make generate` writes.
//
//go:embed openapi.yaml
var spec []byte

// Spec returns the API's OpenAPI document.
func Spec() []byte { return spec }

// A Source is the state the API serves: the service's.
type Source interface {
	Status() service.Status
	Groups() []service.GroupView
	NetBox() service.NetBoxView
}

// Options configure the API.
type Options struct {
	Source Source
	Log    *slog.Logger
	// Tracer starts each request's span. If nil, there are none.
	Tracer trace.Tracer
	// Metrics counts the requests. If nil, they aren't counted.
	Metrics *metrics.Metrics
	// PublicURL, server.public_url, is where clients reach the service, for
	// the API's absolute links. If empty, links use the request's host.
	PublicURL string
}

// base is the API's path, where the spec's server is.
const base = "/api"

// New returns the handler for /api and everything under it. Every response
// carries the request's X-Flow-ID, and Cache-Control: no-store, since it's
// the state as of the last refresh, which the next can change.
func New(o Options) http.Handler {
	if o.Tracer == nil {
		o.Tracer = noop.NewTracerProvider().Tracer("")
	}
	ops, err := operations(spec, base)
	if err != nil {
		// The spec is embedded, and the tests parse it.
		panic("the embedded OpenAPI document: " + err.Error())
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+base+"/openapi.yaml", serveSpec)
	strict := gen.NewStrictHandlerWithOptions(&server{o: o}, []gen.StrictMiddlewareFunc{withRequest}, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  badRequest,
		ResponseErrorHandlerFunc: o.failed,
	})
	gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
		BaseURL:          base,
		BaseRouter:       mux,
		ErrorHandlerFunc: badRequest,
	})
	return &handler{o: o, mux: mux, ops: ops}
}

// server implements the generated interface.
type server struct{ o Options }

var _ gen.StrictServerInterface = (*server)(nil)

// link returns the absolute URL of path, with query, as a client reaches it
// (Zalando [217]): under PublicURL, or else the request's host, over http.
func (o Options) link(r *http.Request, path string, query url.Values) string {
	root := "http://" + r.Host
	if o.PublicURL != "" {
		root = strings.TrimSuffix(o.PublicURL, "/")
	}
	u := root + path
	if q := query.Encode(); q != "" {
		u += "?" + q
	}
	return u
}

// serveSpec serves the API's OpenAPI document.
func serveSpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(spec)
}

// badRequest answers a request whose parameters the generated code
// couldn't bind, such as a limit that isn't a number.
func badRequest(w http.ResponseWriter, r *http.Request, err error) {
	writeProblem(w, r, http.StatusBadRequest, err.Error())
}

// failed answers a request whose handler failed. The error is logged, not
// returned: it may say more about nbpdns than a client should see.
func (o Options) failed(w http.ResponseWriter, r *http.Request, err error) {
	o.Log.ErrorContext(r.Context(), "an API request failed", "path", r.URL.Path, "error", err)
	writeProblem(w, r, http.StatusInternalServerError, "The request failed; nbpdns's log has why, under its X-Flow-ID.")
}

// problem returns an RFC 9457 problem of type about:blank, for status, with
// detail if it isn't empty.
func problem(r *http.Request, status int32, detail string) gen.Problem {
	p := gen.Problem{Type: "about:blank", Title: http.StatusText(int(status)), Status: status, Instance: &r.URL.Path}
	if detail != "" {
		p.Detail = &detail
	}
	return p
}

// writeProblem writes a problem response.
func writeProblem(w http.ResponseWriter, r *http.Request, status int32, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(int(status))
	_ = json.NewEncoder(w).Encode(problem(r, status, detail))
}

// optional returns s, or nil if it's empty: the API's null for "none".
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// GetStatus serves the service's status, without the groups, which have
// their own resource.
func (s *server) GetStatus(_ context.Context, _ gen.GetStatusRequestObject) (gen.GetStatusResponseObject, error) {
	st := s.o.Source.Status()
	sc := st.Schedule
	out := gen.Status{
		Version: st.Version, Revision: st.Revision, Started: st.Started,
		UptimeSeconds: st.UptimeSeconds, Ready: st.Ready, Live: st.Live,
		Schedule: gen.Schedule{
			IntervalSeconds: sc.IntervalSeconds, TimeoutSeconds: sc.TimeoutSeconds,
			LastCompleteRefresh: sc.LastCompleteRefresh, NextRefresh: sc.NextRefresh,
			Refreshes: gen.RefreshCounts{
				Complete: int64(sc.Refreshes.Complete), Incomplete: int64(sc.Refreshes.Incomplete),
				Failed: int64(sc.Refreshes.Failed),
			},
		},
		Netbox:  gen.NetBoxState{Url: st.NetBox.URL, Up: st.NetBox.Up, Error: optional(st.NetBox.Error)},
		Tracing: gen.Tracing{Exported: st.Tracing.Exported},
	}
	if r := sc.LastRefresh; r != nil {
		out.Schedule.LastRefresh = &gen.Refresh{
			Started: r.Started, Finished: r.Finished, DurationSeconds: r.DurationSeconds,
			Outcome: gen.RefreshOutcome(r.Outcome), Error: optional(r.Error),
		}
	}
	if st.Tracing.Exported {
		p := gen.OTLPProtocol(st.Tracing.Protocol)
		out.Tracing.Endpoint, out.Tracing.Protocol = optional(st.Tracing.Endpoint), &p
	}
	return gen.GetStatus200JSONResponse{Body: out}, nil
}
