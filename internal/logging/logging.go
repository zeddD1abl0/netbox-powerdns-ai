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
	return slog.New(contextHandler{base: h}), nil
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
// top level even inside a group, where log collectors look for them. slog's
// handlers put a record's attributes inside every group opened with
// WithGroup, so contextHandler opens no group on its base handler. It keeps
// the groups itself, and writes each record's attributes into them as group
// values, after the IDs. The cost per record doesn't grow with the groups or
// their bound attributes (ITEM-0027).
type contextHandler struct {
	// base is the handler with the attributes added before any group.
	base slog.Handler
	// groups are the groups opened since, outermost first, each with the
	// attributes added inside it.
	groups []group
}

// A group is one WithGroup call, and the attributes added inside it.
type group struct {
	name  string
	attrs []slog.Attr
}

func (c contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return c.base.Enabled(ctx, level)
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
	if len(c.groups) == 0 {
		r.AddAttrs(ids...)
		return c.base.Handle(ctx, r)
	}
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	out.AddAttrs(ids...)
	inner := make([]slog.Attr, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		inner = append(inner, a)
		return true
	})
	// From the innermost group out. A group left with nothing in it is a
	// group value with no attributes, which slog's handlers don't write.
	for i := len(c.groups) - 1; i >= 0; i-- {
		g := c.groups[i]
		attrs := make([]slog.Attr, 0, len(g.attrs)+len(inner))
		attrs = append(append(attrs, g.attrs...), inner...)
		inner = []slog.Attr{{Key: g.name, Value: slog.GroupValue(attrs...)}}
	}
	out.AddAttrs(inner...)
	return c.base.Handle(ctx, out)
}

func (c contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return c
	}
	if len(c.groups) == 0 {
		return contextHandler{base: c.base.WithAttrs(attrs)}
	}
	groups := slices.Clone(c.groups)
	last := &groups[len(groups)-1]
	last.attrs = append(slices.Clip(last.attrs), attrs...)
	return contextHandler{base: c.base, groups: groups}
}

func (c contextHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return c
	}
	return contextHandler{base: c.base, groups: append(slices.Clip(c.groups), group{name: name})}
}
