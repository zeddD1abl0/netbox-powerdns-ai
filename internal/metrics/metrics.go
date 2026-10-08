// Package metrics declares nbpdns's Prometheus metrics (ADR-0029). Each one
// is declared once, in defs, which both registers it and writes its
// reference page. Only nbpdns's own metrics are here: PowerDNS's statistics
// stay on PowerDNS's own /metrics.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
)

// The outcomes of a drift refresh.
const (
	// OutcomeComplete is a refresh that read NetBox and every group.
	OutcomeComplete = "complete"
	// OutcomeIncomplete is a refresh that read NetBox, but not every group.
	OutcomeIncomplete = "incomplete"
	// OutcomeFailed is a refresh that couldn't read NetBox.
	OutcomeFailed = "failed"
)

// Outcomes are the values of the outcome label.
var Outcomes = []string{OutcomeComplete, OutcomeIncomplete, OutcomeFailed}

// The results of a NetBox webhook (ADR-0035).
const (
	// WebhookAccepted is an event that queued a refresh.
	WebhookAccepted = "accepted"
	// WebhookIgnored is an event that named nothing that a server group
	// serves.
	WebhookIgnored = "ignored"
	// WebhookBadSignature is a request whose signature didn't verify.
	WebhookBadSignature = "bad_signature"
	// WebhookInvalid is a signed request that wasn't an event NetBox sends,
	// or was too large.
	WebhookInvalid = "invalid"
)

// WebhookResults are the values of the result label.
var WebhookResults = []string{WebhookAccepted, WebhookIgnored, WebhookBadSignature, WebhookInvalid}

// The kinds of metric.
const (
	counter   = "counter"
	gauge     = "gauge"
	histogram = "histogram"
)

// A def declares one metric.
type def struct {
	name   string
	kind   string
	help   string
	labels []string
	// values lists a label's values, where there's a fixed set.
	values  map[string][]string
	buckets []float64
}

// The zone states and RRset changes, as the drift report names them.
var (
	zoneStates    = drift.States
	driftedStates = drift.DriftedStates
	changeKinds   = drift.ChangeKinds
	// The services nbpdns sends requests to, as internal/httpclient names
	// them, and the methods it sends.
	services = []string{"NetBox", "PowerDNS"}
	methods  = []string{"GET"}
)

