package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api/gen"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
)

// threeGroups are a group compared, one whose primary failed after a
// success, and one no refresh has tried.
func threeGroups() fakeSource {
	last := at("2026-10-08T01:10:02Z")
	report := &drift.GroupReport{
		Group: "site-a", Status: drift.StatusOK,
		Counts:   drift.Counts{InSync: 980, Drift: 15, Missing: 5, Ignored: 1, Unmanaged: 2},
		Problems: []dns.Problem{{Zone: "example.com.", Detail: "a CNAME beside other data"}},
		Warnings: []string{},
	}
	return fakeSource{groups: []service.GroupView{
		{
			Group:  service.Group{Name: "site-a", URL: "https://pdns-a.example.com:8443", Views: []string{"_default_", "internal"}, DriftPolicy: "report"},
			Info:   service.GroupInfo{Name: "site-a", Status: "ok", LastSuccess: &last},
			Report: report,
		},
		{
			Group:  service.Group{Name: "site-b", URL: "https://pdns-b.example.com:8443", Views: []string{"_default_"}, DriftPolicy: "enforce"},
			Info:   service.GroupInfo{Name: "site-b", Status: "failed", Error: "the primary isn't reachable", LastSuccess: &last},
			Report: &drift.GroupReport{Group: "site-b", Status: drift.StatusOK, Problems: []dns.Problem{}, Warnings: []string{"w"}},
		},
		{
			Group: service.Group{Name: "site-c", URL: "http://pdns-c.example.com:8081", Views: []string{"lab"}, DriftPolicy: "ignore"},
			Info:  service.GroupInfo{Name: "site-c", Status: "unknown"},
		},
	}}
}

func decode[T any](t *testing.T, r reply) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(r.body, &v); err != nil {
		t.Fatalf("%v: %s", err, r.body)
	}
	return v
}

func TestListServerGroups(t *testing.T) {
	r := get(t, threeGroups(), http.MethodGet, "/api/server-groups")
	if r.code != http.StatusOK {
		t.Fatalf("%d: %s", r.code, r.body)
	}
	page := decode[gen.ServerGroupPage](t, r)
	if len(page.Items) != 3 || page.Next != nil || page.Self != "http://example.com/api/server-groups" {
		t.Fatalf("page %+v", page)
	}
	a, b, c := page.Items[0], page.Items[1], page.Items[2]
	if a.Name != "site-a" || a.Status != gen.ServerGroupStatusOk || a.Counts == nil || a.Counts.InSync != 980 ||
		a.Counts.Unmanaged != 2 || *a.ProblemCount != 1 || *a.WarningCount != 0 || a.Error != nil ||
		a.DriftPolicy != gen.DriftPolicyReport || len(a.Views) != 2 {
		t.Errorf("site-a: %+v", a)
	}
	if b.Status != gen.ServerGroupStatusFailed || b.Error == nil || *b.Error != "the primary isn't reachable" ||
		b.LastSuccess == nil || *b.WarningCount != 1 {
		t.Errorf("site-b: %+v", b)
	}
	if c.Status != gen.ServerGroupStatusUnknown || c.LastSuccess != nil || c.Counts != nil || c.ProblemCount != nil ||
		c.DriftPolicy != gen.DriftPolicyIgnore {
		t.Errorf("site-c: %+v", c)
	}
	// Nothing known is null, not absent.
	if !strings.Contains(string(r.body), `"counts":null`) || !strings.Contains(string(r.body), `"error":null`) {
		t.Errorf("no nulls in %s", r.body)
	}
}

// pages follows a list's next links from target, and returns the name of
// each page's items.
func pages(t *testing.T, src Source, target string) [][]string {
	t.Helper()
	var out [][]string
	for target != "" && len(out) < 10 {
		r := get(t, src, http.MethodGet, target)
		if r.code != http.StatusOK {
			t.Fatalf("%s: %d %s", target, r.code, r.body)
		}
		page := decode[gen.ServerGroupPage](t, r)
		var names []string
		for _, g := range page.Items {
			names = append(names, g.Name)
		}
		out = append(out, names)
		target = ""
		if page.Next != nil {
			u, err := url.Parse(*page.Next)
			if err != nil || u.Scheme != "http" || u.Host != "example.com" {
				t.Fatalf("next %q isn't absolute", *page.Next)
			}
			target = u.RequestURI()
		}
	}
	return out
}

