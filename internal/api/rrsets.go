package api

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api/gen"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
)

// activeRRsets returns z's RRsets with their active records, as nbpdns
// compares them, leaving out RRsets with none, in z's canonical order.
func activeRRsets(z dns.Zone) []gen.RRset {
	var out []gen.RRset
	for _, s := range z.RRsets {
		var recs []gen.Record
		for _, r := range s.Records {
			if r.Active {
				recs = append(recs, gen.Record{Value: r.Value, Managed: r.Managed})
			}
		}
		if len(recs) > 0 {
			out = append(out, gen.RRset{Name: s.Name, Type: s.Type, Ttl: int64(s.TTL), Records: recs})
		}
	}
	return out
}

// ListZoneRRsets serves a page of a zone's RRsets as NetBox defines them,
// as of NetBox's last successful read (REQ-047). The zone is the one in
// the group's views, whatever its state in the drift report: a zone
// missing on the primary has its records too. The cursor names the RRset
// the page starts after.
func (s *server) ListZoneRRsets(ctx context.Context, req gen.ListZoneRRsetsRequestObject) (gen.ListZoneRRsetsResponseObject, error) {
	notFound := func(p gen.Problem) gen.ListZoneRRsetsResponseObject {
		return gen.ListZoneRRsets404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: gen.NotFoundApplicationProblemPlusJSONResponse{Body: p}}
	}
	p, err := newPage(req.Params.Limit, req.Params.Cursor, "")
	if prob, ok := badParam(ctx, err); ok {
		return gen.ListZoneRRsets400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse: gen.BadRequestApplicationProblemPlusJSONResponse{Body: prob}}, nil
	}
	g, ok := s.group(req.Group)
	if !ok {
		return notFound(noGroup(ctx, req.Group)), nil
	}
	nb := s.o.Source.NetBox()
	if nb.AsOf.IsZero() {
		return notFound(problem(request(ctx), http.StatusNotFound, "NetBox hasn't been read yet, so its records aren't known.")), nil
	}
	n, err := dns.ZoneName(req.Zone)
	if err != nil {
		return notFound(problem(request(ctx), http.StatusNotFound, "Server group "+req.Group+" has no zone "+req.Zone+".")), nil
	}
	n += "."
	z, ok := nb.Zone(g.Views, n)
	if !ok {
		return notFound(problem(request(ctx), http.StatusNotFound,
			"NetBox has no active zone "+n+" in server group "+req.Group+"'s views, "+strings.Join(g.Views, ", ")+".")), nil
	}
	all := activeRRsets(z)
	start := 0
	if p.after != "" {
		name, typ, _ := strings.Cut(p.after, " ")
		start = len(all)
		if i := slices.IndexFunc(all, func(r gen.RRset) bool { return dns.CompareRRsets(r.Name, r.Type, name, typ) > 0 }); i >= 0 {
			start = i
		}
	}
	items, more := take(all, start, p.limit)
	out := gen.RRsetPage{Items: append([]gen.RRset{}, items...), AsOf: nb.AsOf}
	last := ""
	if len(items) > 0 {
		last = changeKey(items[len(items)-1].Name, items[len(items)-1].Type)
	}
	out.Self, out.Next = s.o.links(request(ctx), p, last, more)
	return gen.ListZoneRRsets200JSONResponse{Body: out}, nil
}
