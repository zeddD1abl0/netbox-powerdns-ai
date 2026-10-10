package audit

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/logging"
)

// TestSumIsTheCanonicalForm pins the canonical form that the hash covers, so
// that a reader of the trail, such as a SIEM, can check it.
func TestSumIsTheCanonicalForm(t *testing.T) {
	e := Event{
		Seq: 1, Time: time.Date(2026, 10, 10, 11, 2, 3, 4000, time.FixedZone("AEST", 10*3600)),
		Actor: Actor{Kind: ActorOSUser, ID: "jordan"}, Action: ServiceStarted, Target: Target{Type: "service", ID: "host1"},
		After: json.RawMessage(`{"a":"<b>"}`), Reason: "r&d", Outcome: Success, TraceID: "t", RequestID: "q",
	}
	canonical := `{"seq":1,"time":"2026-10-10T01:02:03.000004Z","actor":"os-user:jordan","action":"service.started",` +
		`"target":"service:host1","before":null,"after":"{\"a\":\"<b>\"}","reason":"r&d","outcome":"success",` +
		`"trace_id":"t","request_id":"q","source_ip":"","prev_hash":""}`
	want := sha512.Sum384([]byte(canonical))
	if got := e.Sum(); got != hex.EncodeToString(want[:]) {
		t.Errorf("Sum() = %s, want the SHA-384 of %s", got, canonical)
	}
}

// TestSumCoversEveryField changes each field in turn, and sees the hash
// change.
func TestSumCoversEveryField(t *testing.T) {
	base := Event{
		Seq: 7, Time: time.Date(2026, 10, 10, 1, 2, 3, 0, time.UTC), Actor: System(), Action: AuditPruned,
		Target: Target{Type: "audit", ID: "trail"}, Before: json.RawMessage(`{}`), After: json.RawMessage(`{}`),
		Reason: "r", Outcome: Success, TraceID: "t", RequestID: "q", SourceIP: "192.0.2.1", PrevHash: "p",
	}
	changes := map[string]func(e *Event){
		"seq":        func(e *Event) { e.Seq++ },
		"time":       func(e *Event) { e.Time = e.Time.Add(time.Microsecond) },
		"actor":      func(e *Event) { e.Actor = File("/etc/nbpdns.yaml") },
		"action":     func(e *Event) { e.Action = ServiceStopped },
		"target":     func(e *Event) { e.Target.ID = "other" },
		"before":     func(e *Event) { e.Before = nil },
		"after":      func(e *Event) { e.After = json.RawMessage(`{"x":1}`) },
		"reason":     func(e *Event) { e.Reason = "" },
		"outcome":    func(e *Event) { e.Outcome = Failure },
		"trace_id":   func(e *Event) { e.TraceID = "u" },
		"request_id": func(e *Event) { e.RequestID = "r" },
		"source_ip":  func(e *Event) { e.SourceIP = "" },
		"prev_hash":  func(e *Event) { e.PrevHash = "o" },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			e := base
			change(&e)
			if e.Sum() == base.Sum() {
				t.Errorf("changing %s leaves the hash as it was", name)
			}
		})
	}
	// The hash doesn't cover itself.
	e := base
	e.Hash = "anything"
	if e.Sum() != base.Sum() {
		t.Error("the hash covers the Hash field")
	}
}

// chain returns five events, linked from anchor, or from the start if anchor
// is nil.
func chain(anchor *Anchor) []Event {
	const n = 5
	seq, prev := int64(1), ""
	if anchor != nil {
		seq, prev = anchor.Seq+1, anchor.Hash
	}
	events := make([]Event, n)
	for i := range events {
		e := Event{
			Seq: seq, Time: time.Date(2026, 10, 10, 0, 0, i, 0, time.UTC), Actor: System(), Action: ServiceStarted,
			Target: Target{Type: "service", ID: "h"}, Outcome: Success, PrevHash: prev,
		}
		e.Hash = e.Sum()
		events[i] = e
		seq, prev = seq+1, e.Hash
	}
	return events
}

