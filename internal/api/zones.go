package api

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api/gen"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
)

// serial returns an SOA serial, or nil for 0, which a report gives for a
// side that has no serial.
func serial(n uint32) *int64 {
	if n == 0 {
		return nil
	}
	s := int64(n)
	return &s
}

// zoneName returns the absolute name of a zone, given with or without its
// final dot, in any case, or false if it isn't a zone name.
func zoneName(raw string) (string, bool) {
	n, err := dns.ZoneName(raw)
	if err != nil {
		return "", false
	}
	return n + ".", true
}

// startAfter returns where the page after a cursor's item starts: the
// index of the first item ordered after key, by cmp, or len(items) if none
// is. A key that's gone from the list still finds its place in it.
func startAfter[T any](items []T, key string, cmp func(T, string) int) int {
	if key == "" {
		return 0
	}
	if i := slices.IndexFunc(items, func(it T) bool { return cmp(it, key) > 0 }); i >= 0 {
		return i
	}
	return len(items)
}

// A recordCount returns how many RRsets with active records NetBox defines
// for a zone of a group, by its absolute name, or nil if NetBox has no
// active zone of that name in the view the drift report compares.
type recordCount func(zone string) *int64

// recordCountOf returns g's count, in nb.
func recordCountOf(nb service.NetBoxView, g service.GroupView) recordCount {
	in := nb.In(g.Views)
	return func(zone string) *int64 {
		z, ok := in(zone)
		if !ok {
			return nil
		}
		n := int64(countActive(z))
		return &n
	}
}

// A zoneEntry is a zone of a group's report: NetBox's, with its report, or
// an unmanaged one, without.
type zoneEntry struct {
	name   string
	report *drift.ZoneReport
}

func (e zoneEntry) state() gen.ZoneState {
	if e.report == nil {
		return gen.ZoneStateUnmanaged
	}
	return gen.ZoneState(e.report.State)
}

// zone maps the entry onto the API's type.
func (e zoneEntry) zone(count recordCount) gen.Zone {
	z := e.report
	if z == nil {
		return gen.Zone{Zone: e.name, State: gen.ZoneStateUnmanaged}
	}
	p := gen.DriftPolicy(z.Policy)
	return gen.Zone{
		Zone: z.Zone, View: optional(z.View), Policy: &p, State: gen.ZoneState(z.State),
		NetboxSerial: serial(z.NetBoxSerial), PowerdnsSerial: serial(z.PowerDNSSerial),
		ChangeCount: int64(len(z.Changes)), RrsetCount: count(z.Zone),
	}
}

// entries returns the zones of r, those NetBox assigns and the unmanaged
// ones, in canonical name order.
func entries(r *drift.GroupReport) []zoneEntry {
	out := make([]zoneEntry, 0, len(r.Zones)+len(r.Unmanaged))
	for i := range r.Zones {
		out = append(out, zoneEntry{name: r.Zones[i].Zone, report: &r.Zones[i]})
	}
	for _, name := range r.Unmanaged {
		out = append(out, zoneEntry{name: name})
	}
	slices.SortFunc(out, func(a, b zoneEntry) int { return dns.CompareNames(a.name, b.name) })
	return out
}

// stateFilter checks the zones' state filter, and returns it in the form a
// cursor carries: its states, sorted, without repeats, as state=a,b.
func stateFilter(states *[]gen.ZoneState) (map[gen.ZoneState]bool, string, error) {
	if states == nil || len(*states) == 0 {
		return nil, "", nil
	}
	want := map[gen.ZoneState]bool{}
	var names []string
	for _, st := range *states {
		if !st.Valid() {
			return nil, "", &paramError{"The state " + string(st) + " isn't one of in_sync, drift, missing, inactive_in_netbox, ignored and unmanaged."}
		}
		if !want[st] {
			want[st] = true
			names = append(names, string(st))
		}
	}
	slices.Sort(names)
	return want, "state=" + strings.Join(names, ","), nil
}

// ListZones serves a page of a group's zones, in canonical name order. The
// cursor names the zone the page starts after, so a refresh between pages
// skips no zone that's still there. Only the page's zones are mapped.
func (s *server) ListZones(ctx context.Context, req gen.ListZonesRequestObject) (gen.ListZonesResponseObject, error) {
	bad := func(p gen.Problem) gen.ListZonesResponseObject {
		return gen.ListZones400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse: gen.BadRequestApplicationProblemPlusJSONResponse{Body: p}}
	}
	want, filter, err := stateFilter(req.Params.State)
	if prob, ok := badParam(ctx, err); ok {
		return bad(prob), nil
	}
	p, err := newPage(req.Params.Limit, req.Params.Cursor, filter)
	if prob, ok := badParam(ctx, err); ok {
		return bad(prob), nil
	}
	g, ok := s.o.Source.Group(req.Group)
	if !ok {
		return gen.ListZones404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: gen.NotFoundApplicationProblemPlusJSONResponse{Body: noGroup(ctx, req.Group)}}, nil
	}
	var all []zoneEntry
	if g.Report != nil {
		all = slices.DeleteFunc(entries(g.Report), func(e zoneEntry) bool { return want != nil && !want[e.state()] })
	}
	start := startAfter(all, p.after, func(e zoneEntry, key string) int { return dns.CompareNames(e.name, key) })
	page, more := take(all, start, p.limit)
	count := recordCountOf(s.o.Source.NetBox(), g)
	out := gen.ZonePage{Items: make([]gen.Zone, len(page)), AsOf: g.Info.LastSuccess}
	for i, e := range page {
		out.Items[i] = e.zone(count)
	}
	last := ""
	if len(page) > 0 {
		last = page[len(page)-1].name
	}
	out.Self, out.Next = s.o.links(request(ctx), p, last, more)
	return gen.ListZones200JSONResponse{Body: out}, nil
}

