// Package netbox reads DNS data from the NetBox DNS plugin, through NetBox's
// REST API (ADR-0023). It only reads: every request is a GET.
package netbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/httpclient"
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

	retry     httpclient.Retry // the default policy if zero
	supported []Release        // Supported if nil
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
	http         *httpclient.Client
	pageSize     int
	concurrency  int
	log          *slog.Logger
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
	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	hc, err := httpclient.New(httpclient.Options{
		Service: "NetBox", URL: base.String(), Keys: "netbox", CAFile: o.CAFile,
		Timeout: o.Timeout, Conns: o.Concurrency, Logger: log, Tracer: o.Tracer, Retry: o.retry,
	})
	if err != nil {
		return nil, err
	}
	c := &Client{
		base:        base,
		http:        hc,
		pageSize:    o.PageSize,
		concurrency: max(o.Concurrency, 1),
		log:         log,
		supported:   o.supported,
	}
	if c.supported == nil {
		c.supported = Supported
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
func (c *Client) Close() { c.http.Close() }

// TokenVersion reports whether the token is a v1 or a v2 NetBox token.
func (c *Client) TokenVersion() int { return c.tokenVersion }

// URL returns NetBox's base URL.
func (c *Client) URL() string { return c.base.String() }

// Encrypted reports whether the client reaches NetBox over https://, so the
// token doesn't cross the network in the clear.
func (c *Client) Encrypted() bool { return c.base.Scheme == "https" }

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

// header returns the headers of every request to NetBox.
func (c *Client) header() http.Header {
	return http.Header{"Authorization": {c.auth.Reveal()}, "Accept": {"application/json"}}
}

// get fetches u and decodes its JSON into out, retrying as the policy says.
func (c *Client) get(ctx context.Context, u *url.URL, out any) error {
	return c.check(ctx, u, c.http.Get(ctx, u, c.header(), out))
}

// check turns an error answer from NetBox into this package's errors.
func (c *Client) check(ctx context.Context, u *url.URL, err error) error {
	var se *httpclient.StatusError
	if !errors.As(err, &se) {
		return err
	}
	apiErr := &APIError{Status: se.Status, Path: se.Path, Detail: detail(se.Body)}
	switch se.Status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		apiErr.Detail = fmt.Sprintf("NetBox redirected to %s; set netbox.url to NetBox's own address", se.Header.Get("Location"))
	case http.StatusUnauthorized, http.StatusForbidden:
		return c.denied(ctx, u, apiErr)
	}
	return apiErr
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
	if err := c.http.GetOnce(ctx, status, c.header(), &s); err != nil {
		return c.check(ctx, status, err)
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