func TestVerifier(t *testing.T) {
	anchor := &Anchor{Seq: 41, Hash: "abc"}
	tests := []struct {
		name    string
		anchor  *Anchor
		events  func() []Event
		wantSeq int64  // where it breaks, or 0
		want    string // part of the problem
	}{
		{"an intact trail", nil, func() []Event { return chain(nil) }, 0, ""},
		{"an intact trail after an anchor", anchor, func() []Event { return chain(anchor) }, 0, ""},
		{"an empty trail", nil, func() []Event { return nil }, 0, ""},
		{"an edited event", nil, func() []Event {
			es := chain(nil)
			es[2].Reason = "edited"
			return es
		}, 3, "its contents don't match its hash"},
		{"an edited event, its hash made again", nil, func() []Event {
			es := chain(nil)
			es[2].Reason = "edited"
			es[2].Hash = es[2].Sum()
			return es
		}, 4, "doesn't link to the event before it"},
		{"a deleted event", nil, func() []Event {
			es := chain(nil)
			return append(es[:2], es[3:]...)
		}, 4, "the events from 3 to 3 are missing"},
		{"the first events deleted, without an anchor", nil, func() []Event { return chain(nil)[2:] }, 3,
			"the events from 1 to 2 are missing"},
		{"an inserted event, the rest renumbered", nil, func() []Event {
			es := chain(nil)
			forged := Event{Seq: 3, Time: es[2].Time, Actor: System(), Action: ServiceStopped,
				Target: Target{Type: "service", ID: "h"}, Outcome: Success, PrevHash: es[1].Hash}
			forged.Hash = forged.Sum()
			rest := chain(nil)[2:]
			for i := range rest {
				rest[i].Seq++
				rest[i].Hash = rest[i].Sum()
			}
			return append(append(es[:2], forged), rest...)
		}, 4, "doesn't link to the event before it"},
		{"two events swapped", nil, func() []Event {
			es := chain(nil)
			es[1], es[2] = es[2], es[1]
			return es
		}, 3, "the events from 2 to 2 are missing"},
		{"a trail that doesn't start at its anchor", anchor, func() []Event { return chain(nil) }, 1,
			"it comes after event 41, out of order"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewVerifier(tt.anchor)
			var err error
			events := tt.events()
			for _, e := range events {
				if err = v.Check(e); err != nil {
					break
				}
			}
			if tt.wantSeq == 0 {
				if err != nil || v.Count() != len(events) {
					t.Errorf("Check() = %v after %d events, want all %d to pass", err, v.Count(), len(events))
				}
				return
			}
			var be *BrokenError
			if !errors.As(err, &be) || be.Seq != tt.wantSeq || !strings.Contains(be.Problem, tt.want) {
				t.Errorf("Check() = %v, want a break at %d with %q", err, tt.wantSeq, tt.want)
			}
		})
	}
}

func TestActors(t *testing.T) {
	for _, a := range []Actor{System(), File("/etc/nbpdns/nbpdns.yaml"), {Kind: ActorOSUser, ID: "jordan"}} {
		if got := ParseActor(a.String()); got != a {
			t.Errorf("ParseActor(%q) = %+v, want %+v", a.String(), got, a)
		}
	}
	if s := System().String(); s != "system" {
		t.Errorf("System() = %q", s)
	}
	if u := OSUser(); u.Kind != ActorOSUser || u.ID == "" {
		t.Errorf("OSUser() = %+v, want this process's user", u)
	}
}

func TestWithContext(t *testing.T) {
	tid, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	sid, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: tid, SpanID: sid}))
	ctx = logging.WithRequestID(ctx, "REQ1")
	e := Event{}.WithContext(ctx)
	if e.TraceID != tid.String() || e.RequestID != "REQ1" {
		t.Errorf("WithContext() = %+v", e)
	}
}

// TestEveryActionIsDeclared checks that each Action constant has a
// declaration, and that each declaration is whole.
func TestEveryActionIsDeclared(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "actions.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var consts []Action
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || vs.Type == nil || len(vs.Values) == 0 {
			return true
		}
		if id, ok := vs.Type.(*ast.Ident); ok && id.Name == "Action" {
			for _, v := range vs.Values {
				if lit, ok := v.(*ast.BasicLit); ok {
					consts = append(consts, Action(strings.Trim(lit.Value, `"`)))
				}
			}
		}
		return true
	})
	if len(consts) < 3 {
		t.Fatalf("found only %d Action constants", len(consts))
	}
	for _, a := range consts {
		if _, ok := Lookup(a); !ok {
			t.Errorf("%s has no declaration in defs", a)
		}
	}
	seen := map[Action]bool{}
	for _, d := range Defs() {
		if seen[d.Action] {
			t.Errorf("%s is declared twice", d.Action)
		}
		seen[d.Action] = true
		if d.Target == "" || d.TargetID == "" || d.Actor == "" || d.Summary == "" || d.After == "" && d.Before == "" {
			t.Errorf("%s's declaration is missing a field: %+v", d.Action, d)
		}
	}
	if len(seen) != len(consts) {
		t.Errorf("%d declarations for %d constants", len(seen), len(consts))
	}
}

func TestReference(t *testing.T) {
	var b bytes.Buffer
	if err := WriteReference(&b); err != nil {
		t.Fatal(err)
	}
	for _, d := range Defs() {
		if !strings.Contains(b.String(), "\n### `"+string(d.Action)+"`\n") {
			t.Errorf("the reference has no section for %s", d.Action)
		}
	}
}
