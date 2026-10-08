package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/logging"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/webhook"
)

const hookSecret = "test-webhook-secret-0123456789" // gitleaks:allow

// fakeNotifier records what it's asked to queue, and reports queued.
type fakeNotifier struct {
	queued bool

	mu        sync.Mutex
	events    []webhook.Event
	refreshes []webhook.Refresh
	spans     []bool // whether each call's ctx had a span
}

func (f *fakeNotifier) Notify(ctx context.Context, e webhook.Event, r webhook.Refresh) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events, f.refreshes = append(f.events, e), append(f.refreshes, r)
	f.spans = append(f.spans, trace.SpanContextFromContext(ctx).IsValid())
	return f.queued
}

// hooks is the API with webhooks on, or off if secret is "", and its log,
// spans, metrics and notifier recorded.
type hooks struct {
	instrumented
	events *fakeNotifier
}

func newHooks(t *testing.T, secret string, queued bool) *hooks {
	t.Helper()
	h := &hooks{
		instrumented: instrumented{log: &bytes.Buffer{}, spans: tracetest.NewSpanRecorder(), metrics: metrics.New(version.Info{})},
		events:       &fakeNotifier{queued: queued},
	}
	log, err := logging.New(h.log, "json", "info")
	if err != nil {
		t.Fatal(err)
	}
	h.h = New(Options{
		Source: fakeSource{}, Log: log, Metrics: h.metrics, Events: h.events,
		WebhookSecret: config.NewSecret(secret),
		Tracer:        sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(h.spans)).Tracer("test"),
	})
	return h
}

// post sends body to /api/netbox-events with sig as its X-Hook-Signature,
// if it isn't "", checks the response against the OpenAPI document, and
// returns it.
func (h *hooks) post(t *testing.T, body []byte, sig string) reply {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/netbox-events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if sig != "" {
		req.Header.Set(webhook.SignatureHeader, sig)
	}
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()
	checker.Check(t, req, resp)
	return reply{rec.Code, rec.Header(), rec.Body.Bytes()}
}

// webhooks returns the count of each webhook result.
func (h *hooks) webhooks(t *testing.T) map[string]float64 {
	t.Helper()
	mfs, err := h.metrics.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, mf := range mfs {
		if mf.GetName() == "nbpdns_netbox_webhooks_total" {
			for _, m := range mf.GetMetric() {
				got[m.GetLabel()[0].GetValue()] = m.GetCounter().GetValue()
			}
		}
	}
	return got
}

func capturedEvent(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../webhook/testdata/netbox-47/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func counts(accepted, ignored, badSignature, invalid float64) map[string]float64 {
	return map[string]float64{
		metrics.WebhookAccepted: accepted, metrics.WebhookIgnored: ignored,
		metrics.WebhookBadSignature: badSignature, metrics.WebhookInvalid: invalid,
	}
}

func TestNetBoxEventAccepted(t *testing.T) {
	h := newHooks(t, hookSecret, true)
	body := capturedEvent(t, "zone-renamed.json")
	r := h.post(t, body, webhook.Sign(hookSecret, body))
	if r.code != http.StatusAccepted || len(r.body) != 0 || r.header.Get("X-Flow-ID") == "" {
		t.Fatalf("%d %q, X-Flow-ID %q", r.code, r.body, r.header.Get("X-Flow-ID"))
	}
	if len(h.events.events) != 1 {
		t.Fatalf("%d notifications", len(h.events.events))
	}
	e, refresh := h.events.events[0], h.events.refreshes[0]
	want := webhook.Refresh{Zones: []webhook.Zone{{View: "capture-a", Name: "renamed.example."}, {View: "capture-a", Name: "capture.example."}}}
	if e.ObjectType != webhook.TypeZone || e.User() != "admin" || !reflect.DeepEqual(refresh, want) || !h.events.spans[0] {
		t.Errorf("notified of %+v, %+v, with a span: %t", e, refresh, h.events.spans[0])
	}
	if got := h.webhooks(t); !reflect.DeepEqual(got, counts(1, 0, 0, 0)) {
		t.Errorf("webhooks %v", got)
	}

	// The request's span and log line carry NetBox's request and user.
	spans := h.spans.Ended()
	if len(spans) != 1 || spans[0].Name() != "POST /api/netbox-events" {
		t.Fatalf("spans %v", spans)
	}
	attrs := map[attribute.Key]string{}
	for _, a := range spans[0].Attributes() {
		attrs[a.Key] = a.Value.String()
	}
	for k, v := range map[attribute.Key]string{
		"netbox.request_id": e.RequestID(), "netbox.user": "admin", "netbox.event": "updated",
		"netbox.object_type": "netbox_dns.zone", "http.response.status_code": "202",
	} {
		if attrs[k] != v || v == "" {
			t.Errorf("span attribute %s = %q, want %q", k, attrs[k], v)
		}
	}
	var line struct {
		Msg             string         `json:"msg"`
		RequestID       string         `json:"request_id"`
		NetBoxRequestID string         `json:"netbox_request_id"`
		NetBoxUser      string         `json:"netbox_user"`
		Zones           []webhook.Zone `json:"zones"`
		Result          string         `json:"result"`
	}
	if err := json.Unmarshal(bytes.SplitN(h.log.Bytes(), []byte("\n"), 2)[0], &line); err != nil {
		t.Fatal(err)
	}
	if line.Msg != "received a NetBox event" || line.RequestID != r.header.Get("X-Flow-ID") || line.NetBoxRequestID != e.RequestID() ||
		line.NetBoxUser != "admin" || !reflect.DeepEqual(line.Zones, want.Zones) || line.Result != "accepted" {
		t.Errorf("log line %+v:\n%s", line, h.log)
	}
}

func TestNetBoxEventIgnored(t *testing.T) {
	tests := []struct {
		name, file string
		queued     bool
	}{
		{"another type", "tag-created.json", false},
		{"zones that no group serves", "record-created.json", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHooks(t, hookSecret, tt.queued)
			body := capturedEvent(t, tt.file)
			if r := h.post(t, body, webhook.Sign(hookSecret, body)); r.code != http.StatusAccepted {
				t.Fatalf("%d %s", r.code, r.body)
			}
			if got := h.webhooks(t); !reflect.DeepEqual(got, counts(0, 1, 0, 0)) {
				t.Errorf("webhooks %v", got)
			}
		})
	}
}

