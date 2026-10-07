package otlp

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/pem"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/tracing"
)

// received records what a collector was sent.
type received struct {
	mu      sync.Mutex
	paths   []string
	tokens  []string // each export's authorization header, or metadata
	spans   []string
	service string
}

func (r *received) add(path, token string, req *coltracepb.ExportTraceServiceRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paths = append(r.paths, path)
	r.tokens = append(r.tokens, token)
	for _, rs := range req.GetResourceSpans() {
		for _, a := range rs.GetResource().GetAttributes() {
			if a.GetKey() == "service.name" {
				r.service = a.GetValue().GetStringValue()
			}
		}
		for _, ss := range rs.GetScopeSpans() {
			for _, s := range ss.GetSpans() {
				r.spans = append(r.spans, s.GetName())
			}
		}
	}
}

// httpCollector returns an OTLP/HTTP collector, served over TLS if tls is
// true.
func httpCollector(t *testing.T, useTLS bool) (*httptest.Server, *received) {
	t.Helper()
	rec := &received{}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			body = gz
		}
		b, err := io.ReadAll(body)
		var req coltracepb.ExportTraceServiceRequest
		if err == nil {
			err = proto.Unmarshal(b, &req)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rec.add(r.URL.Path, r.Header.Get("Authorization"), &req)
		out, _ := proto.Marshal(&coltracepb.ExportTraceServiceResponse{})
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(out)
	}))
	if useTLS {
		srv.StartTLS()
	} else {
		srv.Start()
	}
	t.Cleanup(srv.Close)
	return srv, rec
}

// traceService is an OTLP/gRPC collector's trace service.
type traceService struct {
	coltracepb.UnimplementedTraceServiceServer
	rec *received
}

func (s *traceService) Export(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.rec.add("", strings.Join(md.Get("authorization"), ","), req)
	return &coltracepb.ExportTraceServiceResponse{}, nil
}

// grpcCollector returns the address of an OTLP/gRPC collector, served over
// TLS with cert if it isn't nil.
func grpcCollector(t *testing.T, cert *tls.Certificate) (string, *received) {
	t.Helper()
	lis, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var opts []grpc.ServerOption
	if cert != nil {
		opts = append(opts, grpc.Creds(credentials.NewServerTLSFromCert(cert)))
	}
	srv := grpc.NewServer(opts...)
	rec := &received{}
	coltracepb.RegisterTraceServiceServer(srv, &traceService{rec: rec})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String(), rec
}

// testCert returns a certificate for 127.0.0.1, and a CA file that trusts it.
func testCert(t *testing.T) (*tls.Certificate, string) {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.NotFoundHandler())
	srv.StartTLS()
	defer srv.Close()
	cert := srv.TLS.Certificates[0]
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return &cert, ca
}

// export sends one span, named refresh, through the exporter c configures,
// and returns what was logged. Its error is the first of building the
// exporter, shutting the provider down, and exporting, which the batcher
// reports to OpenTelemetry's error handler rather than to its caller.
func export(t *testing.T, c config.OTLPConfig) (string, error) {
	t.Helper()
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	c.Timeout = 2 * time.Second
	exp, err := NewExporter(t.Context(), c, log)
	if err != nil {
		return logs.String(), err
	}
	var (
		mu        sync.Mutex
		exportErr error
	)
	prev := otel.GetErrorHandler()
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		mu.Lock()
		defer mu.Unlock()
		exportErr = cmp.Or(exportErr, err)
	}))
	defer otel.SetErrorHandler(prev)
	tp := tracing.NewProvider("v1.2.3", sdktrace.WithBatcher(exp))
	_, span := tp.Tracer("test").Start(t.Context(), "refresh")
	span.End()
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	err = tp.Shutdown(ctx)
	mu.Lock()
	defer mu.Unlock()
	return logs.String(), cmp.Or(err, exportErr)
}