func TestServerGroupPages(t *testing.T) {
	tests := []struct {
		target string
		want   string
	}{
		{"/api/server-groups?limit=1", "[[site-a] [site-b] [site-c]]"},
		{"/api/server-groups?limit=2", "[[site-a site-b] [site-c]]"},
		{"/api/server-groups?limit=3", "[[site-a site-b site-c]]"},
		{"/api/server-groups?limit=1000", "[[site-a site-b site-c]]"},
	}
	for _, tt := range tests {
		if got := fmtPages(pages(t, threeGroups(), tt.target)); got != tt.want {
			t.Errorf("%s: pages %s, want %s", tt.target, got, tt.want)
		}
	}
	// No groups is one empty page.
	r := get(t, fakeSource{groups: []service.GroupView{}}, http.MethodGet, "/api/server-groups")
	if page := decode[gen.ServerGroupPage](t, r); len(page.Items) != 0 || page.Next != nil || !strings.Contains(string(r.body), `"items":[]`) {
		t.Errorf("empty: %s", r.body)
	}
}

func fmtPages(p [][]string) string {
	var b strings.Builder
	b.WriteString("[")
	for i, names := range p {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString("[" + strings.Join(names, " ") + "]")
	}
	b.WriteString("]")
	return b.String()
}

func TestServerGroupsBadRequests(t *testing.T) {
	gone := cursor{After: "site-z"}.String()
	filtered := cursor{After: "site-a", Filter: "state=drift"}.String()
	tests := []struct {
		query, detail string
	}{
		{"limit=0", "The limit must be from 1 to 1000."},
		{"limit=1001", "The limit must be from 1 to 1000."},
		{"limit=many", "limit"},
		{"cursor=not-base64!", "The cursor isn't one that this API gave."},
		{"cursor=" + cursor{}.String(), "The cursor isn't one that this API gave."},
		{"cursor=" + gone, "isn't configured any more"},
		{"cursor=" + filtered, "other filters"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			r := get(t, threeGroups(), http.MethodGet, "/api/server-groups?"+tt.query)
			if p := problemOf(t, r, http.StatusBadRequest); !strings.Contains(*p.Detail, tt.detail) {
				t.Errorf("detail %q, want %q", *p.Detail, tt.detail)
			}
		})
	}
}

func TestGetServerGroup(t *testing.T) {
	r := get(t, threeGroups(), http.MethodGet, "/api/server-groups/site-b")
	if g := decode[gen.ServerGroup](t, r); r.code != http.StatusOK || g.Name != "site-b" || g.DriftPolicy != gen.DriftPolicyEnforce {
		t.Errorf("%d: %s", r.code, r.body)
	}
	r = get(t, threeGroups(), http.MethodGet, "/api/server-groups/site-z")
	if p := problemOf(t, r, http.StatusNotFound); *p.Detail != "No server group is named site-z." || *p.Instance != "/api/server-groups/site-z" {
		t.Errorf("problem %+v", p)
	}
}

func TestPublicURLLinks(t *testing.T) {
	h := New(Options{Source: threeGroups(), Log: slog.New(slog.DiscardHandler), PublicURL: "https://dns.example.com/nbpdns"})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/server-groups?limit=2", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()
	checker.Check(t, req, resp)
	page := decode[gen.ServerGroupPage](t, reply{rec.Code, rec.Header(), rec.Body.Bytes()})
	if page.Self != "https://dns.example.com/nbpdns/api/server-groups?limit=2" || page.Next == nil ||
		!strings.HasPrefix(*page.Next, "https://dns.example.com/nbpdns/api/server-groups?cursor=") {
		t.Errorf("self %q, next %v", page.Self, page.Next)
	}
}
