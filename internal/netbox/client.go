// Package netbox reads DNS data from the NetBox DNS plugin, through NetBox's
// REST API (ADR-0023). It only reads: every request is a GET.
package netbox

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
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/tracing"
)

const (
	// maxBody bounds how much of a response nbpdns reads.
	maxBody = 64 << 20
	// maxRetryAfter caps how long a Retry-After header can make nbpdns wait.
	maxRetryAfter = time.Minute
)

// Options configure a Client. URL and Token are required.
type Options struct {
	URL         string
	Token       config.Secret
	CAFile      string
	Timeout     time.Duration
	PageSize    int
	Concurrency int
	Logger      *slog.Logger
	Tracer      trace.Tracer

	retry     retryPolicy // the default policy if zero
	supported []Release   // Supported if nil
}

// OptionsFrom returns the Options for nbpdns's NetBox configuration.
func OptionsFrom(c config.NetBoxConfig, log *slog.Logger, tracer trace.Tracer) Options {
	return Options{
		URL: c.URL, Token: c.Token, CAFile: c.CAFile, Timeout: c.Timeout,
		PageSize: c.PageSize, Concurrency: c.Concurrency, Logger: log, Tracer: tracer,
	}
}

// A Client reads from one NetBox. It's safe for concurrent use.
type Client struct {
	base         *url.URL
	auth         config.Secret // the Authorization header
	tokenVersion int
	http         *http.Client
	pageSize     int
	concurrency  int
	log          *slog.Logger
	retry        retryPolicy
	supported    []Release
}

// New returns a Client. It logs a warning if the URL uses http://, where the
// token crosses the network unencrypted, or if the token is a v1 token.
func New(ctx context.Context, o Options) (*Client, error) {
	if o.URL == "" {
		return nil, config.UnsetError("netbox.url")
	}
	if !o.Token.IsSet() {
		return nil, config.UnsetError("netbox.token")
	}
	base, err := url.Parse(o.URL)
	if err != nil {
		return nil, fmt.Errorf("netbox.url: %w", err)
	}
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	tlsConf, err := tlsConfig(o.CAFile)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConf
	transport.MaxIdleConnsPerHost = max(o.Concurrency, 2)

	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	c := &Client{
		base:        base,
		pageSize:    o.PageSize,
		concurrency: max(o.Concurrency, 1),
		log:         log,
		retry:       o.retry,
		supported:   o.supported,
		http: &http.Client{
			Timeout:   o.Timeout,
			Transport: &tracing.Transport{Base: transport, Tracer: o.Tracer},
			// Never follow a redirect: it would carry the token somewhere
			// the configuration didn't name. Report it instead.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	if c.retry.attempts == 0 {
		c.retry = defaultRetry
	}
	if c.supported == nil {
		c.supported = Supported
	}
	if o.Tracer == nil {
		c.http.Transport = transport
	}
	token := o.Token.Reveal()
	if strings.HasPrefix(token, "nbt_") {
		c.tokenVersion, c.auth = 2, config.NewSecret("Bearer "+token)
	} else {
		c.tokenVersion, c.auth = 1, config.NewSecret("Token "+token)
		log.WarnContext(ctx, "the NetBox token is a v1 token; NetBox recommends v2 tokens, which start with nbt_")
	}
	if base.Scheme == "http" {
		log.WarnContext(ctx, "the NetBox URL uses http://, so the token crosses the network unencrypted; use https:// if NetBox offers it",
			"url", base.String())
	}
	return c, nil
}

// Close closes the client's idle connections to NetBox. Call it when done
// with the client.
func (c *Client) Close() { c.http.CloseIdleConnections() }

// TokenVersion reports whether the token is a v1 or a v2 NetBox token.
func (c *Client) TokenVersion() int { return c.tokenVersion }

// URL returns NetBox's base URL.
func (c *Client) URL() string { return c.base.String() }

// Encrypted reports whether the client reaches NetBox over https://, so the
// token doesn't cross the network in the clear.
func (c *Client) Encrypted() bool { return c.base.Scheme == "https" }

// tlsConfig trusts the system's roots, plus the certificates in caFile.
func tlsConfig(caFile string) (*tls.Config, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("netbox.ca_file: %w", err)
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("netbox.ca_file: no PEM certificates in %s", caFile)
		}
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}, nil
}

// endpoint returns the URL of path, under NetBox's base URL, with query.
func (c *Client) endpoint(path string, query url.Values) *url.URL {
	u := c.base.JoinPath(path)
	if strings.HasSuffix(path, "/") && !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	u.RawQuery = query.Encode()
	return u
}

// rebase returns the URL of the page that NetBox's next-page link names,
// after the page at current. NetBox builds the link from the request it saw,
// which behind a proxy may name another scheme, host or path, and the token
// must only ever go to the configured one. The next page differs from the
// current one only in its query, so that's all rebase takes from the link.
func (c *Client) rebase(current *url.URL, link string) (*url.URL, error) {
	next, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("NetBox's next-page link %q: %w", link, err)
	}
	u := *current
	u.RawQuery = next.RawQuery
	return &u, nil
}

