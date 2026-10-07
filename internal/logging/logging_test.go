package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/tracing"
)

// spanContext returns a context with a request ID and a span from a real
// provider, and the span's IDs.
func spanContext(t *testing.T) (context.Context, trace.SpanContext) {
	t.Helper()
	ctx := WithRequestID(t.Context(), "req-1")
	ctx, span := tracing.Tracer(tracing.NewProvider("test")).Start(ctx, "command")
	t.Cleanup(func() { span.End() })
	return ctx, span.SpanContext()
}

// lines parses each JSON log line in out.
func lines(t *testing.T, out string) []map[string]any {
	t.Helper()
	var recs []map[string]any
	for line := range strings.Lines(strings.TrimSpace(out)) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not a JSON line: %q", line)
		}
		recs = append(recs, m)
	}
	return recs
}

func TestJSONLinesCarryTheIDs(t *testing.T) {
	ctx, sc := spanContext(t)
	var buf bytes.Buffer
	log, err := New(&buf, "json", "debug")
	if err != nil {
		t.Fatal(err)
	}
	// Every way a line can be written: plain, with attributes, and inside
	// groups, where the IDs must stay at the top level.
	log.DebugContext(ctx, "plain")
	log.With("a", 1).InfoContext(ctx, "with attrs")
	log.WithGroup("netbox").InfoContext(ctx, "in a group", "zone", "example.com.")
	log.With("a", 1).WithGroup("netbox").With("b", 2).WarnContext(ctx, "nested", "c", 3)
	log.WithGroup("").ErrorContext(ctx, "empty group")

	recs := lines(t, buf.String())
	if len(recs) != 5 {
		t.Fatalf("got %d lines, want 5:\n%s", len(recs), buf.String())
	}
	for _, r := range recs {
		if r["trace_id"] != sc.TraceID().String() || r["span_id"] != sc.SpanID().String() || r["request_id"] != "req-1" {
			t.Errorf("line %q: trace_id=%v span_id=%v request_id=%v", r["msg"], r["trace_id"], r["span_id"], r["request_id"])
		}
	}
	nested := recs[3]
	group, _ := nested["netbox"].(map[string]any)
	if nested["a"] != 1.0 || group["b"] != 2.0 || group["c"] != 3.0 {
		t.Errorf("attributes moved: %v", nested)
	}
	if g, _ := recs[2]["netbox"].(map[string]any); g["zone"] != "example.com." {
		t.Errorf("grouped attribute lost: %v", recs[2])
	}
}

func TestTextLinesCarryTheIDs(t *testing.T) {
	ctx, sc := spanContext(t)
	var buf bytes.Buffer
	log, err := New(&buf, "text", "info")
	if err != nil {
		t.Fatal(err)
	}
	log.WithGroup("g").InfoContext(ctx, "hello", "k", "v")
	out := buf.String()
	for _, want := range []string{"trace_id=" + sc.TraceID().String(), "span_id=" + sc.SpanID().String(), "request_id=req-1", "g.k=v"} {
		if !strings.Contains(out, want) {
			t.Errorf("line doesn't contain %s: %s", want, out)
		}
	}
}

func TestNoIDsWithoutAContext(t *testing.T) {
	var buf bytes.Buffer
	log, err := New(&buf, "json", "info")
	if err != nil {
		t.Fatal(err)
	}
	log.InfoContext(t.Context(), "no span")
	r := lines(t, buf.String())[0]
	if _, ok := r["trace_id"]; ok {
		t.Errorf("a line without a span has a trace_id: %v", r)
	}
}

func TestLevels(t *testing.T) {
	var buf bytes.Buffer
	log, err := New(&buf, "json", "warn")
	if err != nil {
		t.Fatal(err)
	}
	log.InfoContext(t.Context(), "hidden")
	log.WarnContext(t.Context(), "shown")
	if recs := lines(t, buf.String()); len(recs) != 1 || recs[0]["msg"] != "shown" {
		t.Errorf("at warn, got %v", recs)
	}
}

func TestSecretsAreRedacted(t *testing.T) {
	ctx, _ := spanContext(t)
	for _, format := range []string{"json", "text"} {
		var buf bytes.Buffer
		log, err := New(&buf, format, "info")
		if err != nil {
			t.Fatal(err)
		}
		tok := config.NewSecret("nbt_hunter2.s3cr3t") // A fake token, to test redaction. gitleaks:allow
		cfg := config.Config{NetBox: config.NetBoxConfig{Token: tok}}
		log.InfoContext(ctx, "loaded", slog.Any("token", tok), slog.Any("config", cfg), slog.Any("ptr", &tok))
		if out := buf.String(); strings.Contains(out, "hunter2") || !strings.Contains(out, config.Redacted) {
			t.Errorf("%s: %s", format, out)
		}
	}
}

func TestNewRejectsBadSettings(t *testing.T) {
	if _, err := New(&bytes.Buffer{}, "xml", "info"); err == nil {
		t.Error("format xml accepted")
	}
	if _, err := New(&bytes.Buffer{}, "json", "loud"); err == nil {
		t.Error("level loud accepted")
	}
}

func TestNestedGroups(t *testing.T) {
	ctx, sc := spanContext(t)
	var buf bytes.Buffer
	log, err := New(&buf, "json", "debug")
	if err != nil {
		t.Fatal(err)
	}
	g := log.With("top", 1).WithGroup("a").With("x", 1).WithGroup("b").With("y", 2, "token", config.NewSecret("s3cret"))
	g.InfoContext(ctx, "deep", "z", 3)
	// A group that ends up with nothing in it isn't written.
	log.WithGroup("empty").InfoContext(ctx, "no attrs")

	recs := lines(t, buf.String())
	if len(recs) != 2 {
		t.Fatalf("got %d lines:\n%s", len(recs), buf.String())
	}
	a, _ := recs[0]["a"].(map[string]any)
	b, _ := a["b"].(map[string]any)
	if recs[0]["top"] != 1.0 || a["x"] != 1.0 || b["y"] != 2.0 || b["z"] != 3.0 || b["token"] != "[redacted]" {
		t.Errorf("nested line: %v", recs[0])
	}
	if strings.Contains(buf.String(), "s3cret") {
		t.Errorf("a secret was logged:\n%s", buf.String())
	}
	for _, r := range recs {
		if r["trace_id"] != sc.TraceID().String() || r["request_id"] != "req-1" {
			t.Errorf("line %q lacks the IDs: %v", r["msg"], r)
		}
	}
	if _, ok := recs[1]["empty"]; ok {
		t.Errorf("an empty group was written: %v", recs[1])
	}
}

// BenchmarkLog logs through a logger without groups, and through one with a
// group and bound attributes, whose cost per record shouldn't grow with
// them (ITEM-0027).
func BenchmarkLog(b *testing.B) {
	ctx := WithRequestID(b.Context(), "req-1")
	ctx, span := tracing.Tracer(tracing.NewProvider("test")).Start(ctx, "command")
	defer span.End()
	log, err := New(io.Discard, "json", "info")
	if err != nil {
		b.Fatal(err)
	}
	attrs := []any{"a", 1, "b", "two", "c", 3.0, "d", true, "e", "five"}
	for _, bb := range []struct {
		name string
		log  *slog.Logger
	}{
		{"flat", log.With(attrs...)},
		{"grouped", log.WithGroup("netbox").With(attrs...)},
	} {
		b.Run(bb.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				bb.log.InfoContext(ctx, "http request", "path", "/api/status/", "status", 200)
			}
		})
	}
}
