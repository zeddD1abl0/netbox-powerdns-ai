package api

import (
	"bytes"
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// docsGet requests path from the API, with header, and returns the reply.
func docsGet(t *testing.T, path string, header http.Header) reply {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	New(Options{Source: fakeSource{}, Log: slog.New(slog.DiscardHandler)}).ServeHTTP(rec, req)
	return reply{rec.Code, rec.Header(), rec.Body.Bytes()}
}

var nonceRE = regexp.MustCompile(`'nonce-([A-Z2-7]+)'`)

func TestDocsPage(t *testing.T) {
	r := docsGet(t, "/api/docs", nil)
	if r.code != http.StatusOK || r.header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("%d %s", r.code, r.header.Get("Content-Type"))
	}
	policy := r.header.Get("Content-Security-Policy")
	m := nonceRE.FindStringSubmatch(policy)
	if m == nil || !strings.Contains(string(r.body), `<meta property="csp-nonce" content="`+m[1]+`">`) {
		t.Fatalf("the page's nonce isn't the policy's:\n%s\n%s", policy, r.body)
	}
	for _, want := range []string{"default-src 'none'", "script-src 'self'", "connect-src 'self'", "font-src 'self' data:", "frame-ancestors 'none'"} {
		if !strings.Contains(policy, want) {
			t.Errorf("the policy has no %q: %s", want, policy)
		}
	}
	if strings.Contains(policy, "unsafe") {
		t.Errorf("the policy allows something unsafe: %s", policy)
	}
	if r.header.Get("X-Content-Type-Options") != "nosniff" || r.header.Get("Cache-Control") != "no-store" {
		t.Errorf("headers %v", r.header)
	}
	// Every URL in the page is on this origin.
	if strings.Contains(string(r.body), "//") {
		t.Errorf("the page names another host:\n%s", r.body)
	}
	// Each response has its own nonce.
	if again := nonceRE.FindStringSubmatch(docsGet(t, "/api/docs", nil).header.Get("Content-Security-Policy")); again[1] == m[1] {
		t.Error("two responses had the same nonce")
	}
}

func TestDocsAssets(t *testing.T) {
	plain := docsGet(t, "/api/docs/scalar.js", nil)
	if plain.code != http.StatusOK || plain.header.Get("Content-Encoding") != "" || !bytes.Contains(plain.body, []byte("createApiReference")) ||
		plain.header.Get("Content-Type") != "text/javascript; charset=utf-8" || plain.header.Get("Content-Security-Policy") == "" {
		t.Fatalf("scalar.js: %d %v, %d bytes", plain.code, plain.header, len(plain.body))
	}
	gz := docsGet(t, "/api/docs/scalar.js", http.Header{"Accept-Encoding": {"gzip, br"}})
	zr, err := gzip.NewReader(bytes.NewReader(gz.body))
	if err != nil || gz.header.Get("Content-Encoding") != "gzip" || gz.header.Get("Vary") != "Accept-Encoding" {
		t.Fatalf("gzipped scalar.js: %v, %v", err, gz.header)
	}
	if body, err := io.ReadAll(zr); err != nil || !bytes.Equal(body, plain.body) {
		t.Errorf("gzipped scalar.js isn't scalar.js: %v", err)
	}
	if gz.header.Get("ETag") == plain.header.Get("ETag") {
		t.Error("the two encodings share an ETag")
	}
	// A client that has it revalidates.
	if r := docsGet(t, "/api/docs/scalar.js", http.Header{"If-None-Match": {plain.header.Get("ETag")}}); r.code != http.StatusNotModified || len(r.body) != 0 {
		t.Errorf("revalidation: %d, %d bytes", r.code, len(r.body))
	}
	// The init script reads the API's own document, and turns off what
	// would reach Scalar's hosts.
	init := string(docsGet(t, "/api/docs/init.js", nil).body)
	for _, want := range []string{`url: "openapi.yaml"`, "withDefaultFonts: false", "agent: { disabled: true }", "mcp: { disabled: true }", "telemetry: false"} {
		if !strings.Contains(init, want) {
			t.Errorf("init.js has no %q", want)
		}
	}
	if lic := docsGet(t, "/api/docs/LICENSE.scalar", nil); lic.code != http.StatusOK || !strings.HasPrefix(string(lic.body), "MIT License") {
		t.Errorf("the license: %d %.40s", lic.code, lic.body)
	}
	for _, path := range []string{"/api/docs/nope.js", "/api/docs/"} {
		problemOf(t, docsGet(t, path, nil), http.StatusNotFound)
	}
}

func TestAcceptsGzip(t *testing.T) {
	tests := []struct {
		header string
		want   bool
	}{
		{"", false},
		{"gzip", true},
		{"gzip, deflate, br", true},
		{"br;q=1.0, gzip;q=0.8", true},
		{"gzip;q=0", false},
		{"gzip;q=0, identity", false},
		{"GZIP", true},
		{"*", true},
		{"*;q=0", false},
		{"*, gzip;q=0", false},
		{"identity", false},
		{"x-gzip", true},
	}
	for _, tt := range tests {
		if got := acceptsGzip(tt.header); got != tt.want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", tt.header, got, tt.want)
		}
	}
}

func TestNoneMatch(t *testing.T) {
	const etag = `"abc-gzip"`
	tests := []struct {
		header string
		want   bool
	}{
		{"", false},
		{`"abc-gzip"`, true},
		{`W/"abc-gzip"`, true},
		{`"x", "abc-gzip"`, true},
		{`"x",W/"abc-gzip"`, true},
		{"*", true},
		{`"abc"`, false},
	}
	for _, tt := range tests {
		if got := noneMatch(tt.header, etag); got != tt.want {
			t.Errorf("noneMatch(%q) = %v, want %v", tt.header, got, tt.want)
		}
	}
}
