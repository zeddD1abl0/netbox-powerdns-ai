package api

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// The API reference at /api/docs (ADR-0034): Scalar's standalone bundle,
// vendored gzipped by `make vendor-scalar`, its license, and nbpdns's own
// page and init script.
//
//go:embed docs/index.html docs/init.js docs/scalar.js.gz docs/LICENSE.scalar
var docsFS embed.FS

var docsPage = template.Must(template.ParseFS(docsFS, "docs/index.html"))

// csp is the Content-Security-Policy of the reference page and its assets:
// nothing from any other host, and no inline script. The style that Scalar
// adds carries the response's nonce.
func csp(nonce string) string {
	return strings.Join([]string{
		"default-src 'none'",
		"script-src 'self'",
		"style-src 'self' 'nonce-" + nonce + "'",
		"img-src 'self' data:",
		"font-src 'self' data:",
		"connect-src 'self'",
		"base-uri 'none'",
		"form-action 'none'",
		"frame-ancestors 'none'",
	}, "; ")
}

// secure sets the headers every docs response has: the policy, and no
// sniffing, framing or referrer.
func secure(w http.ResponseWriter, nonce string) {
	h := w.Header()
	h.Set("Content-Security-Policy", csp(nonce))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
}

// serveDocs serves the reference page, with a new nonce.
func serveDocs(w http.ResponseWriter, _ *http.Request) {
	nonce := rand.Text()
	secure(w, nonce)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = docsPage.Execute(w, struct{ Nonce string }{nonce})
}

// An asset is a file of the reference, with its ETag.
type asset struct {
	body, gzipped []byte
	etag          string
	ctype         string
}

var (
	assetsOnce sync.Once
	assets     map[string]*asset
)

// loadAssets reads the embedded assets, once.
func loadAssets() map[string]*asset {
	assetsOnce.Do(func() {
		assets = map[string]*asset{}
		gz, err := docsFS.ReadFile("docs/scalar.js.gz")
		if err != nil {
			panic(err) // Embedded.
		}
		zr, err := gzip.NewReader(bytes.NewReader(gz))
		if err != nil {
			panic(err)
		}
		js, err := io.ReadAll(zr)
		if err != nil {
			panic(err)
		}
		assets["scalar.js"] = newAsset(js, gz, "text/javascript; charset=utf-8")
		init, err := docsFS.ReadFile("docs/init.js")
		if err != nil {
			panic(err)
		}
		assets["init.js"] = newAsset(init, nil, "text/javascript; charset=utf-8")
		lic, err := docsFS.ReadFile("docs/LICENSE.scalar")
		if err != nil {
			panic(err)
		}
		assets["LICENSE.scalar"] = newAsset(lic, nil, "text/plain; charset=utf-8")
	})
	return assets
}

func newAsset(body, gzipped []byte, ctype string) *asset {
	sum := sha256.Sum256(body)
	return &asset{body: body, gzipped: gzipped, etag: `"` + hex.EncodeToString(sum[:8]) + `"`, ctype: ctype}
}

// serveAsset serves one of the reference's files, gzipped if it's stored so
// and the client accepts it. A client that has it revalidates with its
// ETag, rather than downloading it again.
func serveAsset(w http.ResponseWriter, r *http.Request) {
	a := loadAssets()[r.PathValue("file")]
	if a == nil {
		writeProblem(w, r, http.StatusNotFound, "The API reference has no "+r.URL.Path+".")
		return
	}
	secure(w, rand.Text())
	h := w.Header()
	h.Set("Cache-Control", "no-cache")
	h.Set("Content-Type", a.ctype)
	h.Set("Vary", "Accept-Encoding")
	body, etag := a.body, a.etag
	if a.gzipped != nil && acceptsGzip(r.Header.Get("Accept-Encoding")) {
		body, etag = a.gzipped, strings.TrimSuffix(a.etag, `"`)+`-gzip"`
		h.Set("Content-Encoding", "gzip")
	}
	h.Set("ETag", etag)
	if noneMatch(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(body)
}

// acceptsGzip reports whether an Accept-Encoding header accepts gzip (RFC
// 9110, section 12.5.3): it names gzip with a q-value above 0, or, if it
// doesn't name gzip, it names * with one.
func acceptsGzip(header string) bool {
	gzipQ, anyQ := -1.0, -1.0
	for part := range strings.SplitSeq(header, ",") {
		name, params, _ := strings.Cut(part, ";")
		q := 1.0
		for param := range strings.SplitSeq(params, ";") {
			if k, v, ok := strings.Cut(strings.TrimSpace(param), "="); ok && strings.EqualFold(k, "q") {
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					q = f
				}
			}
		}
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "gzip", "x-gzip":
			gzipQ = q
		case "*":
			anyQ = q
		}
	}
	if gzipQ >= 0 {
		return gzipQ > 0
	}
	return anyQ > 0
}

// noneMatch reports whether an If-None-Match header matches etag (RFC
// 9110, section 13.1.2): as *, or as one of its entity tags, compared
// weakly.
func noneMatch(header, etag string) bool {
	for tag := range strings.SplitSeq(header, ",") {
		tag = strings.TrimPrefix(strings.TrimSpace(tag), "W/")
		if tag == "*" || tag == etag {
			return true
		}
	}
	return false
}
