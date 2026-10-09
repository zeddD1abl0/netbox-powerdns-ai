package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api/gen"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/webhook"
)

// maxEventSize bounds a webhook's body (ADR-0036). NetBox's events for the
// DNS plugin's objects are a few KiB.
const maxEventSize = 1 << 20

// A Notifier queues the refreshes that NetBox's webhooks ask for.
type Notifier interface {
	// Notify queues the refresh r that the event e asks for, and reports
	// whether it queued anything: not if r asks for nothing, or names only
	// zones in views that no server group serves. ctx carries the webhook
	// request's span, which the refresh links to.
	Notify(ctx context.Context, e webhook.Event, r webhook.Refresh) bool
}

// verified is the generated server's middleware for the routes whose
// operations need NetBox's signature (ADR-0036). Before the strict server
// decodes anything, it reads the body, up to maxEventSize, and checks its
// signature against netbox.webhook_secret: a request that it refuses is
// never decoded. Until the secret is set, those routes are off.
func (o Options) verified(ops map[string]operation) gen.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !ops[r.Pattern].signed {
				next.ServeHTTP(w, r)
				return
			}
			if !o.WebhookSecret.IsSet() {
				writeProblem(w, r, http.StatusNotFound,
					"Webhooks are off. Set netbox.webhook_secret, and give NetBox's webhook the same secret, to turn them on.")
				return
			}
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxEventSize))
			if err != nil {
				o.countWebhook(metrics.WebhookInvalid)
				if tooLarge := (*http.MaxBytesError)(nil); errors.As(err, &tooLarge) {
					writeProblem(w, r, http.StatusRequestEntityTooLarge, "The body is over 1 MiB.")
					return
				}
				writeProblem(w, r, http.StatusBadRequest, "The body couldn't be read.")
				return
			}
			sig := r.Header.Get(webhook.SignatureHeader)
			if !webhook.Verify(o.WebhookSecret.Reveal(), body, sig) {
				o.countWebhook(metrics.WebhookBadSignature)
				detail := "The request's X-Hook-Signature isn't the body's: give NetBox's webhook the secret in netbox.webhook_secret."
				if sig == "" {
					detail = "The request has no X-Hook-Signature: give NetBox's webhook the secret in netbox.webhook_secret."
				}
				o.Log.WarnContext(r.Context(), "refused a NetBox webhook whose signature didn't verify",
					"remote_addr", r.RemoteAddr, "signed", sig != "")
				w.Header().Set("WWW-Authenticate", webhook.SignatureHeader)
				writeProblem(w, r, http.StatusUnauthorized, detail)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			sw := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(sw, r)
			// The strict server, or the handler, found it isn't an event.
			if sw.status() == http.StatusBadRequest {
				o.countWebhook(metrics.WebhookInvalid)
			}
		})
	}
}

// countWebhook counts a webhook's result, if there are metrics.
func (o Options) countWebhook(result string) {
	if o.Metrics != nil {
		o.Metrics.NetBoxWebhook(result)
	}
}

// ReceiveNetBoxEvent queues the refresh that a NetBox event asks for, and
// records NetBox's request and user in the request's span and log (Q-037).
// verified has checked its signature.
func (s *server) ReceiveNetBoxEvent(ctx context.Context, req gen.ReceiveNetBoxEventRequestObject) (gen.ReceiveNetBoxEventResponseObject, error) {
	e := *req.Body
	span := trace.SpanFromContext(ctx)
	for k, v := range map[string]string{
		"netbox.request_id": e.RequestID(), "netbox.user": e.User(),
		"netbox.event": e.Event, "netbox.object_type": e.ObjectType,
	} {
		if v != "" {
			span.SetAttributes(attribute.String(k, v))
		}
	}
	r, err := e.Refresh()
	if err != nil {
		prob := problem(request(ctx), http.StatusBadRequest, "The body isn't an event that NetBox sends: "+err.Error()+".")
		return gen.ReceiveNetBoxEvent400ApplicationProblemPlusJSONResponse{InvalidEventApplicationProblemPlusJSONResponse: gen.InvalidEventApplicationProblemPlusJSONResponse{Body: prob}}, nil
	}
	queued := s.o.Events.Notify(ctx, e, r)
	result := metrics.WebhookIgnored
	if queued {
		result = metrics.WebhookAccepted
	}
	s.o.countWebhook(result)
	s.o.Log.InfoContext(ctx, "received a NetBox event", "netbox_request_id", e.RequestID(), "netbox_user", e.User(),
		"event", e.Event, "object_type", e.ObjectType, "zones", r.Zones, "full_refresh", r.Full,
		"reason", r.Reason, "result", result)
	return gen.ReceiveNetBoxEvent202Response{}, nil
}
