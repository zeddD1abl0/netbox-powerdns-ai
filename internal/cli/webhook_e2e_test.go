//go:build integration && webhooks

package cli

import (
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
)

// TestWebhooksFromNetBox has the lab's NetBox send its own webhooks to
// serve (ADR-0035), through the worker that the profile webhooks starts:
// make test-webhooks. NetBox reaches serve on the Docker host, so serve
// listens on every interface.
func TestWebhooksFromNetBox(t *testing.T) {
	nb, group := lab.NetBoxes[0], lab.PowerDNSes[0].Group
	f, s := webhookServe(t, map[string]string{"NBPDNS_SERVER_LISTEN": "0.0.0.0:0"})
	_, port, err := net.SplitHostPort(s.addr)
	if err != nil {
		t.Fatal(err)
	}
	hook := lab.AdminCreate(t, nb, "extras/webhooks/", map[string]any{
		"name": "nbpdns-" + f.ID, "payload_url": "http://host.docker.internal:" + port + "/api/netbox-events",
		"http_method": "POST", "http_content_type": "application/json", "secret": webhookSecret,
	})
	lab.AdminCreate(t, nb, "extras/event-rules/", map[string]any{
		"name":         "nbpdns-" + f.ID,
		"object_types": []string{"netbox_dns.view", "netbox_dns.zone", "netbox_dns.record"},
		"event_types":  []string{"object_created", "object_updated", "object_deleted"},
		"action_type":  "webhook", "action_object_type": "extras.webhook", "action_object_id": hook,
	})

	changeRecord(t, nb, zoneID(t, nb, f.InSync, f.View), "www", "A", "192.0.2.10", "192.0.2.11")
	eventually(t, time.Minute, "the zone's drift, from NetBox's webhook (is the worker up? make test-webhooks)", func() bool {
		state, _ := zoneState(t, s, group, f.InSync)
		return state == "drift"
	})
	var st service.Status
	if _, page := s.get("/status?json=1"); json.Unmarshal([]byte(page), &st) != nil {
		t.Fatalf("status: %s", page)
	}
	if e := st.Webhooks.LastEvent; e == nil || e.Request.User != "admin" || e.Request.ID == "" {
		t.Errorf("the last event %+v, want the admin's request", e)
	}
	if code, _ := s.get("/readyz"); code != http.StatusOK {
		t.Errorf("readyz: %d", code)
	}
}