// get fetches u and decodes its JSON into out, retrying as the policy says.
func (c *Client) get(ctx context.Context, u *url.URL, out any) error {
	for attempt := 1; ; attempt++ {
		retryAfter, err := c.try(ctx, u, out, attempt)
		if err == nil {
			return nil
		}
		if attempt >= c.retry.attempts || !retryable(ctx, err) {
			return err
		}
		d := c.retry.delay(attempt, retryAfter)
		c.log.DebugContext(ctx, "retrying a NetBox request", "path", u.RequestURI(), "attempt", attempt,
			"delay_seconds", d.Seconds(), "err", err)
		if serr := c.retry.sleep(ctx, d); serr != nil {
			return err
		}
	}
}

// statusError marks an error response that may succeed if retried.
type statusError struct {
	error
	retry bool
}

func (e statusError) Unwrap() error { return e.error }

// try makes one attempt at fetching u. It returns the wait a Retry-After
// header asked for, if any.
func (c *Client) try(ctx context.Context, u *url.URL, out any, attempt int) (time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", c.auth.Reveal())
	req.Header.Set("Accept", "application/json")
	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		c.log.DebugContext(ctx, "request to NetBox failed", "path", u.RequestURI(), "attempt", attempt, "err", err)
		return 0, &UnreachableError{URL: c.base.String(), Err: err}
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		_ = resp.Body.Close()
	}()
	c.log.DebugContext(ctx, "request to NetBox", "path", u.RequestURI(), "status", resp.StatusCode,
		"attempt", attempt, "duration_seconds", time.Since(start).Seconds())

	if resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
		if err != nil {
			// The connection failed, or timed out, partway through the
			// answer, which a retry may get whole.
			return 0, &UnreachableError{URL: c.base.String(), Err: err}
		}
		if len(body) > maxBody {
			return 0, fmt.Errorf("NetBox's answer to GET %s is larger than %d MiB", u.RequestURI(), maxBody>>20)
		}
		if err := json.Unmarshal(body, out); err != nil {
			return 0, fmt.Errorf("decoding NetBox's answer to GET %s: %w", u.RequestURI(), err)
		}
		return 0, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	apiErr := &APIError{Status: resp.StatusCode, Path: u.RequestURI(), Detail: detail(body)}
	switch resp.StatusCode {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return retryAfterHeader(resp.Header.Get("Retry-After"), time.Now()), statusError{apiErr, true}
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		apiErr.Detail = fmt.Sprintf("NetBox redirected to %s; set netbox.url to NetBox's own address", resp.Header.Get("Location"))
	case http.StatusUnauthorized, http.StatusForbidden:
		return 0, c.denied(ctx, u, apiErr)
	}
	return 0, apiErr
}

// denied says why NetBox refused a request. NetBox answers 403 both for a
// bad token and for a missing permission, and its wording may change, so
// the status endpoint, which needs only a valid token, tells them apart.
func (c *Client) denied(ctx context.Context, u *url.URL, apiErr *APIError) error {
	status := c.endpoint("api/status/", nil)
	if u.Path == status.Path {
		return &AuthError{Detail: apiErr.Detail}
	}
	var s Status
	if _, err := c.try(ctx, status, &s, 1); err != nil {
		return err
	}
	return &PermissionError{ObjectType: objectType(u.Path), Detail: apiErr.Detail}
}

// objectType names the NetBox object type a DNS plugin endpoint lists, or
// returns the path if it isn't one.
func objectType(path string) string {
	for _, e := range endpoints {
		if strings.HasSuffix(path, "/"+pluginAPI+e.path) {
			return e.objectType
		}
	}
	return path
}

// detail extracts NetBox's explanation from an error response: its "detail",
// or its field errors.
func detail(body []byte) string {
	var v map[string]any
	if json.Unmarshal(body, &v) != nil {
		return strings.TrimSpace(string(body[:min(len(body), 200)]))
	}
	if d, ok := v["detail"].(string); ok {
		return d
	}
	var parts []string
	for field, msgs := range v {
		if list, ok := msgs.([]any); ok {
			for _, m := range list {
				parts = append(parts, fmt.Sprintf("%s: %v", field, m))
			}
		}
	}
	slices.Sort(parts)
	return strings.Join(parts, "; ")
}

// A retryPolicy says how often, and after how long, to retry a request.
type retryPolicy struct {
	attempts  int           // including the first
	base, cap time.Duration // the first delay, and the most any delay may be
	sleep     func(ctx context.Context, d time.Duration) error
}

var defaultRetry = retryPolicy{attempts: 4, base: 500 * time.Millisecond, cap: 10 * time.Second, sleep: sleep}

// delay returns how long to wait before the next attempt: what Retry-After
// asked for, up to a minute, or else exponential backoff with jitter.
func (p retryPolicy) delay(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return min(retryAfter, maxRetryAfter)
	}
	d := min(p.base<<(attempt-1), p.cap)
	return d/2 + rand.N(d/2+1) //nolint:gosec // Jitter only spreads retries out; it needn't be unpredictable.
}

func sleep(ctx context.Context, d time.Duration) error {
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
	var se statusError
	if errors.As(err, &se) {
		return se.retry
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
