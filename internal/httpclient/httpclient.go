// Package httpclient is the HTTP client that nbpdns's API clients share
// (ADR-0023, ADR-0026). It verifies TLS 1.2 or later against the system's
// roots and an optional CA file, can present a client certificate, never
// follows a redirect, sends W3C traceparent, and retries GET requests with
// capped, jittered backoff that honors Retry-After. Each API client adds its
// own headers, endpoints and errors.
package httpclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/tracing"
)

const (
	// MaxBody bounds how much of a successful answer is read.
	MaxBody = 64 << 20
	// maxErrorBody bounds how much of an error answer is kept.
	maxErrorBody = 64 << 10
	// maxRetryAfter caps how long a Retry-After header can make a client wait.
	maxRetryAfter = time.Minute
)

// Options configure a Client.
type Options struct {
	// Service names the server in errors and logs, such as NetBox.
	Service string
	// URL is the server's base URL, as errors show it.
	URL string
	// Keys is the configuration path that holds the TLS files' keys, such as
	// netbox, so that errors name netbox.ca_file.
	Keys string
	// CAFile is a PEM file of CA certificates to trust, as well as the
	// system's.
	CAFile string
	// CertFile and KeyFile are a PEM client certificate and its key, to
	// present when the server asks for one. They're set together or not at
	// all.
	CertFile, KeyFile string
	// Timeout bounds each request, its body included.
	Timeout time.Duration
	// Conns is how many idle connections to keep to the server.
	Conns  int
	Logger *slog.Logger
	// Tracer, if set, runs each request in a client span and sends its
	// traceparent.
	Tracer trace.Tracer
	// Retry is the retry policy, or DefaultRetry if zero.
	Retry Retry
	// Observer, if set, is told of every attempt and retry, for metrics.
	Observer Observer
}

// An Observer is told of each request a Client makes, for metrics.
type Observer interface {
	// Request reports one attempt: its method, the answer's status code, or
	// 0 if no answer came, and how long the answer took to start.
	Request(method string, code int, d time.Duration)
	// Retry reports that a request is about to be tried again.
	Retry()
}

// nopObserver is the Observer of a Client that has none.
type nopObserver struct{}

func (nopObserver) Request(string, int, time.Duration) {}
func (nopObserver) Retry()                             {}

// A Client sends GET requests to one server. It's safe for concurrent use.
type Client struct {
	service string
	url     string
	http    *http.Client
	log     *slog.Logger
	retry   Retry
	obs     Observer
}

// New returns a Client.
func New(o Options) (*Client, error) {
	tlsConf, err := TLSConfig(o)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConf
	transport.MaxIdleConnsPerHost = max(o.Conns, 2)
	var rt http.RoundTripper = transport
	if o.Tracer != nil {
		rt = &tracing.Transport{Base: transport, Tracer: o.Tracer}
	}
	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	var obs Observer = nopObserver{}
	if o.Observer != nil {
		obs = o.Observer
	}
	c := &Client{
		service: o.Service,
		url:     o.URL,
		log:     log,
		retry:   o.Retry,
		obs:     obs,
		http: &http.Client{
			Timeout:   o.Timeout,
			Transport: rt,
			// Never follow a redirect: it would carry the credentials
			// somewhere the configuration didn't name. Report it instead.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	if c.retry.Attempts == 0 {
		c.retry = DefaultRetry
	}
	return c, nil
}

// Close closes the client's idle connections. Call it when done with the
// client.
func (c *Client) Close() { c.http.CloseIdleConnections() }

// TLSConfig returns the TLS configuration of a client with o's files: TLS 1.2
// or later, trusting the system's roots, plus the certificates in o.CAFile,
// and presenting o's client certificate, if it has one.
func TLSConfig(o Options) (*tls.Config, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if o.CAFile != "" {
		pem, err := os.ReadFile(o.CAFile)
		if err != nil {
			return nil, fmt.Errorf("%s.ca_file: %w", o.Keys, err)
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("%s.ca_file: no PEM certificates in %s", o.Keys, o.CAFile)
		}
	}
	conf := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}
	switch {
	case o.CertFile == "" && o.KeyFile == "":
	case o.CertFile == "" || o.KeyFile == "":
		return nil, fmt.Errorf("%s.cert_file and %s.key_file go together; set both, or neither", o.Keys, o.Keys)
	default:
		cert, err := tls.LoadX509KeyPair(o.CertFile, o.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("%s.cert_file and %s.key_file: %w", o.Keys, o.Keys, err)
		}
		conf.Certificates = []tls.Certificate{cert}
	}
	return conf, nil
}

// UnreachableError means a server couldn't be reached, or didn't answer in
// time, after every retry.
type UnreachableError struct {
	// Service names the server, such as NetBox.
	Service string
	// URL is the server's base URL.
	URL string
	Err error
}

func (e *UnreachableError) Error() string {
	return fmt.Sprintf("%s at %s isn't reachable: %v", e.Service, e.URL, e.Err)
}

func (e *UnreachableError) Unwrap() error { return e.Err }

// StatusError is an answer other than 200 OK, after any retries. Each API
// client turns it into errors of its own.
type StatusError struct {
	// Service names the server, such as NetBox.
	Service string
	Status  int
	// Path is the request's path and query.
	Path   string
	Header http.Header
	// Body is the start of the answer, up to 64 KiB.
	Body []byte
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s answered GET %s with status %d", e.Service, e.Path, e.Status)
}

// retryableError marks a StatusError that may succeed if retried.
type retryableError struct{ *StatusError }

func (e retryableError) Unwrap() error { return e.StatusError }

