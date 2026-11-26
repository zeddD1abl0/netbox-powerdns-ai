package service

import (
	"context"
	"errors"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

// Restore loads what the history holds as serve starts, as Reload and
// ReloadWebhooks do, so that the service serves it, and is ready, before a
// refresh of its own finishes (ADR-0042, ADR-0048), and the webhooks queued
// before the start are refreshed. With nothing to load, a leader is ready
// once its first refresh is recorded, and a follower once it loads the
// leader's. A failure is logged, and the service starts with nothing. Call
// it before Run.
func (s *Service) Restore(ctx context.Context) {
	if err := s.reload(ctx, true); err != nil {
		s.o.Log.WarnContext(ctx, "couldn't load the last drift reports from the database, so the service is ready once it serves a refresh",
			"err", err)
	}
	if err := s.reloadWebhooks(ctx); err != nil {
		s.o.Log.WarnContext(ctx, "couldn't load NetBox's webhooks' queue from the database; serve loads it at its next check",
			"err", err)
	}
}

// ReloadWebhooks loads what NetBox's webhooks left in the history, which
// every replica writes to (ADR-0048): their last event and refresh, and
// their queue, of which it reads only the rows queued since it last loaded
// it, unless a refresh covered the queue since. It tells Run, on the leader,
// to gather what it loaded. The change feed calls it whenever the webhooks'
// version moves past the service's.
func (s *Service) ReloadWebhooks(ctx context.Context) error { return s.reloadWebhooks(ctx) }

// WebhooksVersion returns the webhooks' version in the history that the
// service holds, for the change feed to tell whether a replica has taken a
// webhook, or a refresh covered their queue, since (ADR-0048). It's 0 before
// the service loads one, and without a history.
func (s *Service) WebhooksVersion() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st.hooks.version
}

// reloadWebhooks loads what NetBox's webhooks left, as ReloadWebhooks does.
func (s *Service) reloadWebhooks(ctx context.Context) error {
	if s.o.History == nil {
		return nil
	}
	for {
		s.mu.Lock()
		covered, seen := s.st.hooks.covered, s.st.hooks.seen
		s.mu.Unlock()
		w, err := s.o.History.LoadWebhooks(ctx, covered, seen)
		if err != nil {
			return err
		}
		s.mu.Lock()
		h := &s.st.hooks
		// Another reload, or a refresh of this replica's, may have moved the
		// queue meanwhile, so that w's rows don't follow on from it.
		if h.covered != covered || h.seen != seen {
			s.mu.Unlock()
			continue
		}
		if covered >= 0 && w.Covered == covered && w.Version <= h.version {
			s.mu.Unlock()
			return nil
		}
		h.load(w)
		s.setPending()
		s.mu.Unlock()
		s.poke()
		return nil
	}
}

// Reload loads what the deployment's refreshes found from the history, so
// that this replica serves what it holds (ADR-0048): the last full refresh,
// each configured group's last report and result, and NetBox's state and
// zones, of which it reads only those written since it last loaded them. It
// sets the drift metrics from them. The change feed calls it whenever the
// refreshes' version moves past the service's. A refresh that this replica
// recorded is served already, so it loads nothing then. Once it has loaded
// a full refresh, the service is ready.
func (s *Service) Reload(ctx context.Context) error { return s.reload(ctx, false) }

// Version returns the refreshes' version in the history that the service
// serves, for the change feed to tell whether another refresh has been
// recorded since (ADR-0048). It's 0 before the service serves one, and
// without a history.
func (s *Service) Version() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st.version
}

// reload loads what the history holds, as Reload does. At start, it records
// that the service started from what it loaded, and logs it.
func (s *Service) reload(ctx context.Context, start bool) error {
	if s.o.History == nil {
		return nil
	}
	s.mu.Lock()
	since := s.st.version
	s.mu.Unlock()
	for {
		d, err := s.o.History.LoadDrift(ctx, since)
		if err != nil {
			return err
		}
		s.mu.Lock()
		// A refresh of this replica's own, or another reload, may have
		// served as much meanwhile.
		if d.Version <= s.st.version {
			s.mu.Unlock()
			return nil
		}
		nb, ok := loadedNetBox(s.st.netbox, d.NetBox, since)
		if !ok {
			s.mu.Unlock()
			if since == 0 {
				return errors.New("loading the drift report: the index of NetBox's zones names a zone that the database doesn't hold")
			}
			// The service doesn't hold the zones as of since, as it should:
			// it loads them all.
			since = 0
			continue
		}
		groups := s.load(d, nb, start)
		ready, asOf := s.st.ready, s.st.restoredAsOf
		s.mu.Unlock()
		if start && ready {
			s.o.Log.InfoContext(ctx, "loaded the last drift reports from the database, so the service is ready",
				"groups", groups, "as_of", asOf, "age_seconds", s.o.now().Sub(asOf).Seconds())
		} else {
			s.o.Log.DebugContext(ctx, "loaded the drift reports that the database holds", "version", d.Version, "groups", groups)
		}
		return nil
	}
}

