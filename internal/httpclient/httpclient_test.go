package httpclient

// These tests run the client against local HTTP servers that send only
// status codes, headers, delays and a small JSON body (ADR-0023). What each
// API answers is tested by the API clients, against the lab.

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/tracing"
)

const body = `{"ok":true}`

// A step is one response of a scripted server.
type step struct {
	status int
	header map[string]string
	delay  time.Duration
}

// scripted returns a server that answers its nth request with steps[n], or
// with the last step once they run out. A 200 carries body. It counts the
// requests it gets.
func scripted(t *testing.T, steps ...step) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := steps[min(int(n.Add(1)), len(steps))-1]
		select {
		case <-time.After(s.delay):
		case <-r.Context().Done():
			return
		}
		for k, v := range s.header {
			w.Header().Set(k, v)
		}
		w.WriteHeader(s.status)
		if s.status == http.StatusOK {
			_, _ = io.WriteString(w, body)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

// testClient returns a client with o's settings, whose retries record their
// delays instead of sleeping.
func testClient(t *testing.T, o Options) (*Client, *[]time.Duration) {
	t.Helper()
	var delays []time.Duration
	o.Service, o.Keys = "Test", "test"
	if o.Timeout == 0 {
		o.Timeout = 5 * time.Second
	}
	o.Retry = Retry{Attempts: 4, Base: 100 * time.Millisecond, Cap: time.Second,
		Sleep: func(_ context.Context, d time.Duration) error { delays = append(delays, d); return nil }}
	c, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c, &delays
}

// get fetches rawURL with c and checks the answer, if there's no error.
func get(t *testing.T, c *Client, rawURL string) error {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ OK bool }
	err = c.Get(t.Context(), u, http.Header{"Accept": {"application/json"}}, &out)
	if err == nil && !out.OK {
		t.Errorf("GET %s decoded %+v", rawURL, out)
	}
	return err
}

func TestRetries(t *testing.T) {
	tests := []struct {
		name       string
		steps      []step
		wantStatus int // of the StatusError, or 0 for success
		wantTries  int32
		wantDelays []time.Duration // exact delays; nil to check the backoff range
	}{
		{"503 then success", []step{{status: 503}, {status: 200}}, 0, 2, nil},
		{"each retried status", []step{{status: 429}, {status: 502}, {status: 504}, {status: 200}}, 0, 4, nil},
		{"gives up after four tries", []step{{status: 503}}, 503, 4, nil},
		{"500 isn't retried", []step{{status: 500}}, 500, 1, nil},
		{"401 isn't retried", []step{{status: 401}}, 401, 1, nil},
		{"Retry-After in seconds", []step{{status: 429, header: map[string]string{"Retry-After": "7"}}, {status: 200}},
			0, 2, []time.Duration{7 * time.Second}},
		{"Retry-After is capped", []step{{status: 503, header: map[string]string{"Retry-After": "3600"}}, {status: 200}},
			0, 2, []time.Duration{time.Minute}},
		{"redirects aren't followed", []step{{status: 302, header: map[string]string{"Location": "http://elsewhere.example/"}}},
			302, 1, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, tries := scripted(t, tt.steps...)
			c, delays := testClient(t, Options{URL: srv.URL})
			err := get(t, c, srv.URL+"/thing/?q=1")
			var se *StatusError
			switch {
			case tt.wantStatus == 0 && err != nil:
				t.Errorf("Get: %v", err)
			case tt.wantStatus != 0 && (!errors.As(err, &se) || se.Status != tt.wantStatus || se.Path != "/thing/?q=1"):
				t.Errorf("Get: %v, want a StatusError with status %d", err, tt.wantStatus)
			}
			if tries.Load() != tt.wantTries {
				t.Errorf("%d requests, want %d", tries.Load(), tt.wantTries)
			}
			if tt.wantDelays != nil && !slices.Equal(*delays, tt.wantDelays) {
				t.Errorf("delays %v, want %v", *delays, tt.wantDelays)
			}
			for i, d := range *delays {
				if tt.wantDelays == nil && (d < 50*time.Millisecond<<i || d > 100*time.Millisecond<<i) {
					t.Errorf("delay %d is %v, outside the backoff for its attempt", i+1, d)
				}
			}
			if se != nil && se.Status == 302 && se.Header.Get("Location") != "http://elsewhere.example/" {
				t.Errorf("redirect Location = %q", se.Header.Get("Location"))
			}
		})
	}

	t.Run("Retry-After as a date", func(t *testing.T) {
		soon := time.Now().Add(20 * time.Second).UTC().Format(http.TimeFormat)
		srv, _ := scripted(t, step{status: 503, header: map[string]string{"Retry-After": soon}}, step{status: 200})
		c, delays := testClient(t, Options{URL: srv.URL})
		if err := get(t, c, srv.URL); err != nil {
			t.Fatal(err)
		}
		if len(*delays) != 1 || (*delays)[0] < 15*time.Second || (*delays)[0] > 20*time.Second {
			t.Errorf("delays %v, want about 20s", *delays)
		}
	})
	t.Run("GetOnce doesn't retry", func(t *testing.T) {
		srv, tries := scripted(t, step{status: 503}, step{status: 200})
		c, _ := testClient(t, Options{URL: srv.URL})
		u, _ := url.Parse(srv.URL)
		var se *StatusError
		if err := c.GetOnce(t.Context(), u, nil, new(any)); !errors.As(err, &se) || se.Status != 503 || tries.Load() != 1 {
			t.Errorf("GetOnce after %d requests: %v, want a 503", tries.Load(), err)
		}
	})
}

