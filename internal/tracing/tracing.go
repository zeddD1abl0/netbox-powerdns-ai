// Package tracing gives each run of nbpdns OpenTelemetry trace context.
// Spans get real trace and span IDs, for the logs and for the W3C
// traceparent header on outgoing requests. internal/otlp exports them, when
// nbpdns's own otlp keys say where. The standard OTEL_ environment
// variables aren't a supported interface, so the provider sets its sampler
// and resource itself.
package tracing

import (
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// scope names nbpdns's instrumentation.
const scope = "github.com/zeddD1abl0/netbox-powerdns-ai"

// NewProvider returns a tracer provider for one run of nbpdns, at version.
// Shut it down when the run ends.
func NewProvider(version string, opts ...sdktrace.TracerProviderOption) *sdktrace.TracerProvider {
	opts = append([]sdktrace.TracerProviderOption{
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
		sdktrace.WithResource(resource.NewSchemaless(
			attribute.String("service.name", "nbpdns"),
			attribute.String("service.version", version),
		)),
	}, opts...)
	return sdktrace.NewTracerProvider(opts...)
}

// Tracer returns nbpdns's tracer from tp.
func Tracer(tp trace.TracerProvider) trace.Tracer { return tp.Tracer(scope) }

// Transport is an http.RoundTripper that runs each request in a client span,
// and sends the span's context in the W3C traceparent header, so the server
// can join the trace. It changes a copy of the request, never the original.
type Transport struct {
	// Base sends the requests. If nil, http.DefaultTransport does.
	Base http.RoundTripper
	// Tracer starts the spans.
	Tracer trace.Tracer
}

// RoundTrip sends req in a client span.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, span := t.Tracer.Start(req.Context(), req.Method,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("http.request.method", req.Method),
			attribute.String("server.address", req.URL.Hostname()),
			attribute.String("url.full", req.URL.String()),
		))
	defer span.End()

	out := req.Clone(ctx)
	propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(out.Header))
	resp, err := t.base().RoundTrip(out)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	if resp.StatusCode >= http.StatusInternalServerError {
		span.SetStatus(codes.Error, resp.Status)
	}
	return resp, nil
}

// CloseIdleConnections closes the idle connections of the transport that
// sends the requests, if it keeps any, so http.Client.CloseIdleConnections
// reaches it.
func (t *Transport) CloseIdleConnections() {
	if c, ok := t.base().(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

func (t *Transport) base() http.RoundTripper {
	if t.Base == nil {
		return http.DefaultTransport
	}
	return t.Base
}
