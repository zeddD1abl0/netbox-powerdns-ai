package service

import (
	"context"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
)

// Restore reads back, from the history, each group's last report and the
// last refresh, so that the service serves them, and is ready, before its
// first refresh finishes (ADR-0042). Only the configured groups are
// restored. With nothing to restore, the service is ready once its first
// refresh finishes. A failure is logged, and the service starts with
// nothing. Call it before Run.
func (s *Service) Restore(ctx context.Context) {
	if s.o.History == nil {
		return
	}
	r, err := s.o.History.RestoreDrift(ctx)
	if err != nil {
		s.o.Log.WarnContext(ctx, "couldn't restore the last drift reports, so the service is ready once its first refresh finishes", "err", err)
		return
	}
	m := s.o.Metrics
	s.mu.Lock()
	st := &s.st
	var restored []drift.GroupReport
	for _, g := range s.groups {
		rg, ok := r.Groups[g.Name]
		if !ok {
			continue
		}
		gs := &groupState{up: rg.Status != drift.StatusFailed, lastError: rg.Error, drifted: map[[2]string]bool{}}
		if rg.Report != nil {
			gs.report, gs.lastSuccess, gs.drifted = *rg.Report, rg.LastSuccess, drifted(*rg.Report)
			restored = append(restored, *rg.Report)
		}
		st.groups[g.Name] = gs
	}
	if last := r.Last; last != nil {
		st.lastStart, st.lastEndStart, st.lastEnd = last.Started, last.Started, last.Finished
		st.lastOutcome, st.lastError = last.Outcome, last.Error
		st.netboxUp = last.Outcome != metrics.OutcomeFailed
		if !st.netboxUp {
			st.netboxError = last.Error
		}
		st.lastComplete = r.LastComplete
	}
	if len(restored) > 0 {
		st.ready, st.restored = true, true
		if r.Last != nil {
			st.restoredAsOf = r.Last.Finished
		}
	}
	groups := make(map[string]groupState, len(st.groups))
	for name, gs := range st.groups {
		groups[name] = *gs
	}
	ready, asOf := st.ready, st.restoredAsOf
	s.mu.Unlock()

	for name, gs := range groups {
		publishState(m, name, gs)
	}
	if !ready {
		return
	}
	m.NetBoxUp.WithLabelValues().Set(gauge(r.Last == nil || r.Last.Outcome != metrics.OutcomeFailed))
	if r.Last != nil {
		m.LastRefresh.WithLabelValues().Set(unix(r.Last.Finished))
	}
	if !r.LastComplete.IsZero() {
		m.LastCompleteRefresh.WithLabelValues().Set(unix(r.LastComplete))
	}
	s.o.Log.InfoContext(ctx, "restored the last drift reports, so the service is ready; its first refresh starts now",
		"groups", len(restored), "as_of", asOf, "age_seconds", s.o.now().Sub(asOf).Seconds())
}

// publishState sets the metrics of the group named name from its kept
// state, gs, as restored at start, or moved by a rename.
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
