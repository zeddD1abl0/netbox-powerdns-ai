// Package audit defines nbpdns's audit trail (ADR-0039): the event that each
// change of state makes, the actions it may record, and the SHA-384 chain
// that makes an edited, deleted or inserted event show. The store keeps the
// events, each in the transaction of the change it records.
package audit

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/logging"
)

// TimeLayout is how an event's time is written, in the store and in its
// hash: in UTC, to the microsecond, at a fixed width.
const TimeLayout = "2006-01-02T15:04:05.000000Z"

// The kinds of actor.
const (
	// ActorOSUser is a person at the command line, by their operating
	// system account, until M10 authenticates people.
	ActorOSUser = "os-user"
	// ActorSystem is nbpdns itself, such as serve at its start.
	ActorSystem = "system"
	// ActorFile is the config file, whose groups serve mirrors at its start.
	ActorFile = "file"
)

// An Actor is who, or what, made a change.
type Actor struct {
	Kind string
	// ID names the actor among its kind, such as a user name. The system
	// has none.
	ID string
}

// String returns the actor as kind:id, such as os-user:jordan, or as its
// kind alone if it has no ID.
func (a Actor) String() string {
	if a.ID == "" {
		return a.Kind
	}
	return a.Kind + ":" + a.ID
}

// ParseActor reads an actor written as String writes it.
func ParseActor(s string) Actor {
	kind, id, _ := strings.Cut(s, ":")
	return Actor{Kind: kind, ID: id}
}

// System returns nbpdns itself, as an actor.
func System() Actor { return Actor{Kind: ActorSystem} }

// File returns the config file at path, as an actor.
func File(path string) Actor { return Actor{Kind: ActorFile, ID: path} }

// OSUser returns the operating system's user running this process: by name,
// or by number, uid:N, if the system doesn't name it, as in a container
// without a user database.
func OSUser() Actor {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return Actor{Kind: ActorOSUser, ID: u.Username}
	}
	return Actor{Kind: ActorOSUser, ID: "uid:" + strconv.Itoa(os.Getuid())}
}

// A Target is what a change was made to.
type Target struct {
	// Type is its kind, which its action's declaration names, such as
	// setting.
	Type string
	// ID names it among its type, such as drift.interval.
	ID string
}

// String returns the target as type:id.
func (t Target) String() string { return t.Type + ":" + t.ID }

// An Outcome says whether the change was made.
type Outcome string

// The outcomes.
const (
	Success Outcome = "success"
	Failure Outcome = "failure"
)

// An Event records one change of state.
type Event struct {
	// Seq is the event's place in the trail, from 1, with no gaps but
	// those a prune leaves before the first event.
	Seq    int64
	Time   time.Time
	Actor  Actor
	Action Action
	Target Target
	// Before and After are the target's values, as JSON, before and after
	// the change, or nil if it had none, such as before it was created.
	// Secrets are never among them.
	Before, After json.RawMessage
	// Reason is why the change was made, as its maker gave it, if they
	// did.
	Reason  string
	Outcome Outcome
	// TraceID and RequestID are those of the command or request that made
	// the change, as its log lines carry them.
	TraceID, RequestID string
	// SourceIP is the address that the change came from, if it came over
	// the network.
	SourceIP string
	// PrevHash is the Hash of the event before it, or empty for the
	// trail's first event.
	PrevHash string
	// Hash is the hex SHA-384 of the event's canonical form, which holds
	// every other field, PrevHash among them.
	Hash string
}

// WithContext returns e with the trace and request IDs of ctx.
func (e Event) WithContext(ctx context.Context) Event {
	if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
		e.TraceID = sc.TraceID().String()
	}
	e.RequestID = logging.RequestID(ctx)
	return e
}

// canonical is an event as it's hashed: every field but the hash, in a fixed
// order. Before and After are their JSON text, so that the hash covers the
// bytes stored.
type canonical struct {
	Seq       int64   `json:"seq"`
	Time      string  `json:"time"`
	Actor     string  `json:"actor"`
	Action    string  `json:"action"`
	Target    string  `json:"target"`
	Before    *string `json:"before"`
	After     *string `json:"after"`
	Reason    string  `json:"reason"`
	Outcome   string  `json:"outcome"`
	TraceID   string  `json:"trace_id"`
	RequestID string  `json:"request_id"`
	SourceIP  string  `json:"source_ip"`
	PrevHash  string  `json:"prev_hash"`
}

// Sum returns the event's hash, the hex SHA-384 of its canonical form: a JSON
// object of its fields but Hash, in a fixed order, its time in TimeLayout,
// its actor and target as their String forms, and Before and After as
// strings holding their JSON, or null.
func (e Event) Sum() string {
	text := func(j json.RawMessage) *string {
		if j == nil {
			return nil
		}
		s := string(j)
		return &s
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	// Encoding strings and integers can't fail.
	_ = enc.Encode(canonical{
		Seq: e.Seq, Time: e.Time.UTC().Format(TimeLayout), Actor: e.Actor.String(), Action: string(e.Action),
		Target: e.Target.String(), Before: text(e.Before), After: text(e.After), Reason: e.Reason,
		Outcome: string(e.Outcome), TraceID: e.TraceID, RequestID: e.RequestID, SourceIP: e.SourceIP,
		PrevHash: e.PrevHash,
	})
	sum := sha512.Sum384(bytes.TrimSuffix(b.Bytes(), []byte("\n")))
	return hex.EncodeToString(sum[:])
}

// Value returns v as JSON, for an event's Before or After.
func Value(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encoding an audit event's value: %w", err)
	}
	return b, nil
}

// An Anchor is where the trail starts once a prune has deleted its oldest
// events: the last deleted event's Seq and Hash, which the first remaining
// one links to.
type Anchor struct {
	Seq  int64
	Hash string
}

// BrokenError says where, and how, the chain breaks.
type BrokenError struct {
	// Seq is the event at which it breaks.
	Seq     int64
	Problem string
}

func (e *BrokenError) Error() string {
	return fmt.Sprintf("the audit trail is broken at event %d: %s", e.Seq, e.Problem)
}

// A Verifier checks a trail's events, given in order, against its chain.
type Verifier struct {
	next  int64  // the Seq that the next event must have
	prev  string // the hash that the next event must link to
	count int
}

// NewVerifier returns a Verifier for a trail that starts at anchor, or, if
// anchor is nil, at its first event.
func NewVerifier(anchor *Anchor) *Verifier {
	if anchor == nil {
		return &Verifier{next: 1}
	}
	return &Verifier{next: anchor.Seq + 1, prev: anchor.Hash}
}

// Check checks the next event, and returns a *BrokenError if the chain
// breaks there.
func (v *Verifier) Check(e Event) error {
	switch {
	case e.Seq > v.next:
		return &BrokenError{Seq: e.Seq, Problem: fmt.Sprintf("the events from %d to %d are missing", v.next, e.Seq-1)}
	case e.Seq < v.next:
		return &BrokenError{Seq: e.Seq, Problem: fmt.Sprintf("it comes after event %d, out of order", v.next-1)}
	case e.Sum() != e.Hash:
		return &BrokenError{Seq: e.Seq, Problem: "its contents don't match its hash, so it was changed"}
	case e.PrevHash != v.prev:
		return &BrokenError{Seq: e.Seq, Problem: "it doesn't link to the event before it, so that event was changed or replaced"}
	}
	v.next, v.prev = e.Seq+1, e.Hash
	v.count++
	return nil
}

// Count returns how many events have passed.
func (v *Verifier) Count() int { return v.count }