// load makes d, which the history holds, with NetBox's zones nb, the
// service's state, and sets the drift metrics from it. At start, it records
// that the service started from it, if it holds a full refresh. It returns
// how many of the configured groups have a report. It's called with s.mu
// held.
func (s *Service) load(d store.DriftState, nb NetBoxView, start bool) int {
	m := s.o.Metrics
	st := &s.st
	old := st.groups
	st.groups = map[string]*groupState{}
	reports := 0
	for _, g := range s.groups {
		was := old[g.Name]
		dg, ok := d.Groups[g.Name]
		if !ok {
			if was != nil {
				m.DropGroup(g.Name)
			}
			continue
		}
		gs := &groupState{up: dg.Status != drift.StatusFailed, lastError: dg.Error, drifted: map[[2]string]bool{}}
		if dg.Report != nil {
			gs.report, gs.lastSuccess, gs.drifted = *dg.Report, dg.LastSuccess, drifted(*dg.Report)
			reports++
		}
		st.groups[g.Name] = gs
		if was != nil && !was.lastSuccess.IsZero() && gs.lastSuccess.IsZero() {
			m.DropGroup(g.Name)
		}
		var removed [][2]string
		if was != nil {
			for k := range was.drifted {
				if !gs.drifted[k] {
					removed = append(removed, k)
				}
			}
		}
		m.GroupUp.WithLabelValues(g.Name).Set(gauge(gs.up))
		if !gs.lastSuccess.IsZero() {
			m.GroupLastSuccess.WithLabelValues(g.Name).Set(unix(gs.lastSuccess))
			setGroup(m, gs.report, gs.drifted, removed)
		}
	}
	if last := d.Last; last != nil {
		st.ready = true
		st.lastEndStart, st.lastEnd, st.lastOutcome, st.lastError = last.Started, last.Finished, last.Outcome, last.Error
		m.LastRefresh.WithLabelValues().Set(unix(last.Finished))
		if start {
			st.restored, st.restoredAsOf = true, last.Finished
		}
	}
	if !d.LastComplete.IsZero() {
		st.lastComplete = d.LastComplete
		m.LastCompleteRefresh.WithLabelValues().Set(unix(d.LastComplete))
	}
	switch d.NetBox.Status {
	case store.NetBoxUp:
		st.netboxRead, st.netboxUp, st.netboxError = true, true, ""
	case store.NetBoxDown:
		st.netboxRead, st.netboxUp, st.netboxError = true, false, d.NetBox.Error
	default:
		st.netboxRead, st.netboxUp, st.netboxError = false, false, ""
	}
	if st.netboxRead {
		m.NetBoxUp.WithLabelValues().Set(gauge(st.netboxUp))
	}
	st.netbox, st.version = nb, d.Version
	return reports
}

// loadedNetBox returns NetBox's zones as n, which the history holds, has
// them: those that it read, and, of the rest, which were written no later
// than the version since, old's. It reports false if old lacks one of those,
// as it can't if it's as of since. old isn't changed, nor any map it shares
// with what the API holds.
func loadedNetBox(old NetBoxView, n store.NetBoxState, since int64) (NetBoxView, bool) {
	// Before a full refresh reads them, there are none.
	if n.ZonesAsOf.IsZero() && len(n.Index) == 0 {
		return NetBoxView{}, true
	}
	read := map[[2]string]dns.Zone{}
	for _, z := range n.Zones {
		read[[2]string{z.View, z.Name}] = z
	}
	v := NetBoxView{AsOf: n.ZonesAsOf, Zones: map[string]map[string]dns.Zone{}}
	for view, names := range n.Index {
		in := make(map[string]dns.Zone, len(names))
		for name := range names {
			z, ok := read[[2]string{view, name}]
			if !ok {
				if z, ok = old.Zones[view][name]; !ok || since == 0 {
					return NetBoxView{}, false
				}
			}
			in[name] = z
		}
		if len(in) > 0 {
			v.Zones[view] = in
		}
	}
	return v, true
}

// publishState sets the metrics of the group named name from its kept
// state, gs, as a rename moves it.
func publishState(m *metrics.Metrics, name string, gs groupState) {
	m.GroupUp.WithLabelValues(name).Set(gauge(gs.up))
	if !gs.lastSuccess.IsZero() {
		m.GroupLastSuccess.WithLabelValues(name).Set(unix(gs.lastSuccess))
		setGroup(m, gs.report, gs.drifted, nil)
	}
}

// gauge returns a Boolean as a gauge's value: 1 or 0.
func gauge(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
