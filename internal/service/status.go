package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
)

// The status of a server group that no refresh has tried yet.
const statusUnknown = "unknown"

// Status is what /status shows (ADR-0029). It holds no token, key or
// header. Fields are only ever added, so every field is always there: one
// with nothing to say yet is null. The reference page "Service endpoints"
// documents each one, which a test checks.
type Status struct {
	Version  string    `json:"version"`
	Revision string    `json:"revision"`
	Started  time.Time `json:"started"`
	// UptimeSeconds is the time since Started.
	UptimeSeconds float64     `json:"uptime_seconds"`
	Ready         bool        `json:"ready"`
	Live          bool        `json:"live"`
	Schedule      Schedule    `json:"schedule"`
	NetBox        NetBox      `json:"netbox"`
	Groups        []GroupInfo `json:"groups"`
	Tracing       Tracing     `json:"tracing"`
}

// Schedule is the refreshes' schedule, and how they went.
type Schedule struct {
	IntervalSeconds float64 `json:"interval_seconds"`
	TimeoutSeconds  float64 `json:"timeout_seconds"`
	// LastRefresh is the last refresh that finished, or null.
	LastRefresh *Refresh `json:"last_refresh"`
	// LastCompleteRefresh is when the last complete refresh finished, or
	// null.
	LastCompleteRefresh *time.Time `json:"last_complete_refresh"`
	// NextRefresh is when the next refresh starts, or null while one runs.
	NextRefresh *time.Time `json:"next_refresh"`
	Refreshes   Refreshes  `json:"refreshes"`
}

// A Refresh is one refresh that finished.
type Refresh struct {
	Started         time.Time `json:"started"`
	Finished        time.Time `json:"finished"`
	DurationSeconds float64   `json:"duration_seconds"`
	// Outcome is complete, incomplete or failed.
	Outcome string `json:"outcome"`
	// Error is why a failed refresh failed: NetBox's error, or the
	// timeout's. A group's error is with the group.
	Error string `json:"error"`
}

// Refreshes counts the refreshes that finished, by outcome.
type Refreshes struct {
	Complete   int `json:"complete"`
	Incomplete int `json:"incomplete"`
	Failed     int `json:"failed"`
}

// NetBox is NetBox's state.
type NetBox struct {
	URL string `json:"url"`
	// Up is whether the last refresh could read NetBox, or null before the
	// first.
	Up *bool `json:"up"`
	// Error is why NetBox couldn't be read, if it couldn't.
	Error string `json:"error"`
}

// GroupInfo is a server group's last-known state.
type GroupInfo struct {
	Name string `json:"name"`
	// URL is the group's primary's.
	URL string `json:"url"`
	// Status is ok or failed, as of the last time a refresh tried the
	// group's primary, or unknown before that.
	Status      string     `json:"status"`
	LastSuccess *time.Time `json:"last_success"`
	// Error is why the primary couldn't be read, if it couldn't.
	Error string `json:"error"`
	// Counts are from the group's last successful read.
	Counts       drift.Counts  `json:"counts"`
	DriftedZones []DriftedZone `json:"drifted_zones"`
	Problems     int           `json:"problems"`
	Warnings     int           `json:"warnings"`
}

// A DriftedZone is a zone that drifted, as of its group's last successful
// read.
type DriftedZone struct {
	Zone string `json:"zone"`
	// State is drift, missing or inactive_in_netbox.
	State string `json:"state"`
	// Changes counts the zone's RRsets that differ.
	Changes int `json:"changes"`
}

// Tracing says where spans are exported.
type Tracing struct {
	Exported bool   `json:"exported"`
	Endpoint string `json:"endpoint"`
	Protocol string `json:"protocol"`
}