func TestBackoff(t *testing.T) {
	for attempt := 1; attempt <= 8; attempt++ {
		want := min(DefaultRetry.Base<<(attempt-1), DefaultRetry.Cap)
		for range 100 {
			if d := DefaultRetry.delay(attempt, 0); d < want/2 || d > want {
				t.Fatalf("delay(%d) = %v, want %v to %v", attempt, d, want/2, want)
			}
		}
	}
}

func TestRetryAfterHeader(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		header string
		want   time.Duration
	}{
		{"", 0},
		{"0", 0},
		{"5", 5 * time.Second},
		{" 5 ", 5 * time.Second},
		{"-3", 0},
		{now.Add(90 * time.Second).Format(http.TimeFormat), 90 * time.Second},
		{now.Add(-time.Minute).Format(http.TimeFormat), 0},
		{"soon", 0},
	}
	for _, tt := range tests {
		if got := retryAfterHeader(tt.header, now); got != tt.want {
			t.Errorf("retryAfterHeader(%q) = %v, want %v", tt.header, got, tt.want)
		}
	}
}

// TestBodyFailureIsRetried stalls partway through the first answer's body,
// past the timeout, and checks the request is tried again.
func TestBodyFailureIsRetried(t *testing.T) {
	var tries atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tries.Add(1) == 1 {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			_, _ = io.WriteString(w, body[:len(body)/2])
			w.(http.Flusher).Flush()
			select {
			case <-time.After(time.Second):
			case <-r.Context().Done():
			}
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	c, delays := testClient(t, Options{URL: srv.URL, Timeout: 200 * time.Millisecond})
	if err := get(t, c, srv.URL); err != nil || tries.Load() != 2 || len(*delays) != 1 {
		t.Errorf("Get after %d requests: %v", tries.Load(), err)
	}
}

func TestBadAnswers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "not json")
	}))
	t.Cleanup(srv.Close)
	c, delays := testClient(t, Options{URL: srv.URL})
	if err := get(t, c, srv.URL+"/x"); err == nil || !strings.Contains(err.Error(), "decoding Test's answer to GET /x") || len(*delays) != 0 {
		t.Errorf("Get: %v, want a decoding error, not retried", err)
	}
}

func TestTimeout(t *testing.T) {
	t.Run("a slow answer is retried", func(t *testing.T) {
		srv, tries := scripted(t, step{status: 200, delay: time.Second}, step{status: 200})
		c, _ := testClient(t, Options{URL: srv.URL, Timeout: 100 * time.Millisecond})
		if err := get(t, c, srv.URL); err != nil || tries.Load() != 2 {
			t.Errorf("Get after %d requests: %v", tries.Load(), err)
		}
	})
	t.Run("always slow", func(t *testing.T) {
		srv, tries := scripted(t, step{status: 200, delay: time.Second})
		c, _ := testClient(t, Options{URL: srv.URL, Timeout: 50 * time.Millisecond})
		err := get(t, c, srv.URL)
		var ue *UnreachableError
		var ne net.Error
		if !errors.As(err, &ue) || !errors.As(err, &ne) || !ne.Timeout() || tries.Load() != 4 {
			t.Errorf("Get after %d requests: %v, want a timeout after 4", tries.Load(), err)
		}
		if ue != nil && (ue.Service != "Test" || ue.URL != srv.URL) {
			t.Errorf("UnreachableError names %s at %s", ue.Service, ue.URL)
		}
	})
	t.Run("nothing listening", func(t *testing.T) {
		l, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := l.Addr().String()
		_ = l.Close()
		c, delays := testClient(t, Options{URL: "http://" + addr})
		var ue *UnreachableError
		if err := get(t, c, "http://"+addr); !errors.As(err, &ue) || len(*delays) != 3 {
			t.Errorf("Get after %d retries: %v, want an UnreachableError after 3", len(*delays), err)
		}
	})
}

func TestCanceledContextStopsRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var tries atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		tries.Add(1)
		cancel()
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{Service: "Test", URL: srv.URL, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	u, _ := url.Parse(srv.URL)
	start := time.Now()
	if err := c.Get(ctx, u, nil, new(any)); err == nil {
		t.Error("Get succeeded")
	}
	if tries.Load() != 1 || time.Since(start) > 2*time.Second {
		t.Errorf("%d requests in %v; a canceled context should stop the retries", tries.Load(), time.Since(start))
	}
}

func TestHeaders(t *testing.T) {
	traceparent := regexp.MustCompile(`^00-[0-9a-f]{32}-[0-9a-f]{16}-01$`)
	for _, traced := range []bool{true, false} {
		var got http.Header
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = r.Header.Clone()
			_, _ = io.WriteString(w, body)
		}))
		t.Cleanup(srv.Close)
		o := Options{URL: srv.URL}
		if traced {
			tp := tracing.NewProvider("test")
			t.Cleanup(func() { _ = tp.Shutdown(context.WithoutCancel(t.Context())) })
			o.Tracer = tracing.Tracer(tp)
		}
		c, _ := testClient(t, o)
		u, _ := url.Parse(srv.URL)
		header := http.Header{"X-Api-Key": {"secret"}}
		if err := c.Get(t.Context(), u, header, new(any)); err != nil {
			t.Fatal(err)
		}
		if got.Get("X-Api-Key") != "secret" {
			t.Errorf("X-API-Key = %q", got.Get("X-Api-Key"))
		}
		if tp := got.Get("Traceparent"); traceparent.MatchString(tp) != traced {
			t.Errorf("traceparent = %q, want one: %v", tp, traced)
		}
		if header.Get("Traceparent") != "" {
			t.Error("Get changed the caller's header")
		}
	}
}

// TestNoCredentialsLogged makes requests fail every way the client logs, at
// debug level, and checks the credential header never appears.
func TestNoCredentialsLogged(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	for _, steps := range [][]step{{{status: 503}}, {{status: 403}}, {{status: 200, delay: time.Second}}} {
		srv, _ := scripted(t, steps...)
		c, _ := testClient(t, Options{URL: srv.URL, Logger: log, Timeout: 50 * time.Millisecond})
		u, _ := url.Parse(srv.URL)
		_ = c.Get(t.Context(), u, http.Header{"X-Api-Key": {"thesecretpartofthekey"}}, new(any))
	}
	if !strings.Contains(logs.String(), `"msg":"http request"`) || !strings.Contains(logs.String(), `"service":"Test"`) {
		t.Errorf("the requests weren't logged:\n%s", logs.String())
	}
	if strings.Contains(logs.String(), "thesecretpart") {
		t.Errorf("the key is in the log:\n%s", logs.String())
	}
}

// pki is a test CA, with a server and a client certificate it issued.
type pki struct {
	caFile               string
	server               tls.Certificate
	clientCert, clientKy string
	pool                 *x509.CertPool
}

