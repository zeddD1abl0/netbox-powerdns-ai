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
	if a.gzipped != nil && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		body, etag = a.gzipped, strings.TrimSuffix(a.etag, `"`)+`-gzip"`
		h.Set("Content-Encoding", "gzip")
	}
	h.Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(body)
}