// Get fetches u with header and decodes its JSON into out, retrying as the
// policy says. A failure is an *UnreachableError, a *StatusError, or an
// error decoding the answer.
func (c *Client) Get(ctx context.Context, u *url.URL, header http.Header, out any) error {
	for attempt := 1; ; attempt++ {
		retryAfter, err := c.try(ctx, u, header, out, attempt)
		if err == nil {
			return nil
		}
		if attempt >= c.retry.Attempts || !retryable(ctx, err) {
			return unmark(err)
		}
		d := c.retry.delay(attempt, retryAfter)
		c.obs.Retry()
		c.log.DebugContext(ctx, "retrying an http request", "service", c.service, "path", u.RequestURI(),
			"attempt", attempt, "delay_seconds", d.Seconds(), "err", err)
		if serr := c.retry.sleep(ctx, d); serr != nil {
			return unmark(err)
		}
	}
}

// GetOnce is Get without retries.
func (c *Client) GetOnce(ctx context.Context, u *url.URL, header http.Header, out any) error {
	_, err := c.try(ctx, u, header, out, 1)
	return unmark(err)
}

func unmark(err error) error {
	var re retryableError
	if errors.As(err, &re) {
		return re.StatusError
	}
	return err
}

// try makes one attempt at fetching u. It returns the wait a Retry-After
// header asked for, if any.
func (c *Client) try(ctx context.Context, u *url.URL, header http.Header, out any, attempt int) (time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return 0, err
	}
	req.Header = header.Clone()
	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		c.obs.Request(http.MethodGet, 0, time.Since(start))
		c.log.DebugContext(ctx, "http request failed", "service", c.service, "path", u.RequestURI(), "attempt", attempt, "err", err)
		return 0, &UnreachableError{Service: c.service, URL: c.url, Err: err}
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, MaxBody))
		_ = resp.Body.Close()
	}()
	c.obs.Request(http.MethodGet, resp.StatusCode, time.Since(start))
	c.log.DebugContext(ctx, "http request", "service", c.service, "path", u.RequestURI(), "status", resp.StatusCode,
		"attempt", attempt, "duration_seconds", time.Since(start).Seconds())

	if resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody+1))
		if err != nil {
			// The connection failed, or timed out, partway through the
			// answer, which a retry may get whole.
			return 0, &UnreachableError{Service: c.service, URL: c.url, Err: err}
		}
		if len(body) > MaxBody {
			return 0, fmt.Errorf("%s's answer to GET %s is larger than %d MiB", c.service, u.RequestURI(), MaxBody>>20)
		}
		if err := json.Unmarshal(body, out); err != nil {
			return 0, fmt.Errorf("decoding %s's answer to GET %s: %w", c.service, u.RequestURI(), err)
		}
		return 0, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	se := &StatusError{Service: c.service, Status: resp.StatusCode, Path: u.RequestURI(), Header: resp.Header, Body: body}
	switch resp.StatusCode {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return retryAfterHeader(resp.Header.Get("Retry-After"), time.Now()), retryableError{se}
	}
	return 0, se
}

// TextDetail returns an error answer's text for an error message: an HTML
// page's title, such as a proxy's error page has, or else the text itself,
// on one line, up to 200 bytes.
func TextDetail(body []byte) string {
	s := strings.Join(strings.Fields(strings.ToValidUTF8(string(body), "")), " ")
	// Only ASCII letters are made lowercase, so lower has s's byte offsets.
	lower := strings.Map(func(r rune) rune {
		if 'A' <= r && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}, s)
	if i, j := strings.Index(lower, "<title>"), strings.Index(lower, "</title>"); i >= 0 && j > i {
		s = strings.TrimSpace(s[i+len("<title>") : j])
	}
	if len(s) > 200 {
		s = s[:200]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}

// A Retry says how often, and after how long, to retry a request.
type Retry struct {
	// Attempts counts every try, the first included.
	Attempts int
	// Base is the first delay, and Cap the most any delay may be.
	Base, Cap time.Duration
	// Sleep waits, unless the context ends first. If nil, it sleeps.
	Sleep func(ctx context.Context, d time.Duration) error
}

// DefaultRetry is the policy of every client that sets none.
var DefaultRetry = Retry{Attempts: 4, Base: 500 * time.Millisecond, Cap: 10 * time.Second}

// delay returns how long to wait before the next attempt: what Retry-After
// asked for, up to a minute, or else exponential backoff with jitter.
func (r Retry) delay(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return min(retryAfter, maxRetryAfter)
	}
	d := min(r.Base<<(attempt-1), r.Cap)
	return d/2 + rand.N(d/2+1) //nolint:gosec // Jitter only spreads retries out; it needn't be unpredictable.
}

func (r Retry) sleep(ctx context.Context, d time.Duration) error {
	if r.Sleep != nil {
		return r.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// retryable reports whether a failed attempt may succeed if repeated: the
// statuses that ask for a retry, and network failures that pass, such as a
// timeout or a refused connection. A TLS failure, an unknown host name or a
// canceled context won't change on a retry.
func retryable(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	var re retryableError
	if errors.As(err, &re) {
		return true
	}
	var ue *UnreachableError
	if !errors.As(err, &ue) {
		return false
	}
	return transient(err)
}

// transient reports whether a network error is one that passes. It lists
// the ones that do, since TLS failures don't all have types of their own.
func transient(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.IsTemporary || dnsErr.IsTimeout
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	var oe *net.OpError
	if errors.As(err, &oe) && (oe.Op == "dial" || oe.Op == "read" || oe.Op == "write") {
		return true
	}
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET)
}

// retryAfterHeader parses a Retry-After header: seconds, or an HTTP date.
func retryAfterHeader(v string, now time.Time) time.Duration {
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		return time.Duration(max(s, 0)) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(t.Sub(now), 0)
	}
	return 0
}
