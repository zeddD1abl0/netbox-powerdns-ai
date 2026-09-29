// Package logging builds nbpdns's operational logger, on log/slog. Every
// record logged with a context carries the trace_id and span_id of the
// context's span, and its request_id. Logging calls must pass the context,
// which golangci-lint's sloglint enforces.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"

	"go.opentelemetry.io/otel/trace"
)

// New returns a logger that writes format ("json" or "text") to w, from
// level ("debug", "info", "warn" or "error") up.
func New(w io.Writer, format, level string) (*slog.Logger, error) {
	var lv slog.Level
	if err := lv.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("log level %q: %w", level, err)
	}
	opts := &slog.HandlerOptions{Level: lv}
	var h slog.Handler
	switch format {
	case "json":
		h = slog.NewJSONHandler(w, opts)
	case "text":
		h = slog.NewTextHandler(w, opts)
	default:
		return nil, fmt.Errorf("log format %q isn't json or text", format)
	}
	return slog.New(contextHandler{base: h, h: h}), nil
}

type requestIDKey struct{}

// WithRequestID returns a copy of ctx whose log records carry id as their
// request_id. A command's ID identifies one run of nbpdns; from M04, a
// request's ID identifies one HTTP request.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns ctx's request ID, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// contextHandler adds the IDs in a record's context to the record, at the
// top level even inside a group, where log collectors look for them.
type contextHandler struct {
	// base is the handler with the attributes added before any group.
	base slog.Handler
	// h is base with every later WithGroup and WithAttrs applied, and ops
	// are those calls, replayed onto base plus the IDs.
	h   slog.Handler
	ops []func(slog.Handler) slog.Handler
}

func (c contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return c.h.Enabled(ctx, level)
}

func (c contextHandler) Handle(ctx context.Context, r slog.Record) error {
	var ids []slog.Attr
	// This handler is the one place that sets these keys; sloglint forbids
	// them everywhere else.
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		ids = append(ids,
			slog.String("trace_id", sc.TraceID().String()), //nolint:sloglint // The handler sets the IDs.
			slog.String("span_id", sc.SpanID().String()))   //nolint:sloglint // The handler sets the IDs.
	}
	if id := RequestID(ctx); id != "" {
		ids = append(ids, slog.String("request_id", id)) //nolint:sloglint // The handler sets the IDs.
	}
	switch {
	case len(ids) == 0:
		return c.h.Handle(ctx, r)
	case len(c.ops) == 0:
		r.AddAttrs(ids...)
		return c.h.Handle(ctx, r)
	}
	h := c.base.WithAttrs(ids)
	for _, op := range c.ops {
		h = op(h)
	}
	return h.Handle(ctx, r)
}

func (c contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(c.ops) == 0 {
		b := c.base.WithAttrs(attrs)
		return contextHandler{base: b, h: b}
	}
	return c.with(func(h slog.Handler) slog.Handler { return h.WithAttrs(attrs) })
}

func (c contextHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return c
	}
	return c.with(func(h slog.Handler) slog.Handler { return h.WithGroup(name) })
}

func (c contextHandler) with(op func(slog.Handler) slog.Handler) contextHandler {
	return contextHandler{base: c.base, h: op(c.h), ops: append(slices.Clip(c.ops), op)}
}
