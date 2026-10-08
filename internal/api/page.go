package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api/gen"
)

// The page sizes (ADR-0033), as api/openapi.yaml's Limit says.
const (
	defaultLimit = 100
	maxLimit     = 1000
)

// A cursor is where a page starts: after the item keyed After, in a list
// read with the filter Filter. Clients get it as base64url of its JSON, and
// treat it as opaque.
type cursor struct {
	After  string `json:"a"`
	Filter string `json:"f,omitempty"`
}

func (c cursor) String() string {
	b, err := json.Marshal(c)
	if err != nil {
		panic(err) // Two strings always marshal.
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// A paramError is a list's invalid limit or cursor: the request's fault, a
// 400 problem.
type paramError struct{ detail string }

func (e *paramError) Error() string { return e.detail }

// A page is a list's page: where it starts and how many it may hold.
type page struct {
	limit int
	// after is the key of the item it starts after, or "" for the first
	// page.
	after  string
	filter string
}

// newPage checks a list's limit and cursor, for a list read with filter.
func newPage(limit *int32, cur *string, filter string) (page, error) {
	p := page{limit: defaultLimit, filter: filter}
	if limit != nil {
		if *limit < 1 || *limit > maxLimit {
			return p, &paramError{fmt.Sprintf("The limit must be from 1 to %d.", maxLimit)}
		}
		p.limit = int(*limit)
	}
	if cur == nil || *cur == "" {
		return p, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(*cur)
	var c cursor
	if err != nil || json.Unmarshal(b, &c) != nil || c.After == "" {
		return p, &paramError{"The cursor isn't one that this API gave."}
	}
	if c.Filter != filter {
		return p, &paramError{"The cursor is for a list read with other filters; start again without it."}
	}
	p.after = c.After
	return p, nil
}

// take returns the page of items, which start at index start, and whether
// more follow.
func take[T any](items []T, start, limit int) ([]T, bool) {
	start = min(start, len(items))
	end := min(start+limit, len(items))
	return items[start:end], end < len(items)
}

// links returns the absolute URLs of the page r asked for, and of the next
// one, which starts after the item keyed last, or nil if there's none.
// The path is the request's escaped one, so a zone named with a / or a %
// keeps its escapes.
func (o Options) links(r *http.Request, p page, last string, more bool) (string, *string) {
	self := o.link(r, r.URL.EscapedPath(), r.URL.Query())
	if !more {
		return self, nil
	}
	q := r.URL.Query()
	q.Set("cursor", cursor{After: last, Filter: p.filter}.String())
	q.Set("limit", strconv.Itoa(p.limit))
	next := o.link(r, r.URL.EscapedPath(), q)
	return self, &next
}

type requestKey struct{}

// withRequest gives the strict handlers their request, through ctx, for its
// links and problems.
func withRequest(f gen.StrictHandlerFunc, _ string) gen.StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, req any) (any, error) {
		return f(context.WithValue(ctx, requestKey{}, r), w, r, req)
	}
}

// request returns ctx's request, which withRequest put there.
func request(ctx context.Context) *http.Request {
	r, _ := ctx.Value(requestKey{}).(*http.Request)
	return r
}
