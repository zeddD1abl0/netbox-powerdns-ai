package api

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api/gen"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
)

// serverGroup maps a group's configuration and last-known state onto the
// API's type.
func serverGroup(g service.GroupView) gen.ServerGroup {
	out := gen.ServerGroup{
		Name: g.Name, PrimaryUrl: g.URL, Views: g.Views, DriftPolicy: gen.DriftPolicy(g.DriftPolicy),
		Status: gen.ServerGroupStatus(g.Info.Status), Error: optional(g.Info.Error), LastSuccess: g.Info.LastSuccess,
	}
	if out.Views == nil {
		out.Views = []string{}
	}
	if r := g.Report; r != nil {
		out.Counts = zoneCounts(r.Counts)
		problems, warnings := int64(len(r.Problems)), int64(len(r.Warnings))
		out.ProblemCount, out.WarningCount = &problems, &warnings
	}
	return out
}

func zoneCounts(c drift.Counts) *gen.ZoneCounts {
	return &gen.ZoneCounts{
		InSync: int64(c.InSync), Drift: int64(c.Drift), Missing: int64(c.Missing),
		InactiveInNetbox: int64(c.Inactive), Ignored: int64(c.Ignored), Unmanaged: int64(c.Unmanaged),
	}
}

// badParam returns err's problem, if it's a paramError, for a 400.
func badParam(ctx context.Context, err error) (gen.Problem, bool) {
	var pe *paramError
	if !errors.As(err, &pe) {
		return gen.Problem{}, false
	}
	return problem(request(ctx), http.StatusBadRequest, pe.detail), true
}

// ListServerGroups serves a page of the groups, in the configuration's
// order. The cursor names the group the page starts after.
func (s *server) ListServerGroups(ctx context.Context, req gen.ListServerGroupsRequestObject) (gen.ListServerGroupsResponseObject, error) {
	p, err := newPage(req.Params.Limit, req.Params.Cursor, "")
	if prob, ok := badParam(ctx, err); ok {
		return gen.ListServerGroups400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse: gen.BadRequestApplicationProblemPlusJSONResponse{Body: prob}}, nil
	}
	groups := s.o.Source.Groups()
	start := 0
	if p.after != "" {
		i := slices.IndexFunc(groups, func(g service.GroupView) bool { return g.Name == p.after })
		if i < 0 {
			prob := problem(request(ctx), http.StatusBadRequest, "The cursor's server group isn't configured any more; start again without it.")
			return gen.ListServerGroups400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse: gen.BadRequestApplicationProblemPlusJSONResponse{Body: prob}}, nil
		}
		start = i + 1
	}
	items, more := take(groups, start, p.limit)
	out := gen.ServerGroupPage{Items: make([]gen.ServerGroup, len(items))}
	for i, g := range items {
		out.Items[i] = serverGroup(g)
	}
	last := ""
	if len(items) > 0 {
		last = items[len(items)-1].Name
	}
	out.Self, out.Next = s.o.links(request(ctx), p, last, more)
	return gen.ListServerGroups200JSONResponse{Body: out}, nil
}

// group returns the group named name, if there's one.
func (s *server) group(name string) (service.GroupView, bool) {
	for _, g := range s.o.Source.Groups() {
		if g.Name == name {
			return g, true
		}
	}
	return service.GroupView{}, false
}

// noGroup returns the problem for a group that isn't configured.
func noGroup(ctx context.Context, name string) gen.Problem {
	return problem(request(ctx), http.StatusNotFound, "No server group is named "+name+".")
}

// GetServerGroup serves one group.
func (s *server) GetServerGroup(ctx context.Context, req gen.GetServerGroupRequestObject) (gen.GetServerGroupResponseObject, error) {
	g, ok := s.group(req.Group)
	if !ok {
		return gen.GetServerGroup404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: gen.NotFoundApplicationProblemPlusJSONResponse{Body: noGroup(ctx, req.Group)}}, nil
	}
	return gen.GetServerGroup200JSONResponse{Body: serverGroup(g)}, nil
}
