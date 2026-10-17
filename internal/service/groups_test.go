package service

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/webhook"
)

// names returns the names of the service's groups.
func names(s *Service) string {
	var out []string
	for _, g := range s.Groups() {
		out = append(out, g.Name)
	}
	return strings.Join(out, ",")
}

// TestSetGroups adds, removes and renames groups while the service runs,
// and checks their state, their metrics and the views that webhooks match.
func TestSetGroups(t *testing.T) {
	a := Group{ID: 1, Name: "site-a", Views: []string{"v"}, ManagedBy: "cli"}
	b := Group{ID: 2, Name: "site-b", Views: []string{"w"}, ManagedBy: "file"}
	c := Group{ID: 3, Name: "site-c", Views: []string{"x"}, ManagedBy: "cli"}
	s, _, _ := testService(t, Options{Groups: []Group{a, b, c}})
	m := s.o.Metrics
	t0 := time.Now()
	s.record(t.Context(), s.o.Log, report(group("site-a", drift.StateDrift), group("site-b", drift.StateInSync),
		group("site-c", drift.StateDrift)), nil, t0, t0)

	renamed := a
	renamed.Name = "site-z"
	d := Group{ID: 4, Name: "site-d", Views: []string{"y"}, ManagedBy: "cli"}
	s.SetGroups([]Group{renamed, b, d})
	if got := names(s); got != "site-z,site-b,site-d" {
		t.Errorf("groups %s, want site-z,site-b,site-d", got)
	}
	// The renamed group keeps its state, under its new name.
	z, ok := s.Group("site-z")
	if !ok || z.Report == nil || z.Report.Group != "site-z" || z.Info.Status != drift.StatusOK || len(z.Info.DriftedZones) != 1 {
		t.Errorf("site-z = %+v, want site-a's state", z)
	}
	if g, _ := s.Group("site-d"); g.Info.Status != statusUnknown || g.Report != nil {
		t.Errorf("site-d = %+v, want nothing known", g)
	}
	if _, ok := s.Group("site-c"); ok {
		t.Error("site-c is still a group")
	}
	// The removed and the old names' series go; the new name's are set.
	for _, gone := range []string{"site-a", "site-c"} {
		if n := testutil.CollectAndCount(m.GroupUp, "nbpdns_server_group_up"); n != 2 ||
			strings.Contains(gathered(t, m), `group="`+gone+`"`) {
			t.Errorf("%s's series are left, or the groups' up has %d series, want 2:\n%s", gone, n, gathered(t, m))
		}
	}
	if testutil.ToFloat64(m.GroupUp.WithLabelValues("site-z")) != 1 ||
		testutil.ToFloat64(m.ZoneDrifted.WithLabelValues("site-z", "a.example.", drift.StateDrift)) != 1 {
		t.Errorf("site-z's series aren't set:\n%s", gathered(t, m))
	}

	// Webhooks match the groups' views now.
	if s.Notify(t.Context(), event("r1", "u"), webhook.Refresh{Zones: []webhook.Zone{{View: "x", Name: "a.example."}}}) {
		t.Error("a zone of the removed group's view was queued")
	}
	if !s.Notify(t.Context(), event("r2", "u"), webhook.Refresh{Zones: []webhook.Zone{{View: "y", Name: "a.example."}}}) {
		t.Error("a zone of the new group's view wasn't queued")
	}

	// A refresh that the change overtook keeps nothing for the groups that
	// are gone.
	s.record(t.Context(), s.o.Log, report(group("site-a", drift.StateInSync), group("site-c", drift.StateInSync),
		group("site-b", drift.StateInSync)), nil, t0, t0)
	if got := names(s); got != "site-z,site-b,site-d" || strings.Contains(gathered(t, m), `group="site-c"`) {
		t.Errorf("an overtaken refresh brought back a group: %s\n%s", got, gathered(t, m))
	}
	if status := s.Status(); len(status.Groups) != 3 || status.Groups[0].Name != "site-z" {
		t.Errorf("status groups = %+v", status.Groups)
	}

	// A new group that reuses a removed one's name, with another ID, starts
	// with nothing known, and the removed one's series go.
	reused := Group{ID: 5, Name: "site-b", Views: []string{"w"}, ManagedBy: "cli"}
	s.SetGroups([]Group{renamed, reused, d})
	if g, _ := s.Group("site-b"); g.Info.Status != statusUnknown || g.Report != nil {
		t.Errorf("the new site-b = %+v, want nothing known, not the removed site-b's state", g)
	}
	if strings.Contains(gathered(t, m), `group="site-b"`) {
		t.Errorf("the removed site-b's series are left:\n%s", gathered(t, m))
	}
}

// TestRefreshNow brings the next refresh forward, an hour early.
func TestRefreshNow(t *testing.T) {
	var n atomic.Int32
	s, _, _ := testService(t, Options{Interval: time.Hour, Refresh: func(context.Context, []drift.ZoneRef) (drift.Report, error) {
		n.Add(1)
		return report(), nil
	}})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()
	waitFor := func(want int32) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for n.Load() < want {
			if time.Now().After(deadline) {
				t.Fatalf("%d refreshes, want %d", n.Load(), want)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	waitFor(1)
	s.RefreshNow()
	waitFor(2)
	time.Sleep(50 * time.Millisecond)
	if got := n.Load(); got != 2 {
		t.Errorf("%d refreshes, want 2: RefreshNow asked for one", got)
	}
}