// Status returns the service's status.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.st
	now := s.o.now()
	alive, _ := s.alive(st.lastStart)
	out := Status{
		Version: s.o.Version.Version, Revision: s.o.Version.Commit, Started: s.started.UTC(),
		UptimeSeconds: now.Sub(s.started).Seconds(), Ready: st.ready, Live: alive,
		Schedule: Schedule{
			IntervalSeconds: s.o.Interval.Seconds(), TimeoutSeconds: s.o.Timeout.Seconds(),
			Refreshes: Refreshes{
				Complete: st.refreshes[metrics.OutcomeComplete], Incomplete: st.refreshes[metrics.OutcomeIncomplete],
				Failed: st.refreshes[metrics.OutcomeFailed],
			},
		},
		NetBox:  NetBox{URL: s.o.NetBoxURL, Error: st.netboxError},
		Groups:  []GroupInfo{},
		Tracing: Tracing{Exported: s.o.OTLP.Endpoint != "", Endpoint: s.o.OTLP.Endpoint, Protocol: s.o.OTLP.Protocol},
	}
	if st.ready {
		out.Schedule.LastRefresh = &Refresh{
			Started: st.lastStart.UTC(), Finished: st.lastEnd.UTC(),
			DurationSeconds: st.lastEnd.Sub(st.lastStart).Seconds(), Outcome: st.lastOutcome, Error: st.lastError,
		}
		up := st.netboxUp
		out.NetBox.Up = &up
	}
	if !st.lastComplete.IsZero() {
		out.Schedule.LastCompleteRefresh = utc(st.lastComplete)
	}
	// While a refresh runs, the next one isn't scheduled yet.
	if !st.next.IsZero() && !st.lastStart.After(st.lastEnd) {
		out.Schedule.NextRefresh = utc(st.next)
	}
	for _, g := range s.o.Groups {
		out.Groups = append(out.Groups, groupInfo(g, st.groups[g.Name]))
	}
	return out
}

// groupInfo returns g's info, from gs, its state, which is nil before any
// refresh tried it.
func groupInfo(g Group, gs *groupState) GroupInfo {
	gi := GroupInfo{Name: g.Name, URL: g.URL, Status: statusUnknown, DriftedZones: []DriftedZone{}}
	if gs == nil {
		return gi
	}
	gi.Status, gi.Error = drift.StatusFailed, gs.lastError
	if gs.up {
		gi.Status = drift.StatusOK
	}
	if !gs.lastSuccess.IsZero() {
		gi.LastSuccess = utc(gs.lastSuccess)
		r := gs.report
		gi.Counts, gi.Problems, gi.Warnings = r.Counts, len(r.Problems), len(r.Warnings)
		for _, z := range r.Zones {
			if drift.IsDrifted(z.State) {
				gi.DriftedZones = append(gi.DriftedZones, DriftedZone{Zone: z.Zone, State: z.State, Changes: len(z.Changes)})
			}
		}
	}
	return gi
}

// A GroupView is a server group's configuration and last-known state, for
// the API.
type GroupView struct {
	Group
	Info GroupInfo
	// Report is the group's last successful comparison, or nil if it has
	// none. A kept report is never changed, only replaced by the next, so
	// it's safe to read without the service's lock.
	Report *drift.GroupReport
}

// Groups returns each group's configuration and last-known state, in the
// configuration's order.
func (s *Service) Groups() []GroupView {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]GroupView, len(s.o.Groups))
	for i, g := range s.o.Groups {
		gs := s.st.groups[g.Name]
		out[i] = GroupView{Group: g, Info: groupInfo(g, gs)}
		if gs != nil && !gs.lastSuccess.IsZero() {
			r := gs.report
			out[i].Report = &r
		}
	}
	return out
}

func utc(t time.Time) *time.Time {
	u := t.UTC()
	return &u
}

