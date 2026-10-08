//go:build integration

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api/contract"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/webhook"
)

const webhookSecret = "nbpdns-lab-webhook-secret-0123456789" // gitleaks:allow

// webhookServe runs `nbpdns serve` on a drift fixture, with webhooks on and
// an interval of an hour, so that only webhooks refresh, and env's other
// settings. It returns the fixture, and serve once it's ready.
func webhookServe(t *testing.T, env map[string]string) (*lab.DriftFixture, *served) {
	t.Helper()
	nb, p := lab.NetBoxes[0], lab.PowerDNSes[0]
	f := lab.NewDriftFixture(t, nb, p)
	cfg := configFile(t, fmt.Sprintf("powerdns:\n  groups:\n    - name: %s\n      views: [%s]\n      zone_policies: {%s: ignore}\n      primary: {url: %q, api_key: %q}\n",
		p.Group, f.View, strings.TrimSuffix(f.Ignored, "."), p.URL(), lab.PowerDNSAPIKey))
	for k, v := range env {
		cfg[k] = v
	}
	cfg["NBPDNS_NETBOX_URL"], cfg["NBPDNS_NETBOX_TOKEN"] = nb.URL(), f.ReaderToken
	cfg["NBPDNS_NETBOX_WEBHOOK_SECRET"] = webhookSecret
	cfg["NBPDNS_DRIFT_INTERVAL"] = "1h"
	cfg["NBPDNS_DRIFT_WEBHOOK_DELAY"] = "200ms"
	s := startServe(t, cfg)
	s.waitReady(2 * time.Minute)
	return f, s
}

// zoneID returns the NetBox ID of the zone named name, in view.
func zoneID(t *testing.T, nb lab.NetBox, name, view string) int {
	t.Helper()
	var page struct {
		Results []struct {
			ID int `json:"id"`
		} `json:"results"`
	}
	lab.AdminDo(t, nb, http.MethodGet, fmt.Sprintf("plugins/netbox-dns/zones/?name=%s&view=%s", strings.TrimSuffix(name, "."), view), nil, &page)
	if len(page.Results) != 1 {
		t.Fatalf("NetBox has %d zones %s in %s", len(page.Results), name, view)
	}
	return page.Results[0].ID
}

// changeRecord changes the value of the active record of zone with name,
// type and value, as NetBox's admin.
func changeRecord(t *testing.T, nb lab.NetBox, zone int, name, typ, from, to string) {
	t.Helper()
	var page struct {
		Results []struct {
			ID int `json:"id"`
		} `json:"results"`
	}
	lab.AdminDo(t, nb, http.MethodGet, fmt.Sprintf("plugins/netbox-dns/records/?zone_id=%d&name=%s&type=%s&value=%s&status=active", zone, name, typ, from), nil, &page)
	if len(page.Results) != 1 {
		t.Fatalf("zone %d has %d records %s %s %s", zone, len(page.Results), name, typ, from)
	}
	lab.AdminDo(t, nb, http.MethodPatch, fmt.Sprintf("plugins/netbox-dns/records/%d/", page.Results[0].ID), map[string]any{"value": to}, nil)
}

// replayed returns a NetBox 4.7 event, captured from the lab, from
// internal/webhook's test data, with edit applied to it, as JSON.
func replayed(t *testing.T, file string, edit func(e map[string]any)) []byte {
	t.Helper()
	b, err := os.ReadFile("../webhook/testdata/netbox-47/" + file)
	if err != nil {
		t.Fatal(err)
	}
	var e map[string]any
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatal(err)
	}
	edit(e)
	out, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// obj returns the object at the path of keys in e.
func obj(t *testing.T, e map[string]any, keys ...string) map[string]any {
	t.Helper()
	for _, k := range keys {
		next, ok := e[k].(map[string]any)
		if !ok {
			t.Fatalf("the event has no object %s", strings.Join(keys, "."))
		}
		e = next
	}
	return e
}