func TestExport(t *testing.T) {
	cert, ca := testCert(t)
	headers := config.NewSecret("Authorization=Bearer%20s3cret")
	t.Run("http/protobuf", func(t *testing.T) {
		srv, rec := httpCollector(t, false)
		logs, err := export(t, config.OTLPConfig{Endpoint: srv.URL + "/collector/", Protocol: config.OTLPHTTP, Headers: headers})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(rec.paths, []string{"/collector/v1/traces"}) || !reflect.DeepEqual(rec.tokens, []string{"Bearer s3cret"}) ||
			!reflect.DeepEqual(rec.spans, []string{"refresh"}) || rec.service != "nbpdns" {
			t.Errorf("received %+v", rec)
		}
		if !strings.Contains(logs, "uses http://") || strings.Contains(logs, "s3cret") {
			t.Errorf("logs:\n%s", logs)
		}
	})
	t.Run("http/protobuf over TLS", func(t *testing.T) {
		srv, rec := httpCollector(t, true)
		logs, err := export(t, config.OTLPConfig{Endpoint: srv.URL, Protocol: config.OTLPHTTP, CAFile: ca})
		if err != nil || !reflect.DeepEqual(rec.spans, []string{"refresh"}) || strings.Contains(logs, "http://") {
			t.Errorf("export: %v; received %+v; logs:\n%s", err, rec, logs)
		}
		// Without the CA file, the collector's certificate isn't trusted.
		srv2, rec2 := httpCollector(t, true)
		if _, err := export(t, config.OTLPConfig{Endpoint: srv2.URL, Protocol: config.OTLPHTTP}); err == nil || len(rec2.spans) != 0 {
			t.Errorf("export to an untrusted collector: %v, received %+v", err, rec2)
		}
	})
	t.Run("grpc", func(t *testing.T) {
		addr, rec := grpcCollector(t, nil)
		if _, err := export(t, config.OTLPConfig{Endpoint: "http://" + addr, Protocol: config.OTLPGRPC, Headers: headers}); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(rec.tokens, []string{"Bearer s3cret"}) || !reflect.DeepEqual(rec.spans, []string{"refresh"}) || rec.service != "nbpdns" {
			t.Errorf("received %+v", rec)
		}
	})
	t.Run("grpc over TLS", func(t *testing.T) {
		addr, rec := grpcCollector(t, cert)
		if _, err := export(t, config.OTLPConfig{Endpoint: "https://" + addr, Protocol: config.OTLPGRPC, CAFile: ca}); err != nil ||
			!reflect.DeepEqual(rec.spans, []string{"refresh"}) {
			t.Errorf("export: %v; received %+v", err, rec)
		}
		addr2, rec2 := grpcCollector(t, cert)
		if _, err := export(t, config.OTLPConfig{Endpoint: "https://" + addr2, Protocol: config.OTLPGRPC}); err == nil || len(rec2.spans) != 0 {
			t.Errorf("export to an untrusted collector: %v, received %+v", err, rec2)
		}
	})
	t.Run("a collector that redirects", func(t *testing.T) {
		// The headers mustn't follow the redirect.
		target, rec := httpCollector(t, false)
		redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect) //nolint:gosec // A collector that redirects is the test.
		}))
		defer redirect.Close()
		if _, err := export(t, config.OTLPConfig{Endpoint: redirect.URL, Protocol: config.OTLPHTTP, Headers: headers}); err == nil ||
			len(rec.paths) != 0 {
			t.Errorf("export through a redirect: %v; the target got %+v", err, rec)
		}
	})
	t.Run("no endpoint", func(t *testing.T) {
		if exp, err := NewExporter(t.Context(), config.OTLPConfig{}, slog.New(slog.DiscardHandler)); exp != nil || err != nil {
			t.Errorf("NewExporter() = %v, %v; want nothing", exp, err)
		}
	})
	t.Run("a bad header", func(t *testing.T) {
		_, err := NewExporter(t.Context(), config.OTLPConfig{Endpoint: "https://otel.example.com", Headers: config.NewSecret("s3cret")}, slog.New(slog.DiscardHandler))
		if err == nil || strings.Contains(err.Error(), "s3cret") {
			t.Errorf("NewExporter: %v", err)
		}
	})
	t.Run("a CA file that isn't there", func(t *testing.T) {
		_, err := NewExporter(t.Context(), config.OTLPConfig{Endpoint: "https://otel.example.com", CAFile: "/nonexistent/ca.pem"}, slog.New(slog.DiscardHandler))
		if err == nil || !strings.Contains(err.Error(), "otlp.ca_file") {
			t.Errorf("NewExporter: %v", err)
		}
	})
}