func TestNetBoxEventSignatures(t *testing.T) {
	body := capturedEvent(t, "record-created.json")
	tests := []struct {
		name   string
		body   []byte
		sig    string
		detail string
	}{
		{"none", body, "", "The request has no X-Hook-Signature"},
		{"another secret's", body, webhook.Sign("another-secret-0123456789", body), "isn't the body's"},
		{"another body's", body, webhook.Sign(hookSecret, capturedEvent(t, "record-updated.json")), "isn't the body's"},
		{"not hex", body, "sha512=" + webhook.Sign(hookSecret, body), "isn't the body's"},
		// The signature is checked before anything decodes the body, so a
		// forged request learns nothing about how it would be read.
		{"over a body that isn't JSON", []byte("{not json"), webhook.Sign("another-secret-0123456789", []byte("{not json")), "isn't the body's"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHooks(t, hookSecret, true)
			r := h.post(t, tt.body, tt.sig)
			p := problemOf(t, r, http.StatusUnauthorized)
			if !strings.Contains(*p.Detail, tt.detail) || r.header.Get("WWW-Authenticate") != webhook.SignatureHeader {
				t.Errorf("detail %q, WWW-Authenticate %q", *p.Detail, r.header.Get("WWW-Authenticate"))
			}
			if len(h.events.events) != 0 {
				t.Error("a refresh was queued")
			}
			if got := h.webhooks(t); !reflect.DeepEqual(got, counts(0, 0, 1, 0)) {
				t.Errorf("webhooks %v", got)
			}
			if tt.sig != "" && strings.Contains(h.log.String(), tt.sig) {
				t.Errorf("the log has the signature:\n%s", h.log)
			}
			if strings.Contains(h.log.String(), hookSecret) {
				t.Errorf("the log has the secret:\n%s", h.log)
			}
		})
	}
}

func TestNetBoxEventInvalid(t *testing.T) {
	tests := []struct {
		name, body, detail string
	}{
		{"not JSON", `{"event": `, "can't decode JSON body"},
		{"a JSON array", `[]`, "can't decode JSON body"},
		{"no object_type", `{"event": "created", "data": {}}`, "The body isn't an event that NetBox sends: the event has no event or no object_type."},
		{"a record without its zone", `{"event": "created", "object_type": "netbox_dns.record", "data": {"name": "www"}}`,
			"a netbox_dns.record event's data has no zone with a name and a view"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHooks(t, hookSecret, true)
			body := []byte(tt.body)
			p := problemOf(t, h.post(t, body, webhook.Sign(hookSecret, body)), http.StatusBadRequest)
			if !strings.Contains(*p.Detail, tt.detail) {
				t.Errorf("detail %q, want %q", *p.Detail, tt.detail)
			}
			if len(h.events.events) != 0 {
				t.Error("a refresh was queued")
			}
			if got := h.webhooks(t); !reflect.DeepEqual(got, counts(0, 0, 0, 1)) {
				t.Errorf("webhooks %v", got)
			}
		})
	}
}

func TestNetBoxEventTooLarge(t *testing.T) {
	h := newHooks(t, hookSecret, true)
	body := append([]byte(`{"event": "created", "object_type": "netbox_dns.view", "data": {"x": "`),
		bytes.Repeat([]byte("x"), maxEventSize)...)
	body = append(body, `"}}`...)
	p := problemOf(t, h.post(t, body, webhook.Sign(hookSecret, body)), http.StatusRequestEntityTooLarge)
	if *p.Detail != "The body is over 1 MiB." || len(h.events.events) != 0 {
		t.Errorf("detail %q, %d notifications", *p.Detail, len(h.events.events))
	}
	if got := h.webhooks(t); !reflect.DeepEqual(got, counts(0, 0, 0, 1)) {
		t.Errorf("webhooks %v", got)
	}
}

func TestNetBoxEventsOff(t *testing.T) {
	h := newHooks(t, "", true)
	body := capturedEvent(t, "record-created.json")
	p := problemOf(t, h.post(t, body, webhook.Sign(hookSecret, body)), http.StatusNotFound)
	if !strings.Contains(*p.Detail, "Set netbox.webhook_secret") || len(h.events.events) != 0 {
		t.Errorf("detail %q, %d notifications", *p.Detail, len(h.events.events))
	}
	if got := h.webhooks(t); !reflect.DeepEqual(got, counts(0, 0, 0, 0)) {
		t.Errorf("webhooks %v", got)
	}
	// The other operations don't need a signature.
	if r := h.do(t, http.MethodGet, "/api/status", nil); r.code != http.StatusOK {
		t.Errorf("GET /api/status: %d", r.code)
	}
	// Only POST is answered.
	r := h.do(t, http.MethodGet, "/api/netbox-events", nil)
	if p := problemOf(t, r, http.StatusMethodNotAllowed); *p.Detail != "/api/netbox-events answers only POST, not GET." {
		t.Errorf("detail %q", *p.Detail)
	}
}

func TestNewNeedsEventsForWebhooks(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("New didn't panic")
		}
	}()
	New(Options{Source: fakeSource{}, WebhookSecret: config.NewSecret(hookSecret)})
}
