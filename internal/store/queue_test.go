package store

import (
	"reflect"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
)

// TestWebhookQueue queues webhooks' refreshes, reads them back as a replica
// that holds the queue up to a row would, and covers them with a recorded
// refresh, on either database (ADR-0048).
func TestWebhookQueue(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.migrated(t)
		ctx := t.Context()
		load := func(covered, since int64) WebhookState {
			t.Helper()
			w, err := s.LoadWebhooks(ctx, covered, since)
			if err != nil {
				t.Fatal(err)
			}
			return w
		}
		if w := load(-1, 0); !reflect.DeepEqual(w, WebhookState{}) {
			t.Fatalf("LoadWebhooks() of a new database = %+v, want nothing", w)
		}

		// An ignored event is the last event, and moves the version, but
		// queues nothing.
		t0 := Stored(time.Date(2026, 11, 6, 1, 2, 3, 456789000, time.UTC))
		ignored := WebhookEvent{Received: t0, Event: "created", ObjectType: "extras.tag", RequestID: "r0", User: "alice"}
		if err := s.RecordWebhook(ctx, ignored, nil); err != nil {
			t.Fatal(err)
		}
		if w := load(0, 0); w.Version != 1 || !reflect.DeepEqual(w.LastEvent, &ignored) || w.Queued != nil || w.Covered != 0 {
			t.Errorf("after an ignored event: %+v, last event %+v", w, w.LastEvent)
		}

		zones := &QueuedRefresh{Received: t0.Add(time.Second), Zones: []drift.ZoneRef{{View: "v", Name: "a.example."}, {View: "v/w", Name: "b.example."}},
			RequestID: "r1", User: "bob", TraceID: "0102030405060708090a0b0c0d0e0f10", SpanID: "0102030405060708"}
		viewChanged := &QueuedRefresh{Received: t0.Add(2 * time.Second), Full: true, Reason: "view v was updated"}
		for _, q := range []*QueuedRefresh{zones, viewChanged} {
			if err := s.RecordWebhook(ctx, WebhookEvent{Received: q.Received, Event: "updated", ObjectType: "netbox_dns.zone"}, q); err != nil {
				t.Fatal(err)
			}
		}
		if zones.ID != 2 || viewChanged.ID != 3 {
			t.Fatalf("queued as %d and %d, want 2 and 3", zones.ID, viewChanged.ID)
		}
		w := load(0, 0)
		if w.Version != 3 || !reflect.DeepEqual(w.Queued, []QueuedRefresh{*zones, *viewChanged}) || w.LastEvent.RequestID != "" ||
			!w.LastEvent.Received.Equal(viewChanged.Received) {
			t.Errorf("queued %+v, version %d, last event %+v", w.Queued, w.Version, w.LastEvent)
		}
		if v, err := s.Versions(ctx); err != nil || v.Webhooks != 3 {
			t.Errorf("Versions() = %+v, %v, want the webhooks' 3", v, err)
		}
		// A replica that holds the queue up to row 2 reads the rest.
		if w := load(0, 2); !reflect.DeepEqual(w.Queued, []QueuedRefresh{*viewChanged}) {
			t.Errorf("after row 2: %+v", w.Queued)
		}

		// A refresh that gathered row 2 covers it once it's recorded.
		last := WebhookRefresh{Started: t0.Add(5 * time.Second), Finished: t0.Add(6 * time.Second), Zones: []string{"v/a.example.", "v/w/b.example."},
			Outcome: "complete", Events: 1, Requests: []WebhookRequest{{ID: "r1", User: "bob"}}}
		rec := full(t0.Add(6 * time.Second))
		rec.Queue = &CoveredQueue{Through: 2, Refresh: last}
		if _, err := s.RecordRefresh(ctx, rec); err != nil {
			t.Fatal(err)
		}
		w = load(0, 3)
		if w.Covered != 2 || w.Version != 4 || !reflect.DeepEqual(w.LastRefresh, &last) {
			t.Errorf("after the refresh: covered %d, version %d, last refresh %+v", w.Covered, w.Version, w.LastRefresh)
		}
		// The mark moved from the 0 that the replica had, so it reads every
		// row queued.
		if !reflect.DeepEqual(w.Queued, []QueuedRefresh{*viewChanged}) {
			t.Errorf("queued %+v once the mark moved", w.Queued)
		}
		if w := load(2, 3); w.Queued != nil {
			t.Errorf("queued %+v after the last row", w.Queued)
		}

		// A full refresh, with no requests, covers the rest.
		rec = full(t0.Add(9 * time.Second))
		rec.Queue = &CoveredQueue{Through: 3, Refresh: WebhookRefresh{Started: t0.Add(8 * time.Second), Finished: t0.Add(9 * time.Second),
			Full: true, Reason: "view v was updated", Outcome: "complete", Events: 1}}
		if _, err := s.RecordRefresh(ctx, rec); err != nil {
			t.Fatal(err)
		}
		w = load(2, 3)
		if w.Covered != 3 || w.Queued != nil || w.LastRefresh.Zones != nil || w.LastRefresh.Requests == nil || !w.LastRefresh.Full {
			t.Errorf("after the full refresh: %+v, last refresh %+v", w, w.LastRefresh)
		}
		// A refresh that covered none of the queue leaves it as it was.
		if _, err := s.RecordRefresh(ctx, full(t0.Add(time.Minute))); err != nil {
			t.Fatal(err)
		}
		if v := load(3, 3); v.Version != w.Version || !reflect.DeepEqual(v.LastRefresh, w.LastRefresh) {
			t.Errorf("a refresh without webhooks changed them: %+v", v)
		}
	})
}