// The declarations, in the order the reference lists them.
var (
	defRefreshes = def{name: "nbpdns_drift_refreshes_total", kind: counter, labels: []string{"outcome"},
		values: map[string][]string{"outcome": Outcomes},
		help:   "Drift refreshes finished, by outcome. A refresh is incomplete if a server group couldn't be read, and failed if NetBox couldn't be read."}
	defRefreshDuration = def{name: "nbpdns_drift_refresh_duration_seconds", kind: histogram,
		buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600, 1200},
		help:    "How long each drift refresh took."}
	defZoneRefreshes = def{name: "nbpdns_drift_zone_refreshes_total", kind: counter, labels: []string{"outcome"},
		values: map[string][]string{"outcome": Outcomes},
		help: "Zone refreshes finished, by outcome: refreshes of only the zones that NetBox's webhooks named. " +
			"Incomplete if a server group couldn't be read, and failed if NetBox couldn't be read. " +
			"A webhook that asks for a full refresh is counted in `nbpdns_drift_refreshes_total`."}
	defZoneRefreshDuration = def{name: "nbpdns_drift_zone_refresh_duration_seconds", kind: histogram,
		buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
		help:    "How long each zone refresh took."}
	defPendingZones = def{name: "nbpdns_drift_pending_zones", kind: gauge,
		help: "The zones that NetBox's webhooks named, waiting for their refresh."}
	defLastRefresh = def{name: "nbpdns_drift_last_refresh_timestamp_seconds", kind: gauge,
		help: "When the last drift refresh finished, whatever its outcome, as a Unix time. It has no value before the first."}
	defLastComplete = def{name: "nbpdns_drift_last_complete_refresh_timestamp_seconds", kind: gauge,
		help: "When the last complete drift refresh finished, as a Unix time. It has no value before the first."}
	defZones = def{name: "nbpdns_drift_zones", kind: gauge, labels: []string{"group", "state"},
		values: map[string][]string{"state": zoneStates},
		help:   "The server group's zones, by state, as of its primary's last successful read."}
	defRRsetChanges = def{name: "nbpdns_drift_rrset_changes", kind: gauge, labels: []string{"group", "kind"},
		values: map[string][]string{"kind": changeKinds},
		help:   "The RRsets that differ in the server group's zones, by kind."}
	defZoneDrifted = def{name: "nbpdns_drift_zone_drifted", kind: gauge, labels: []string{"group", "zone", "state"},
		values: map[string][]string{"state": driftedStates},
		help:   "1 for each zone that drifted, by its state. A zone back in sync has no series."}
	defProblems = def{name: "nbpdns_drift_problems", kind: gauge, labels: []string{"group"},
		help: "Problems in the data that normalization worked around, in the server group's zones."}
	defWarnings = def{name: "nbpdns_drift_warnings", kind: gauge, labels: []string{"group"},
		help: "Warnings about the configuration or NetBox's zones, for the server group."}
	defNetBoxUp = def{name: "nbpdns_netbox_up", kind: gauge,
		help: "1 if the last drift refresh could read NetBox, else 0. It has no value before the first refresh, nor after one stopped by drift.timeout, which keeps the value before it."}
	defGroupUp = def{name: "nbpdns_server_group_up", kind: gauge, labels: []string{"group"},
		help: "1 if the server group's primary could be read the last time a refresh tried, else 0. A refresh that can't read NetBox doesn't try the primaries."}
	defGroupLastSuccess = def{name: "nbpdns_server_group_last_success_timestamp_seconds", kind: gauge, labels: []string{"group"},
		help: "When the server group's primary was last read and compared, as a Unix time."}
	defRequests = def{name: "nbpdns_http_client_requests_total", kind: counter, labels: []string{"service", "target", "method", "code"},
		values: map[string][]string{"service": services, "method": methods},
		help: "Requests to NetBox and to each primary, one per attempt. The target is `netbox`, or the server group's name, " +
			"and the code is the answer's status, or `error` if none came."}
	defRequestDuration = def{name: "nbpdns_http_client_request_duration_seconds", kind: histogram, labels: []string{"service", "target", "method"},
		values:  map[string][]string{"service": services, "method": methods},
		buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		help:    "How long each request took to answer, or to fail."}
	defRetries = def{name: "nbpdns_http_client_retries_total", kind: counter, labels: []string{"service", "target"},
		values: map[string][]string{"service": services},
		help:   "Requests to NetBox and to each primary that were tried again."}
	defAPIRequests = def{name: "nbpdns_api_requests_total", kind: counter, labels: []string{"operation", "code"},
		help: "Requests to the API. The operation is the request's `operationId` in `api/openapi.yaml`, such as `getStatus`, " +
			"or `openapi` for the OpenAPI document, `docs` for the API reference, or `unmatched` for any other path or method. " +
			"The code is the answer's status."}
	defAPIRequestDuration = def{name: "nbpdns_api_request_duration_seconds", kind: histogram, labels: []string{"operation"},
		buckets: []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
		help:    "How long the API took to answer each request."}
	defWebhooks = def{name: "nbpdns_netbox_webhooks_total", kind: counter, labels: []string{"result"},
		values: map[string][]string{"result": WebhookResults},
		help: "Requests to `/api/netbox-events`, NetBox's webhooks, by result: `accepted` if the event queued a refresh, " +
			"`ignored` if it named nothing that a server group serves, `bad_signature` if its signature didn't verify, " +
			"and `invalid` if it wasn't an event that NetBox sends, or was over 1 MiB. Requests while webhooks are off aren't counted."}
	defBuildInfo = def{name: "nbpdns_build_info", kind: gauge, labels: []string{"version", "revision", "goversion"},
		help: "1, with the running build's version, VCS revision, and Go version."}

	defs = []def{
		defRefreshes, defRefreshDuration, defZoneRefreshes, defZoneRefreshDuration, defPendingZones,
		defLastRefresh, defLastComplete,
		defZones, defRRsetChanges, defZoneDrifted, defProblems, defWarnings,
		defNetBoxUp, defGroupUp, defGroupLastSuccess,
		defRequests, defRequestDuration, defRetries, defAPIRequests, defAPIRequestDuration,
		defWebhooks, defBuildInfo,
	}
)

