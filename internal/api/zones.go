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

// zoneOf maps a zone's report onto the API's type.
func zoneOf(z drift.ZoneReport) gen.Zone {
	p := gen.DriftPolicy(z.Policy)
	return gen.Zone{
		Zone: z.Zone, View: optional(z.View), Policy: &p, State: gen.ZoneState(z.State),
		NetboxSerial: serial(z.NetBoxSerial), PowerdnsSerial: serial(z.PowerDNSSerial),
		ChangeCount: int64(len(z.Changes)),
	}
}

// unmanagedZone is a zone on the primary that NetBox doesn't assign to the
// group.
func unmanagedZone(name string) gen.Zone {
	return gen.Zone{Zone: name, State: gen.ZoneStateUnmanaged}
}

// zones returns the zones of report, those NetBox assigns and the
// unmanaged ones, in canonical name order.
func zones(r *drift.GroupReport) []gen.Zone {
	out := make([]gen.Zone, 0, len(r.Zones)+len(r.Unmanaged))
	for _, z := range r.Zones {
		out = append(out, zoneOf(z))
	}
	for _, name := range r.Unmanaged {
		out = append(out, unmanagedZone(name))
	}
	slices.SortFunc(out, func(a, b gen.Zone) int { return dns.CompareNames(a.Zone, b.Zone) })
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
// skips no zone that's still there.
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
	g, ok := s.group(req.Group)
	if !ok {
		return gen.ListZones404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: gen.NotFoundApplicationProblemPlusJSONResponse{Body: noGroup(ctx, req.Group)}}, nil
	}
	out := gen.ZonePage{Items: []gen.Zone{}, AsOf: g.Info.LastSuccess}
	var all []gen.Zone
	if g.Report != nil {
		for _, z := range zones(g.Report) {
			if want == nil || want[z.State] {
				all = append(all, z)
			}
		}
	}
	start := 0
	if p.after != "" {
		start = len(all)
		if i := slices.IndexFunc(all, func(z gen.Zone) bool { return dns.CompareNames(z.Zone, p.after) > 0 }); i >= 0 {
			start = i
		}
	}
	items, more := take(all, start, p.limit)
	out.Items = append(out.Items, items...)
	last := ""
	if len(items) > 0 {
		last = items[len(items)-1].Zone
	}
	out.Self, out.Next = s.o.links(request(ctx), p, last, more)
	return gen.ListZones200JSONResponse{Body: out}, nil
}

// zone finds the zone named name, with or without its final dot, in the
// group named group. It returns the group, the zone's report, which is nil
// for an unmanaged zone, and the zone.
func (s *server) zone(ctx context.Context, group, name string) (service.GroupView, *drift.ZoneReport, gen.Zone, *gen.Problem) {
	notFound := func(detail string) *gen.Problem {
		p := problem(request(ctx), http.StatusNotFound, detail)
		return &p
	}
	g, ok := s.group(group)
	if !ok {
		p := noGroup(ctx, group)
		return g, nil, gen.Zone{}, &p
	}
	if g.Report == nil {
		return g, nil, gen.Zone{}, notFound("Server group " + group + " hasn't been read yet, so its zones aren't known.")
	}
	n, err := dns.ZoneName(name)
	if err != nil {
		return g, nil, gen.Zone{}, notFound("Server group " + group + " has no zone " + name + ".")
	}
	n += "."
	for i, z := range g.Report.Zones {
		if z.Zone == n {
			return g, &g.Report.Zones[i], zoneOf(z), nil
		}
	}
	if slices.Contains(g.Report.Unmanaged, n) {
		return g, nil, unmanagedZone(n), nil
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
		AsOf: asOf(g),
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

// changeKey is a change's key in a cursor: its owner name and type.
func changeKey(name, typ string) string { return name + " " + typ }

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
	start := 0
	if p.after != "" {
		name, typ, _ := strings.Cut(p.after, " ")
		start = len(changes)
		if i := slices.IndexFunc(changes, func(c drift.Change) bool { return dns.CompareRRsets(c.Name, c.Type, name, typ) > 0 }); i >= 0 {
			start = i
		}
	}
	page, more := take(changes, start, p.limit)
	out := gen.ChangePage{Items: make([]gen.Change, len(page)), AsOf: asOf(g)}
	for i, c := range page {
		out.Items[i] = gen.Change{Name: c.Name, Type: c.Type, Kind: gen.ChangeKind(c.Kind), Netbox: side(c.NetBox), Powerdns: side(c.PowerDNS)}
	}
	last := ""
	if len(page) > 0 {
		last = changeKey(page[len(page)-1].Name, page[len(page)-1].Type)
	}
	out.Self, out.Next = s.o.links(request(ctx), p, last, more)
	return gen.ListZoneChanges200JSONResponse{Body: out}, nil
}
