package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api/gen"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

// A History is the drift history that the API serves (ADR-0042): the
// store's.
type History interface {
	ZoneHistory(ctx context.Context, group, zone string, before int64, limit int) ([]store.ZoneHistoryRow, error)
	Refreshes(ctx context.Context, group string, before int64, limit int) ([]store.RefreshRow, error)
}

// before returns the ID that a newest-first page starts before: its
// cursor's, or 0 for the first page.
func before(ctx context.Context, p page) (int64, *gen.Problem) {
	if p.after == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(p.after, 10, 64)
	if err != nil || id < 1 {
		prob := problem(request(ctx), http.StatusBadRequest, "The cursor isn't one that this API gave.")
		return 0, &prob
	}
	return id, nil
}

// ListZoneHistory serves a page of a zone's drift history, newest first.
// The cursor names the row the page starts before. A zone with no history,
// such as one the group has never had, has an empty page: its history may
// outlive the zone.
func (s *server) ListZoneHistory(ctx context.Context, req gen.ListZoneHistoryRequestObject) (gen.ListZoneHistoryResponseObject, error) {
	bad := func(p gen.Problem) gen.ListZoneHistoryResponseObject {
		return gen.ListZoneHistory400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse: gen.BadRequestApplicationProblemPlusJSONResponse{Body: p}}
	}
	notFound := func(p gen.Problem) gen.ListZoneHistoryResponseObject {
		return gen.ListZoneHistory404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: gen.NotFoundApplicationProblemPlusJSONResponse{Body: p}}
	}
	p, err := newPage(req.Params.Limit, req.Params.Cursor, "")
	if prob, ok := badParam(ctx, err); ok {
		return bad(prob), nil
	}
	from, prob := before(ctx, p)
	if prob != nil {
		return bad(*prob), nil
	}
	if _, ok := s.o.Source.Group(req.Group); !ok {
		return notFound(noGroup(ctx, req.Group)), nil
	}
	name, ok := zoneName(req.Zone)
	if !ok {
		return notFound(problem(request(ctx), http.StatusNotFound, "Server group "+req.Group+" has no zone "+req.Zone+".")), nil
	}
	out := gen.ZoneHistoryPage{Items: []gen.ZoneHistoryEntry{}}
	var rows []store.ZoneHistoryRow
	if s.o.History != nil {
		if rows, err = s.o.History.ZoneHistory(ctx, req.Group, name, from, p.limit+1); err != nil {
			return nil, err
		}
	}
	more := len(rows) > p.limit
	rows = rows[:min(len(rows), p.limit)]
	for _, r := range rows {
		out.Items = append(out.Items, gen.ZoneHistoryEntry{
			Time: r.At, RefreshId: r.RefreshID, State: gen.HistoryState(r.State), View: r.View,
			DriftPolicy: gen.DriftPolicy(r.Policy), NetboxSerial: serial(r.NetBoxSerial), PowerdnsSerial: serial(r.PowerDNSSerial),
			ChangeCount: int64(len(r.Changes)), Changes: apiChanges(r.Changes),
		})
	}
	last := ""
	if len(rows) > 0 {
		last = strconv.FormatInt(rows[len(rows)-1].ID, 10)
	}
	out.Self, out.Next = s.o.links(request(ctx), p, last, more)
	return gen.ListZoneHistory200JSONResponse{Body: out}, nil
}

// ListRefreshes serves a page of the refreshes, newest first, each with the
// group's result in it. The cursor names the refresh the page starts
// before.
func (s *server) ListRefreshes(ctx context.Context, req gen.ListRefreshesRequestObject) (gen.ListRefreshesResponseObject, error) {
	bad := func(p gen.Problem) gen.ListRefreshesResponseObject {
		return gen.ListRefreshes400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse: gen.BadRequestApplicationProblemPlusJSONResponse{Body: p}}
	}
	p, err := newPage(req.Params.Limit, req.Params.Cursor, "")
	if prob, ok := badParam(ctx, err); ok {
		return bad(prob), nil
	}
	from, prob := before(ctx, p)
	if prob != nil {
		return bad(*prob), nil
	}
	if _, ok := s.o.Source.Group(req.Group); !ok {
		return gen.ListRefreshes404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: gen.NotFoundApplicationProblemPlusJSONResponse{Body: noGroup(ctx, req.Group)}}, nil
	}
	out := gen.RefreshPage{Items: []gen.RefreshEntry{}}
	var rows []store.RefreshRow
	if s.o.History != nil {
		if rows, err = s.o.History.Refreshes(ctx, req.Group, from, p.limit+1); err != nil {
			return nil, err
		}
	}
	more := len(rows) > p.limit
	rows = rows[:min(len(rows), p.limit)]
	for _, r := range rows {
		e := gen.RefreshEntry{
			Id: r.ID, Kind: gen.RefreshEntryKind(r.Kind), Cause: gen.RefreshEntryCause(r.Cause), Started: r.Started, Finished: r.Finished,
			Outcome: gen.RefreshEntryOutcome(r.Outcome), Error: optional(r.Error), NetboxRequestIds: r.NetBoxRequestIDs, TraceId: r.TraceID,
		}
		if e.NetboxRequestIds == nil {
			e.NetboxRequestIds = []string{}
		}
		if r.Zones != nil {
			e.Zones = &r.Zones
		}
		if g := r.Group; g != nil {
			e.Group = &gen.GroupResult{Status: gen.GroupResultStatus(g.Status), Error: optional(g.Error)}
			if g.Status == drift.StatusOK {
				e.Group.Counts = zoneCounts(g.Counts)
			}
		}
		out.Items = append(out.Items, e)
	}
	last := ""
	if len(rows) > 0 {
		last = strconv.FormatInt(rows[len(rows)-1].ID, 10)
	}
	out.Self, out.Next = s.o.links(request(ctx), p, last, more)
	return gen.ListRefreshes200JSONResponse{Body: out}, nil
}

// apiChanges maps a zone's changes onto the API's type.
func apiChanges(cs []drift.Change) []gen.Change {
	out := make([]gen.Change, len(cs))
	for i, c := range cs {
		out[i] = gen.Change{Name: c.Name, Type: c.Type, Kind: gen.ChangeKind(c.Kind), Netbox: side(c.NetBox), Powerdns: side(c.PowerDNS)}
	}
	return out
}