// zone finds the zone named name, with or without its final dot, in the
// group named group. It returns the group, the zone's report, which is nil
// for an unmanaged zone, and the zone, or the problem why it can't.
func (s *server) zone(ctx context.Context, group, name string) (service.GroupView, *drift.ZoneReport, gen.Zone, *gen.Problem) {
	notFound := func(detail string) *gen.Problem {
		p := problem(request(ctx), http.StatusNotFound, detail)
		return &p
	}
	g, ok := s.o.Source.Group(group)
	if !ok {
		p := noGroup(ctx, group)
		return g, nil, gen.Zone{}, &p
	}
	if g.Report == nil {
		return g, nil, gen.Zone{}, notFound("Server group " + group + " hasn't been read yet, so its zones aren't known.")
	}
	n, ok := zoneName(name)
	if !ok {
		return g, nil, gen.Zone{}, notFound("Server group " + group + " has no zone " + name + ".")
	}
	count := recordCountOf(s.o.Source.NetBox(), g)
	for i, z := range g.Report.Zones {
		if z.Zone == n {
			e := zoneEntry{name: n, report: &g.Report.Zones[i]}
			return g, e.report, e.zone(count), nil
		}
	}
	if slices.Contains(g.Report.Unmanaged, n) {
		return g, nil, zoneEntry{name: n}.zone(count), nil
	}
	return g, nil, gen.Zone{}, notFound("Server group " + group + " has no zone " + n + ".")
}

// asOf returns g's last success, which a group with a report has.
func asOf(g service.GroupView) time.Time {
	if g.Info.LastSuccess == nil {
		return time.Time{}
	}
	return *g.Info.LastSuccess
}

// GetZone serves one zone of a group.
func (s *server) GetZone(ctx context.Context, req gen.GetZoneRequestObject) (gen.GetZoneResponseObject, error) {
	g, _, z, prob := s.zone(ctx, req.Group, req.Zone)
	if prob != nil {
		return gen.GetZone404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: gen.NotFoundApplicationProblemPlusJSONResponse{Body: *prob}}, nil
	}
	return gen.GetZone200JSONResponse{Body: gen.ZoneDetail{
		Zone: z.Zone, View: z.View, Policy: z.Policy, State: z.State,
		NetboxSerial: z.NetboxSerial, PowerdnsSerial: z.PowerdnsSerial, ChangeCount: z.ChangeCount,
		RrsetCount: z.RrsetCount, AsOf: asOf(g),
	}}, nil
}

// side maps one side of a change onto the API's type.
func side(s *drift.Side) *gen.Side {
	if s == nil {
		return nil
	}
	values := s.Values
	if values == nil {
		values = []string{}
	}
	return &gen.Side{Ttl: int64(s.TTL), Values: values}
}

// rrsetKey is an RRset's key in a cursor: its owner name and type.
func rrsetKey(name, typ string) string { return name + " " + typ }

// compareRRset orders an RRset, by its name and type, against a cursor's
// key, canonically.
func compareRRset(name, typ, key string) int {
	kname, ktyp, _ := strings.Cut(key, " ")
	return dns.CompareRRsets(name, typ, kname, ktyp)
}

// ListZoneChanges serves a page of a zone's changes, in canonical order.
// The cursor names the RRset the page starts after.
func (s *server) ListZoneChanges(ctx context.Context, req gen.ListZoneChangesRequestObject) (gen.ListZoneChangesResponseObject, error) {
	p, err := newPage(req.Params.Limit, req.Params.Cursor, "")
	if prob, ok := badParam(ctx, err); ok {
		return gen.ListZoneChanges400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse: gen.BadRequestApplicationProblemPlusJSONResponse{Body: prob}}, nil
	}
	g, zr, _, prob := s.zone(ctx, req.Group, req.Zone)
	if prob != nil {
		return gen.ListZoneChanges404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: gen.NotFoundApplicationProblemPlusJSONResponse{Body: *prob}}, nil
	}
	var changes []drift.Change
	if zr != nil {
		changes = zr.Changes
	}
	start := startAfter(changes, p.after, func(c drift.Change, key string) int { return compareRRset(c.Name, c.Type, key) })
	page, more := take(changes, start, p.limit)
	out := gen.ChangePage{Items: make([]gen.Change, len(page)), AsOf: asOf(g)}
	for i, c := range page {
		out.Items[i] = gen.Change{Name: c.Name, Type: c.Type, Kind: gen.ChangeKind(c.Kind), Netbox: side(c.NetBox), Powerdns: side(c.PowerDNS)}
	}
	last := ""
	if len(page) > 0 {
		last = rrsetKey(page[len(page)-1].Name, page[len(page)-1].Type)
	}
	out.Self, out.Next = s.o.links(request(ctx), p, last, more)
	return gen.ListZoneChanges200JSONResponse{Body: out}, nil
}
