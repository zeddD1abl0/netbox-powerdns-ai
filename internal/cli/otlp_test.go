package cli

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

// TestCommandsExportSpans runs a command with otlp.endpoint set, and checks
// that its root span reaches the collector before it returns, and that
// config show exports nothing.
func TestCommandsExportSpans(t *testing.T) {
	var (
		mu    sync.Mutex
		spans []string
		auth  []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		b, err := io.ReadAll(gz)
		var req coltracepb.ExportTraceServiceRequest
		if err == nil {
			err = proto.Unmarshal(b, &req)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		auth = append(auth, r.Header.Get("Authorization"))
		for _, rs := range req.GetResourceSpans() {
			for _, ss := range rs.GetScopeSpans() {
				for _, s := range ss.GetSpans() {
					spans = append(spans, s.GetName())
				}
			}
		}
		out, _ := proto.Marshal(&coltracepb.ExportTraceServiceResponse{})
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	env := map[string]string{
		"NBPDNS_OTLP_ENDPOINT": srv.URL, "NBPDNS_OTLP_HEADERS": "Authorization=Bearer%20s3cret",
		// NetBox refuses connections, which still makes a trace.
		"NBPDNS_NETBOX_URL": "http://127.0.0.1:1", "NBPDNS_NETBOX_TOKEN": "nbt_abc.def",
	}
	code, _, stderr := run(t, env, "netbox", "check")
	if code != exitError {
		t.Fatalf("exit %d:\n%s", code, stderr)
	}
	mu.Lock()
	got, gotAuth := slices.Clone(spans), slices.Clone(auth)
	mu.Unlock()
	if !slices.Contains(got, "nbpdns netbox check") || !slices.Contains(gotAuth, "Bearer s3cret") {
		t.Errorf("the collector got spans %v, with authorization %v", got, gotAuth)
	}
	warned := false
	for l := range strings.Lines(stderr) {
		warned = warned || strings.Contains(l, "the OTLP endpoint uses http://") && strings.Contains(l, `"request_id":`) && strings.Contains(l, `"trace_id":`)
	}
	if !warned {
		t.Errorf("no warning about http://, with the command's IDs:\n%s", stderr)
	}

	// config show exports nothing, so the otlp keys can't stop it, even a
	// CA file that isn't there.
	env["NBPDNS_OTLP_CA_FILE"] = "/nonexistent/ca.pem"
	code, out, stderr := run(t, env, "config", "show")
	if code != exitOK {
		t.Fatalf("config show: exit %d:\n%s", code, stderr)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(spans) != len(got) {
		t.Errorf("config show exported spans: %v", spans[len(got):])
	}
	if strings.Contains(out+stderr, "s3cret") || !strings.Contains(out, "otlp.headers") {
		t.Errorf("the headers aren't redacted:\n%s", out)
	}
}