// post sends body to path, signed with secret, and checks the response
// against the API's OpenAPI document.
func (s *served) post(path string, body []byte, secret string) (int, string) {
	s.t.Helper()
	checker, err := contract.New(api.Spec(), "/api")
	if err != nil {
		s.t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(s.t.Context(), http.MethodPost, "http://"+s.addr+path, bytes.NewReader(body))
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(webhook.SignatureHeader, webhook.Sign(secret, body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	checker.Check(s.t, req, resp)
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatal(err)
	}
	return resp.StatusCode, string(b)
}

// eventually checks ok every 100 ms, until it's true, or fails t after
// within.
func eventually(t *testing.T, within time.Duration, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(within); !ok(); time.Sleep(100 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%s didn't happen within %s", what, within)
		}
	}
}

// zoneState returns the zone's state and change count through the API.
func zoneState(t *testing.T, s *served, group, zone string) (string, int) {
	t.Helper()
	code, body := s.api("/api/server-groups/" + group + "/zones/" + zone)
	var z struct {
		State       string `json:"state"`
		ChangeCount int    `json:"change_count"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(body), &z) != nil {
		t.Fatalf("zone %s: %d %s", zone, code, body)
	}
	return z.State, z.ChangeCount
}

// metric returns the value of the series named series in serve's metrics,
// or -1 if there's none.
func metric(t *testing.T, s *served, series string) float64 {
	t.Helper()
	_, body := s.get("/metrics")
	m := regexp.MustCompile("(?m)^" + regexp.QuoteMeta(series) + " ([0-9.e+]+)$").FindStringSubmatch(body)
	if m == nil {
		return -1
	}
	var v float64
	if _, err := fmt.Sscan(m[1], &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// TestServeWebhooks replays NetBox 4.7's webhooks, signed, against serve
// on the lab (ADR-0035): a record's change refreshes its zone, and only its
// zone, within seconds; a forged one changes nothing; and a view's change
// makes a full refresh.
func TestServeWebhooks(t *testing.T) {
	nb, group := lab.NetBoxes[0], lab.PowerDNSes[0].Group
	f, s := webhookServe(t, nil)
	if state, _ := zoneState(t, s, group, f.InSync); state != "in_sync" {
		t.Fatalf("%s is %s before the change", f.InSync, state)
	}
	refreshes := func(kind, outcome string) float64 {
		return metric(t, s, fmt.Sprintf(`nbpdns_drift_%srefreshes_total{outcome=%q}`, kind, outcome))
	}
	if n := refreshes("", "complete"); n != 1 {
		t.Fatalf("%v full refreshes before any webhook", n)
	}

	// The record changes in NetBox, and NetBox's event for it comes.
	id := zoneID(t, nb, f.InSync, f.View)
	changeRecord(t, nb, id, "www", "A", "192.0.2.10", "192.0.2.11")
	const requestID = "7d1b8a52-1f0e-4f43-9b7e-6a2f43c7c111"
	event := replayed(t, "record-updated.json", func(e map[string]any) {
		z := obj(t, e, "data", "zone")
		z["id"], z["name"] = id, strings.TrimSuffix(f.InSync, ".")
		obj(t, e, "data", "zone", "view")["name"] = f.View
		obj(t, e, "snapshots", "prechange")["zone"] = id
		obj(t, e, "request")["id"] = requestID
	})
	if code, body := s.post("/api/netbox-events", event, webhookSecret); code != http.StatusAccepted {
		t.Fatalf("the event: %d %s", code, body)
	}
	eventually(t, 15*time.Second, "the zone refresh", func() bool { return refreshes("zone_", "complete") == 1 })
	if state, changes := zoneState(t, s, group, f.InSync); state != "drift" || changes != 1 {
		t.Errorf("%s is %s with %d changes after the webhook, want drift with 1", f.InSync, state, changes)
	}
	// The other zones are as the full refresh left them.
	if state, changes := zoneState(t, s, group, f.Drift); state != "drift" || changes != 5 {
		t.Errorf("%s is %s with %d changes", f.Drift, state, changes)
	}
	code, body := s.api("/api/server-groups/" + group)
	var g struct {
		Counts struct {
			InSync int `json:"in_sync"`
			Drift  int `json:"drift"`
		} `json:"counts"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(body), &g) != nil || g.Counts.InSync != 0 || g.Counts.Drift != 2 {
		t.Errorf("the group: %d %s", code, body)
	}
	if n := refreshes("", "complete"); n != 1 {
		t.Errorf("%v full refreshes, want only the first", n)
	}
	var st service.Status
	if _, page := s.get("/status?json=1"); json.Unmarshal([]byte(page), &st) != nil {
		t.Fatalf("status: %s", page)
	}
	w := st.Webhooks
	if !w.Enabled || w.LastEvent == nil || w.LastEvent.Request.ID != requestID || w.LastRefresh == nil ||
		w.LastRefresh.Outcome != "complete" || len(w.LastRefresh.Zones) != 1 || w.LastRefresh.Zones[0] != f.View+"/"+f.InSync {
		t.Errorf("webhooks %+v, last event %+v, last refresh %+v", w, w.LastEvent, w.LastRefresh)
	}
	if !strings.Contains(s.stderr.String(), `"netbox_request_ids":["`+requestID+`"]`) {
		t.Errorf("no log line with NetBox's request ID:\n%s", s.stderr)
	}

	// A forged event changes nothing.
	if code, body := s.post("/api/netbox-events", event, "a-secret-that-isnt-nbpdnss"); code != http.StatusUnauthorized {
		t.Errorf("a forged event: %d %s", code, body)
	}
	time.Sleep(time.Second)
	if n := refreshes("zone_", "complete"); n != 1 || metric(t, s, `nbpdns_netbox_webhooks_total{result="bad_signature"}`) != 1 {
		t.Errorf("%v zone refreshes after a forged event", n)
	}

	// A view's change makes a full refresh.
	view := replayed(t, "view-renamed.json", func(e map[string]any) {
		obj(t, e, "data")["name"] = f.View
	})
	if code, body := s.post("/api/netbox-events", view, webhookSecret); code != http.StatusAccepted {
		t.Fatalf("the view's event: %d %s", code, body)
	}
	eventually(t, 30*time.Second, "the full refresh", func() bool { return refreshes("", "complete") == 2 })
	if state, changes := zoneState(t, s, group, f.InSync); state != "drift" || changes != 1 {
		t.Errorf("%s is %s with %d changes after the full refresh", f.InSync, state, changes)
	}
}