func newPKI(t *testing.T) *pki {
	t.Helper()
	dir := t.TempDir()
	caKey := newKey(t)
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(caDER)
	issue := func(serial int64, tmpl *x509.Certificate) (certPEM, keyPEM []byte) {
		key := newKey(t)
		tmpl.SerialNumber = big.NewInt(serial)
		tmpl.NotBefore, tmpl.NotAfter = time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
		der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		keyDER, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
			pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	}
	srvCert, srvKey := issue(2, &x509.Certificate{
		Subject: pkix.Name{CommonName: "127.0.0.1"}, IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	cliCert, cliKey := issue(3, &x509.Certificate{
		Subject: pkix.Name{CommonName: "nbpdns"}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	server, err := tls.X509KeyPair(srvCert, srvKey)
	if err != nil {
		t.Fatal(err)
	}
	p := &pki{server: server, pool: x509.NewCertPool()}
	p.pool.AddCert(ca)
	p.caFile, p.clientCert, p.clientKy = filepath.Join(dir, "ca.pem"), filepath.Join(dir, "client.pem"), filepath.Join(dir, "client-key.pem")
	for path, b := range map[string][]byte{
		p.caFile: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), p.clientCert: cliCert, p.clientKy: cliKey,
	} {
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func newKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// tlsServer returns a TLS server with p's certificate and conf's settings.
func tlsServer(t *testing.T, p *pki, conf *tls.Config) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	// The servers' own logs of the failed handshakes are expected.
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	conf.Certificates = []tls.Certificate{p.server}
	srv.TLS = conf
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func TestTLS(t *testing.T) {
	p := newPKI(t)
	srv := tlsServer(t, p, &tls.Config{MinVersion: tls.VersionTLS12})

	t.Run("an untrusted certificate isn't retried", func(t *testing.T) {
		c, delays := testClient(t, Options{URL: srv.URL})
		err := get(t, c, srv.URL)
		var cve *tls.CertificateVerificationError
		if !errors.As(err, &cve) || len(*delays) != 0 {
			t.Errorf("Get after %d retries: %v, want a certificate error and no retry", len(*delays), err)
		}
	})
	t.Run("the CA file is trusted", func(t *testing.T) {
		c, _ := testClient(t, Options{URL: srv.URL, CAFile: p.caFile})
		if err := get(t, c, srv.URL); err != nil {
			t.Error(err)
		}
	})
	t.Run("TLS 1.1 is refused", func(t *testing.T) {
		old := tlsServer(t, p, &tls.Config{MinVersion: tls.VersionTLS10, MaxVersion: tls.VersionTLS11}) //nolint:gosec // The server nbpdns must refuse.
		c, delays := testClient(t, Options{URL: old.URL, CAFile: p.caFile})
		if err := get(t, c, old.URL); err == nil || len(*delays) != 0 {
			t.Errorf("Get after %d retries: %v, want a failure and no retry", len(*delays), err)
		}
	})
	t.Run("https:// to a plain HTTP server isn't retried", func(t *testing.T) {
		plain, _ := scripted(t, step{status: 200})
		https := strings.Replace(plain.URL, "http://", "https://", 1)
		c, delays := testClient(t, Options{URL: https})
		if err := get(t, c, https); err == nil || !strings.Contains(err.Error(), "HTTP response to HTTPS client") || len(*delays) != 0 {
			t.Errorf("Get after %d retries: %v, want a failure and no retry", len(*delays), err)
		}
	})
}

func TestClientCertificate(t *testing.T) {
	p := newPKI(t)
	srv := tlsServer(t, p, &tls.Config{MinVersion: tls.VersionTLS12, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: p.pool})

	t.Run("presented", func(t *testing.T) {
		c, _ := testClient(t, Options{URL: srv.URL, CAFile: p.caFile, CertFile: p.clientCert, KeyFile: p.clientKy})
		if err := get(t, c, srv.URL); err != nil {
			t.Error(err)
		}
	})
	t.Run("missing, and not retried", func(t *testing.T) {
		c, delays := testClient(t, Options{URL: srv.URL, CAFile: p.caFile})
		if err := get(t, c, srv.URL); err == nil || len(*delays) != 0 {
			t.Errorf("Get after %d retries: %v, want a failure and no retry", len(*delays), err)
		}
	})
}

func TestBadTLSFiles(t *testing.T) {
	p := newPKI(t)
	dir := t.TempDir()
	notPEM := filepath.Join(dir, "not.pem")
	if err := os.WriteFile(notPEM, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing.pem")
	tests := []struct {
		name string
		o    Options
		want string
	}{
		{"a CA file that isn't PEM", Options{CAFile: notPEM}, "test.ca_file: no PEM certificates"},
		{"a missing CA file", Options{CAFile: missing}, "test.ca_file:"},
		{"a certificate without its key", Options{CertFile: p.clientCert}, "test.cert_file and test.key_file go together"},
		{"a key without its certificate", Options{KeyFile: p.clientKy}, "test.cert_file and test.key_file go together"},
		{"a key that doesn't match", Options{CertFile: p.clientCert, KeyFile: notPEM}, "test.cert_file and test.key_file:"},
		{"a missing certificate", Options{CertFile: missing, KeyFile: p.clientKy}, "test.cert_file and test.key_file:"},
	}
	for _, tt := range tests {
		tt.o.Keys = "test"
		if _, err := New(tt.o); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: %v, want %q", tt.name, err, tt.want)
		}
	}
}

func TestTextDetail(t *testing.T) {
	long := strings.Repeat("é", 150) // 300 bytes, 2 to a character
	tests := []struct{ body, want string }{
		{"Not Found\n", "Not Found"},
		{"two\n  lines", "two lines"},
		{"bad \xff byte", "bad byte"},
		{"<html><head><TITLE> 502 Bad Gateway </TITLE></head><body>...</body></html>", "502 Bad Gateway"},
		{long, long[:200]},
		{"x" + long, ("x" + long)[:199]},
	}
	for _, tt := range tests {
		if got := TextDetail([]byte(tt.body)); got != tt.want {
			t.Errorf("TextDetail(%.20q) = %.20q, want %.20q", tt.body, got, tt.want)
		}
	}
}