// Metrics holds nbpdns's metrics, registered with Registry.
type Metrics struct {
	Registry *prometheus.Registry

	Refreshes           *prometheus.CounterVec
	RefreshDuration     prometheus.Histogram
	ZoneRefreshes       *prometheus.CounterVec
	ZoneRefreshDuration prometheus.Histogram
	PendingZones        prometheus.Gauge
	// LastRefresh, LastCompleteRefresh and NetBoxUp have no labels. They're
	// vectors so that they have no series until they're first set, which
	// would otherwise read as a refresh at 1970 and NetBox down.
	LastRefresh         *prometheus.GaugeVec
	LastCompleteRefresh *prometheus.GaugeVec
	Zones               *prometheus.GaugeVec
	RRsetChanges        *prometheus.GaugeVec
	ZoneDrifted         *prometheus.GaugeVec
	Problems            *prometheus.GaugeVec
	Warnings            *prometheus.GaugeVec
	NetBoxUp            *prometheus.GaugeVec
	GroupUp             *prometheus.GaugeVec
	GroupLastSuccess    *prometheus.GaugeVec

	requests           *prometheus.CounterVec
	requestDuration    *prometheus.HistogramVec
	retries            *prometheus.CounterVec
	apiRequests        *prometheus.CounterVec
	apiRequestDuration *prometheus.HistogramVec
	webhooks           *prometheus.CounterVec
}

// New returns nbpdns's metrics for the build info, registered in a new
// registry with the Go runtime and process collectors.
func New(info version.Info) *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	m := &Metrics{
		Registry:            reg,
		Refreshes:           register(reg, prometheus.NewCounterVec(counterOpts(defRefreshes), defRefreshes.labels)),
		RefreshDuration:     register(reg, prometheus.NewHistogram(histogramOpts(defRefreshDuration))),
		ZoneRefreshes:       register(reg, prometheus.NewCounterVec(counterOpts(defZoneRefreshes), defZoneRefreshes.labels)),
		ZoneRefreshDuration: register(reg, prometheus.NewHistogram(histogramOpts(defZoneRefreshDuration))),
		PendingZones:        register(reg, prometheus.NewGauge(gaugeOpts(defPendingZones))),
		LastRefresh:         register(reg, prometheus.NewGaugeVec(gaugeOpts(defLastRefresh), nil)),
		LastCompleteRefresh: register(reg, prometheus.NewGaugeVec(gaugeOpts(defLastComplete), nil)),
		Zones:               register(reg, prometheus.NewGaugeVec(gaugeOpts(defZones), defZones.labels)),
		RRsetChanges:        register(reg, prometheus.NewGaugeVec(gaugeOpts(defRRsetChanges), defRRsetChanges.labels)),
		ZoneDrifted:         register(reg, prometheus.NewGaugeVec(gaugeOpts(defZoneDrifted), defZoneDrifted.labels)),
		Problems:            register(reg, prometheus.NewGaugeVec(gaugeOpts(defProblems), defProblems.labels)),
		Warnings:            register(reg, prometheus.NewGaugeVec(gaugeOpts(defWarnings), defWarnings.labels)),
		NetBoxUp:            register(reg, prometheus.NewGaugeVec(gaugeOpts(defNetBoxUp), nil)),
		GroupUp:             register(reg, prometheus.NewGaugeVec(gaugeOpts(defGroupUp), defGroupUp.labels)),
		GroupLastSuccess:    register(reg, prometheus.NewGaugeVec(gaugeOpts(defGroupLastSuccess), defGroupLastSuccess.labels)),
		requests:            register(reg, prometheus.NewCounterVec(counterOpts(defRequests), defRequests.labels)),
		requestDuration:     register(reg, prometheus.NewHistogramVec(histogramOpts(defRequestDuration), defRequestDuration.labels)),
		retries:             register(reg, prometheus.NewCounterVec(counterOpts(defRetries), defRetries.labels)),
		apiRequests:         register(reg, prometheus.NewCounterVec(counterOpts(defAPIRequests), defAPIRequests.labels)),
		apiRequestDuration:  register(reg, prometheus.NewHistogramVec(histogramOpts(defAPIRequestDuration), defAPIRequestDuration.labels)),
		webhooks:            register(reg, prometheus.NewCounterVec(counterOpts(defWebhooks), defWebhooks.labels)),
	}
	build := register(reg, prometheus.NewGaugeVec(gaugeOpts(defBuildInfo), defBuildInfo.labels))
	build.WithLabelValues(info.Version, info.Commit, info.GoVersion).Set(1)
	// Every outcome has a series from the start, so a rate is defined before
	// the first failure.
	for _, o := range Outcomes {
		m.Refreshes.WithLabelValues(o)
		m.ZoneRefreshes.WithLabelValues(o)
	}
	for _, r := range WebhookResults {
		m.webhooks.WithLabelValues(r)
	}
	return m
}

