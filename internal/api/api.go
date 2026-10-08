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

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api/gen"
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
}

// Options configure the API.
type Options struct {
	Source Source
	Log    *slog.Logger
}

// New returns the handler for /api and everything under it.
func New(o Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/openapi.yaml", serveSpec)
	strict := gen.NewStrictHandlerWithOptions(&server{o: o}, nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  badRequest,
		ResponseErrorHandlerFunc: o.failed,
	})
	gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
		BaseURL:          "/api",
		BaseRouter:       mux,
		ErrorHandlerFunc: badRequest,
	})
	return noStore(mux)
}

// server implements the generated interface.
type server struct{ o Options }

var _ gen.StrictServerInterface = (*server)(nil)

// noStore marks every response as one not to cache: it's the state as of
// the last refresh, which the next can change.
func noStore(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
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
	writeProblem(w, r, http.StatusInternalServerError, "")
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
