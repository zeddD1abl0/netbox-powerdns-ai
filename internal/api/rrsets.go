package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api/gen"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
)

// isActive reports whether an RRset has an active record: whether nbpdns
// compares it, and the API lists it.
func isActive(s dns.RRset) bool {
	for _, r := range s.Records {
		if r.Active {
			return true
		}
	}
	return false
}

// countActive counts z's RRsets with active records, without copying them.
func countActive(z dns.Zone) int {
	n := 0
	for _, s := range z.RRsets {
		if isActive(s) {
			n++
		}
	}
	return n
}

// activeRRset maps an RRset with active records onto the API's type, with
// only those records.
func activeRRset(s dns.RRset) gen.RRset {
	var recs []gen.Record
	for _, r := range s.Records {
		if r.Active {
			recs = append(recs, gen.Record{Value: r.Value, Managed: r.Managed})
		}
	}
	return gen.RRset{Name: s.Name, Type: s.Type, Ttl: int64(s.TTL), Records: recs}
}

// ListZoneRRsets serves a page of a zone's RRsets as NetBox defines them,
// as of NetBox's last successful read (REQ-047). The zone is the one in
// the view the drift report compares, whatever its state there: a zone
// missing on the primary has its records too. The cursor names the RRset
// the page starts after. Only the page's RRsets are mapped.
func (s *server) ListZoneRRsets(ctx context.Context, req gen.ListZoneRRsetsRequestObject) (gen.ListZoneRRsetsResponseObject, error) {
	notFound := func(detail string) gen.ListZoneRRsetsResponseObject {
		return gen.ListZoneRRsets404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: gen.NotFoundApplicationProblemPlusJSONResponse{
			Body: problem(request(ctx), http.StatusNotFound, detail)}}
	}
	p, err := newPage(req.Params.Limit, req.Params.Cursor, "")
	if prob, ok := badParam(ctx, err); ok {
		return gen.ListZoneRRsets400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse: gen.BadRequestApplicationProblemPlusJSONResponse{Body: prob}}, nil
	}
	g, ok := s.o.Source.Group(req.Group)
	if !ok {
		return gen.ListZoneRRsets404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: gen.NotFoundApplicationProblemPlusJSONResponse{Body: noGroup(ctx, req.Group)}}, nil
	}
	nb := s.o.Source.NetBox()
	if nb.AsOf.IsZero() {
		return notFound("NetBox hasn't been read yet, so its records aren't known."), nil
	}
	n, ok := zoneName(req.Zone)
	if !ok {
		return notFound("Server group " + req.Group + " has no zone " + req.Zone + "."), nil
	}
	z, ok := nb.In(g.Views)(n)
	if !ok {
		return notFound("NetBox has no active zone " + n + " in server group " + req.Group + "'s views, " + strings.Join(g.Views, ", ") + "."), nil
	}
	var all []dns.RRset
	for _, rs := range z.RRsets {
		if isActive(rs) {
			all = append(all, rs)
		}
	}
	start := startAfter(all, p.after, func(rs dns.RRset, key string) int { return compareRRset(rs.Name, rs.Type, key) })
	page, more := take(all, start, p.limit)
	out := gen.RRsetPage{Items: make([]gen.RRset, len(page)), AsOf: nb.AsOf}
	for i, rs := range page {
		out.Items[i] = activeRRset(rs)
	}
	last := ""
	if len(page) > 0 {
		last = rrsetKey(page[len(page)-1].Name, page[len(page)-1].Type)
	}
	out.Self, out.Next = s.o.links(request(ctx), p, last, more)
	return gen.ListZoneRRsets200JSONResponse{Body: out}, nil
}