// Handler serves the metrics in Prometheus's text or OpenMetrics format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{EnableOpenMetrics: true})
}

// APIRequest counts one request to the API, for operation, answered with
// code after d.
func (m *Metrics) APIRequest(operation string, code int, d time.Duration) {
	m.apiRequests.WithLabelValues(operation, strconv.Itoa(code)).Inc()
	m.apiRequestDuration.WithLabelValues(operation).Observe(d.Seconds())
}

// NetBoxWebhook counts a request to /api/netbox-events, with one of
// WebhookResults.
func (m *Metrics) NetBoxWebhook(result string) { m.webhooks.WithLabelValues(result).Inc() }

// Observer returns an observer of the requests to target, a server of
// service: NetBox, or a server group's primary.
func (m *Metrics) Observer(service, target string) *RequestObserver {
	return &RequestObserver{m: m, service: service, target: target}
}

// A RequestObserver counts an HTTP client's requests. It implements
// internal/httpclient's Observer.
type RequestObserver struct {
	m               *Metrics
	service, target string
}

// Request counts one attempt. A code of 0 means no answer came.
func (o *RequestObserver) Request(method string, code int, d time.Duration) {
	c := "error"
	if code != 0 {
		c = strconv.Itoa(code)
	}
	o.m.requests.WithLabelValues(o.service, o.target, method, c).Inc()
	o.m.requestDuration.WithLabelValues(o.service, o.target, method).Observe(d.Seconds())
}

// Retry counts a request that's tried again.
func (o *RequestObserver) Retry() { o.m.retries.WithLabelValues(o.service, o.target).Inc() }

func register[C prometheus.Collector](reg *prometheus.Registry, c C) C {
	reg.MustRegister(c)
	return c
}

func counterOpts(d def) prometheus.CounterOpts {
	return prometheus.CounterOpts{Name: d.name, Help: d.help}
}
func gaugeOpts(d def) prometheus.GaugeOpts { return prometheus.GaugeOpts{Name: d.name, Help: d.help} }

func histogramOpts(d def) prometheus.HistogramOpts {
	return prometheus.HistogramOpts{Name: d.name, Help: d.help, Buckets: d.buckets}
}