// status serves the status, as text, or as JSON with ?json=1.
func (s *Service) status(w http.ResponseWriter, r *http.Request) {
	st := s.Status()
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Query().Get("json") == "1" {
		w.Header().Set("Content-Type", "application/json")
		e := json.NewEncoder(w)
		e.SetIndent("", "  ")
		_ = e.Encode(st)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_ = writeStatus(w, st)
}

// writeStatus writes st for a person to read, with aligned tables.
func writeStatus(w io.Writer, st Status) error {
	var b strings.Builder
	p := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	yes := func(v bool) string {
		if v {
			return "yes"
		}
		return "no"
	}
	p("nbpdns %s, up %s, since %s\n", st.Version, (time.Duration(st.UptimeSeconds) * time.Second).Round(time.Second), when(&st.Started))
	p("Ready: %s. Live: %s.\n\n", yes(st.Ready), yes(st.Live))

	sc := st.Schedule
	p("Drift refreshes, every %s, each for up to %s:\n", seconds(sc.IntervalSeconds), seconds(sc.TimeoutSeconds))
	if lr := sc.LastRefresh; lr != nil {
		p("  last:          %s, %s, in %s\n", when(&lr.Finished), lr.Outcome, seconds(lr.DurationSeconds))
		if lr.Error != "" {
			p("                 %s\n", lr.Error)
		}
	} else {
		p("  last:          none yet; there's no drift report until the first refresh finishes\n")
	}
	p("  last complete: %s\n", when(sc.LastCompleteRefresh))
	p("  next:          %s\n", when(sc.NextRefresh))
	p("  finished:      %d complete, %d incomplete, %d failed\n\n", sc.Refreshes.Complete, sc.Refreshes.Incomplete, sc.Refreshes.Failed)

	nb := "not read yet"
	if st.NetBox.Up != nil {
		nb = "read by the last refresh"
		if !*st.NetBox.Up {
			nb = "couldn't be read by the last refresh: " + st.NetBox.Error
		}
	}
	p("NetBox at %s: %s\n\n", st.NetBox.URL, nb)

	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "GROUP\tSTATUS\tLAST SUCCESS\tIN SYNC\tDRIFT\tMISSING\tINACTIVE\tIGNORED\tUNMANAGED\tPROBLEMS\tWARNINGS\tPRIMARY")
	for _, g := range st.Groups {
		c := g.Counts
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%s\n", g.Name, g.Status, when(g.LastSuccess),
			c.InSync, c.Drift, c.Missing, c.Inactive, c.Ignored, c.Unmanaged, g.Problems, g.Warnings, g.URL)
	}
	_ = tw.Flush()

	var drifted, failed [][]string
	for _, g := range st.Groups {
		for _, z := range g.DriftedZones {
			drifted = append(drifted, []string{g.Name, z.Zone, z.State, strconv.Itoa(z.Changes)})
		}
		if g.Error != "" {
			failed = append(failed, []string{g.Name, g.Error})
		}
	}
	section := func(title string, header []string, rows [][]string) {
		if len(rows) == 0 {
			return
		}
		p("\n%s:\n", title)
		tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, strings.Join(header, "\t"))
		for _, r := range rows {
			fmt.Fprintln(tw, strings.Join(r, "\t"))
		}
		_ = tw.Flush()
	}
	section("Drifted zones, as of each group's last successful read", []string{"GROUP", "ZONE", "STATE", "CHANGES"}, drifted)
	section("Groups whose primary couldn't be read", []string{"GROUP", "ERROR"}, failed)

	if st.Tracing.Exported {
		p("\nSpans are exported over %s to %s.\n", st.Tracing.Protocol, st.Tracing.Endpoint)
	} else {
		p("\nSpans aren't exported; set otlp.endpoint to export them.\n")
	}
	p("\nFor this page as JSON, add ?json=1. For each RRset's changes, run nbpdns drift.\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// when writes t in RFC 3339 form, or "none" if it's nil.
func when(t *time.Time) string {
	if t == nil {
		return "none"
	}
	return t.UTC().Format(time.RFC3339)
}

// seconds writes a number of seconds as a duration, such as 5m0s.
func seconds(s float64) string {
	return time.Duration(s * float64(time.Second)).Round(100 * time.Millisecond).String()
}
