// Package otlp exports nbpdns's spans to an OpenTelemetry collector, over
// OTLP/HTTP or OTLP/gRPC (ADR-0029). nbpdns's own keys configure it. The
// exporters would also read the OTEL_EXPORTER_OTLP_ environment variables,
// which aren't a supported interface, so every option they'd read from them
// is set here.
package otlp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/encoding/gzip" // Registers gzip for the gRPC exporter's compression.

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/httpclient"
)

// tracesPath is where an OTLP/HTTP collector takes spans, under its base URL.
const tracesPath = "/v1/traces"

// NewExporter returns the span exporter that c configures, or nil if c sets
// no endpoint. It logs a warning if the endpoint uses http://.
func NewExporter(ctx context.Context, c config.OTLPConfig, log *slog.Logger) (sdktrace.SpanExporter, error) {
	if c.Endpoint == "" {
		return nil, nil //nolint:nilnil // No endpoint means no exporter, which isn't an error.
	}
	u, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("otlp.endpoint: %w", err)
	}
	headers, err := config.ParseHeaders(c.Headers.Reveal())
	if err != nil {
		return nil, fmt.Errorf("otlp.headers: %w", err)
	}
	plain := u.Scheme == "http"
	if plain {
		log.WarnContext(ctx, "the OTLP endpoint uses http://, so the spans, and any otlp.headers such as a token, "+
			"cross the network unencrypted; use https://", "url", u.String())
	}
	if c.Protocol == config.OTLPGRPC {
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpointURL(u.String()),
			otlptracegrpc.WithHeaders(headers),
			otlptracegrpc.WithTimeout(c.Timeout),
			otlptracegrpc.WithCompressor("gzip"),
		}
		if plain {
			opts = append(opts, otlptracegrpc.WithInsecure())
		} else {
			tlsConf, err := httpclient.TLSConfig(httpclient.Options{Keys: "otlp", CAFile: c.CAFile})
			if err != nil {
				return nil, err
			}
			opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(tlsConf)))
		}
		return otlptracegrpc.New(ctx, opts...)
	}
	endpoint := *u
	endpoint.Path = strings.TrimSuffix(u.Path, "/") + tracesPath
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpointURL(endpoint.String()),
		otlptracehttp.WithHeaders(headers),
		otlptracehttp.WithTimeout(c.Timeout),
		otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
	}
	// nbpdns's own client, which, like its others, never follows a
	// redirect: one would carry the headers wherever it points. Supplying
	// it overrides the exporter's TLS and timeout options, so it carries
	// both.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if plain {
		opts = append(opts, otlptracehttp.WithInsecure())
	} else {
		tlsConf, err := httpclient.TLSConfig(httpclient.Options{Keys: "otlp", CAFile: c.CAFile})
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig = tlsConf
	}
	opts = append(opts, otlptracehttp.WithHTTPClient(&http.Client{
		Transport:     transport,
		Timeout:       c.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}))
	return otlptracehttp.New(ctx, opts...)
}
