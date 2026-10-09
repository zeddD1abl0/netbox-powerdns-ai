package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/client_golang/prometheus/testutil/promlint"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
)

var info = version.Info{Version: "v1.2.3", Commit: "0123abc", GoVersion: "go1.27.1"}

// filled returns metrics with a series of every metric, so that each one is
// gathered.
func filled(t *testing.T) *Metrics {
	t.Helper()
	m := New(info)
	m.RefreshDuration.Observe(12)
	m.ZoneRefreshDuration.Observe(0.3)
	m.LastRefresh.WithLabelValues().SetToCurrentTime()
	m.LastCompleteRefresh.WithLabelValues().SetToCurrentTime()
	m.Zones.WithLabelValues("site-a", "in_sync").Set(3)
	m.RRsetChanges.WithLabelValues("site-a", "changed").Set(1)
	m.ZoneDrifted.WithLabelValues("site-a", "example.com.", "drift").Set(1)
	m.Problems.WithLabelValues("site-a").Set(0)
	m.Warnings.WithLabelValues("site-a").Set(0)
	m.NetBoxUp.WithLabelValues().Set(1)
	m.GroupUp.WithLabelValues("site-a").Set(1)
	m.GroupLastSuccess.WithLabelValues("site-a").SetToCurrentTime()
	o := m.Observer("NetBox", "netbox")
	o.Request(http.MethodGet, 200, 30*time.Millisecond)
	o.Retry()
	m.APIRequest("getStatus", 200, time.Millisecond)
	return m
}

func TestLint(t *testing.T) {
	m := filled(t)
	mfs, err := m.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	gathered := map[string]bool{}
	for _, mf := range mfs {
		gathered[mf.GetName()] = true
	}
	for _, d := range defs {
		if !gathered[d.name] {
			t.Errorf("%s isn't registered", d.name)
		}
	}
	problems, err := promlint.NewWithMetricFamilies(mfs).Lint()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Errorf("%s: %s", p.Metric, p.Text)
	}
}

func TestObserver(t *testing.T) {
	m := New(info)
	o := m.Observer("PowerDNS", "site-a")
	o.Request(http.MethodGet, 200, 10*time.Millisecond)
	o.Request(http.MethodGet, 200, 20*time.Millisecond)
	o.Request(http.MethodGet, 0, time.Second)
	o.Retry()
	for _, tt := range []struct {
		code string
		want float64
	}{{"200", 2}, {"error", 1}} {
		if got := testutil.ToFloat64(m.requests.WithLabelValues("PowerDNS", "site-a", "GET", tt.code)); got != tt.want {
			t.Errorf("requests with code %s: %v, want %v", tt.code, got, tt.want)
		}
	}
	if got := testutil.ToFloat64(m.retries.WithLabelValues("PowerDNS", "site-a")); got != 1 {
		t.Errorf("retries %v", got)
	}
	if n := testutil.CollectAndCount(m.requestDuration); n != 1 {
		t.Errorf("%d duration series, want 1", n)
	}
}

func TestNetBoxWebhook(t *testing.T) {
	m := New(info)
	m.NetBoxWebhook(WebhookAccepted)
	m.NetBoxWebhook(WebhookAccepted)
	m.NetBoxWebhook(WebhookBadSignature)
	for _, tt := range []struct {
		result string
		want   float64
	}{{WebhookAccepted, 2}, {WebhookIgnored, 0}, {WebhookBadSignature, 1}, {WebhookInvalid, 0}} {
		if got := testutil.ToFloat64(m.webhooks.WithLabelValues(tt.result)); got != tt.want {
			t.Errorf("webhooks with result %s: %v, want %v", tt.result, got, tt.want)
		}
	}
}

func TestHandler(t *testing.T) {
	m := New(info)
	srv := httptest.NewServer(m.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL) //nolint:noctx // A test against a local server.
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`nbpdns_build_info{goversion="go1.27.1",revision="0123abc",version="v1.2.3"} 1`,
		// Every outcome is there before any refresh.
		`nbpdns_drift_refreshes_total{outcome="complete"} 0`,
		`nbpdns_drift_refreshes_total{outcome="incomplete"} 0`,
		`nbpdns_drift_refreshes_total{outcome="failed"} 0`,
		// And every webhook result, before any webhook.
		`nbpdns_netbox_webhooks_total{result="accepted"} 0`,
		`nbpdns_netbox_webhooks_total{result="bad_signature"} 0`,
		"go_goroutines ",
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("no %q in:\n%s", want, b)
		}
	}
	// Until a refresh sets them, these have no value, rather than one that
	// reads as NetBox down, or a refresh in 1970.
	for _, absent := range []string{"nbpdns_netbox_up ", "nbpdns_drift_last_refresh_timestamp_seconds ", "nbpdns_drift_last_complete_refresh_timestamp_seconds "} {
		if strings.Contains(string(b), absent) {
			t.Errorf("%q has a value before any refresh:\n%s", absent, b)
		}
	}
}

func TestReference(t *testing.T) {
	var b strings.Builder
	if err := WriteReference(&b); err != nil {
		t.Fatal(err)
	}
	for _, d := range defs {
		if !strings.Contains(b.String(), "| `"+d.name+"` | "+d.kind+" |") {
			t.Errorf("the reference has no row for %s", d.name)
		}
	}
	if !strings.Contains(b.String(), "Buckets, in seconds: 1, 5, 10, 30, 60, 120, 300, 600, 1200.") {
		t.Errorf("the refresh duration's buckets aren't listed:\n%s", b.String())
	}
}
