// Package powerdns reads DNS data from PowerDNS Authoritative servers,
// through their HTTP API (ADR-0006, ADR-0026): from each server group's
// primary, into the model that NetBox's data is read into (internal/dns). It
// only reads: every request is a GET.
package powerdns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/httpclient"
)

// Options configure a Client for one server group's primary.
type Options struct {
	// Group is the server group's name, which errors and logs give.
	Group       string
	Primary     config.Primary
	Timeout     time.Duration
	Concurrency int
	Logger      *slog.Logger
	Tracer      trace.Tracer
	// Observer, if set, counts the client's requests, for metrics.
	Observer httpclient.Observer

	retry     httpclient.Retry // the default policy if zero
	supported []string         // Supported if nil
}

// OptionsFrom returns the Options for group g's primary, with nbpdns's
// PowerDNS configuration c.
func OptionsFrom(g config.Group, c config.PowerDNSConfig, log *slog.Logger, tracer trace.Tracer) Options {
	return Options{
		Group: g.Name, Primary: g.Primary, Timeout: c.Timeout, Concurrency: c.Concurrency,
		Logger: log, Tracer: tracer,
	}
}

// A Client reads from one server group's primary. It's safe for concurrent
// use.
type Client struct {
	group       string
	serverID    string
	url         *url.URL // the API's server, such as …/api/v1/servers/localhost
	key         config.Secret
	http        *httpclient.Client
	concurrency int
	log         *slog.Logger
	supported   []string
}

// New returns a Client. It logs a warning if the URL uses http://, where the
// API key crosses the network unencrypted.
func New(ctx context.Context, o Options) (*Client, error) {
	keys := config.GroupsKey + "." + o.Group + ".primary"
	if o.Primary.URL == "" || !o.Primary.APIKey.IsSet() {
		return nil, fmt.Errorf("server group %s needs %s.url, and %s.api_key or %s.api_key_file", o.Group, keys, keys, keys)
	}
	base, err := url.Parse(o.Primary.URL)
	if err != nil {
		return nil, fmt.Errorf("%s.url: %w", keys, err)
	}
	serverID := o.Primary.ServerID
	if serverID == "" {
		serverID = "localhost"
	}
	if !strings.HasPrefix(base.Path, "/") {
		base.Path = "/" + base.Path
	}
	server := base.JoinPath("api/v1/servers", serverID)
	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	hc, err := httpclient.New(httpclient.Options{
		Service: "PowerDNS", URL: base.String(), Keys: keys,
		CAFile: o.Primary.CAFile, CertFile: o.Primary.CertFile, KeyFile: o.Primary.KeyFile,
		Timeout: o.Timeout, Conns: o.Concurrency, Logger: log, Tracer: o.Tracer, Retry: o.retry, Observer: o.Observer,
	})
	if err != nil {
		return nil, err
	}
	c := &Client{
		group: o.Group, serverID: serverID, url: server, key: o.Primary.APIKey, http: hc,
		concurrency: max(o.Concurrency, 1), log: log, supported: o.supported,
	}
	if c.supported == nil {
		c.supported = Supported
	}
	if base.Scheme == "http" {
		log.WarnContext(ctx, "a PowerDNS API URL uses http://, so its API key, which can change every zone, crosses the network unencrypted; put the API behind TLS",
			"group", o.Group, "url", base.String())
	}
	return c, nil
}

// Close closes the client's idle connections. Call it when done with the
// client.
func (c *Client) Close() { c.http.Close() }

// Group returns the server group's name.
func (c *Client) Group() string { return c.group }

// URL returns the API's URL for the server, such as
// https://pdns.example.com/api/v1/servers/localhost.
func (c *Client) URL() string { return c.url.String() }

// Encrypted reports whether the client reaches the API over https://, so the
// key doesn't cross the network in the clear.
func (c *Client) Encrypted() bool { return c.url.Scheme == "https" }

// header returns the headers of every request to the API.
func (c *Client) header() http.Header {
	return http.Header{"X-Api-Key": {c.key.Reveal()}, "Accept": {"application/json"}}
}

// endpoint returns the URL of path, under the server, with query.
func (c *Client) endpoint(path string, query url.Values) *url.URL {
	u := c.url.JoinPath(path)
	u.RawQuery = query.Encode()
	return u
}

// get fetches u and decodes its JSON into out, retrying as the policy says.
// notFound, if not nil, returns the error for a 404.
func (c *Client) get(ctx context.Context, u *url.URL, out any, notFound func() error) error {
	err := c.http.Get(ctx, u, c.header(), out)
	var se *httpclient.StatusError
	if !errors.As(err, &se) {
		return err
	}
	apiErr := &APIError{Group: c.group, Status: se.Status, Path: se.Path, Detail: detail(se.Body)}
	switch se.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return &AuthError{Group: c.group, Detail: apiErr.Detail}
	case http.StatusNotFound:
		if notFound != nil {
			return notFound()
		}
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		apiErr.Detail = fmt.Sprintf("PowerDNS redirected to %s; set %s.%s.primary.url to the API's own address",
			se.Header.Get("Location"), config.GroupsKey, c.group)
	}
	return apiErr
}

// detail extracts PowerDNS's explanation from an error response: its "error"
// field; or, from a proxy in front of the API, an HTML page's title; or else
// the start of the text, on one line. PowerDNS 5.0 answers some errors in
// plain text.
func detail(body []byte) string {
	var v struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &v) == nil && v.Error != "" {
		return v.Error
	}
	return httpclient.TextDetail(body)
}
