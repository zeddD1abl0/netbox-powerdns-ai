package config

import (
	"cmp"
	"slices"
	"time"
)

// Config is nbpdns's configuration, with the defaults, the config file, the
// environment and the flags applied.
type Config struct {
	Log      LogConfig
	NetBox   NetBoxConfig
	PowerDNS PowerDNSConfig
	Drift    DriftConfig
	OTLP     OTLPConfig
	Server   ServerConfig
}

// DriftConfig says how drift is compared, and how often `nbpdns serve`
// compares it.
type DriftConfig struct {
	// GroupConcurrency is how many server groups are compared at once.
	GroupConcurrency int
	// Interval is the time from one refresh's start to the next's.
	Interval time.Duration
	// Timeout bounds a refresh.
	Timeout time.Duration
}

// ServerConfig configures the listener of `nbpdns serve`.
type ServerConfig struct {
	// Listen is the TCP address to listen on.
	Listen string
}

// LogConfig configures the operational log, written to standard error.
type LogConfig struct {
	Level  string // debug, info, warn or error
	Format string // json or text
}

// NetBoxConfig says how to reach NetBox and read from it.
type NetBoxConfig struct {
	URL         string
	Token       Secret
	CAFile      string
	Timeout     time.Duration
	PageSize    int
	Concurrency int
}

// PowerDNSConfig says how to read the PowerDNS server groups.
type PowerDNSConfig struct {
	Timeout     time.Duration
	Concurrency int
	// Groups come from the config file's powerdns.groups (ADR-0026).
	Groups []Group
}

// The OTLP protocols.
const (
	OTLPHTTP = "http/protobuf"
	OTLPGRPC = "grpc"
)

// OTLPConfig configures the export of spans to an OpenTelemetry collector,
// over OTLP (ADR-0029).
type OTLPConfig struct {
	// Endpoint is the collector's URL. If it's empty, nothing is exported.
	Endpoint string
	// Protocol is OTLPHTTP or OTLPGRPC.
	Protocol string
	// Headers are sent with every export, as name=value,name=value.
	Headers Secret
	CAFile  string
	Timeout time.Duration
}

// Group returns the group named name.
func (c PowerDNSConfig) Group(name string) (Group, bool) {
	for _, g := range c.Groups {
		if g.Name == name {
			return g, true
		}
	}
	return Group{}, false
}

// keys declares every configuration key, bound to a field of c, sorted by
// name.
func keys(c *Config) []Key {
	ks := []Key{
		enumKey(&c.Log.Level, Key{
			Name:    "log.level",
			Summary: "The lowest level of log message to write.",
			Details: "At `debug`, nbpdns also logs each request it makes.",
			Default: "info",
		}, "debug", "info", "warn", "error"),
		enumKey(&c.Log.Format, Key{
			Name:    "log.format",
			Summary: "How log lines are written to standard error.",
			Details: "`json` writes one JSON object per line, for log collectors. " +
				"`text` writes `key=value` pairs, for reading in a terminal.",
			Default: "json",
		}, "json", "text"),
		stringKey(&c.NetBox.URL, "an `http` or `https` URL", "url", Key{
			Name:    "netbox.url",
			Summary: "NetBox's base URL, such as `https://netbox.example.com`.",
			Details: "Every command that reads from NetBox needs it.",
			Warning: "With an `http://` URL, the NetBox token crosses the network unencrypted, " +
				"and anyone on the path can read it. Use `https://` wherever NetBox offers it. " +
				"nbpdns logs a warning each time it connects to NetBox over `http://`.",
		}, checkURL),
		secretKey(&c.NetBox.Token, Key{
			Name:    "netbox.token",
			Summary: "The NetBox API token that nbpdns reads with.",
			Details: "Use a v2 token, which starts with `nbt_`, whose user can only view the DNS plugin's objects. " +
				"A v1 token still works, with a warning. " +
				"Pass the token in a file or the environment rather than as a flag, " +
				"which other users of the host can see in the process list.",
		}, nil),
		stringKey(&c.NetBox.CAFile, "path", "path", Key{
			Name:    "netbox.ca_file",
			Summary: "A PEM file of CA certificates to trust for NetBox, as well as the system's.",
			Details: "Use it when NetBox's certificate comes from a private CA.",
		}, nil),
		durationKey(&c.NetBox.Timeout, Key{
			Name:    "netbox.timeout",
			Summary: "How long one request to NetBox may take.",
			Details: "Write it with a unit, such as `30s` or `2m`.",
			Default: "30s",
		}),
		intKey(&c.NetBox.PageSize, Key{
			Name:    "netbox.page_size",
			Summary: "How many objects to ask NetBox for in each page of a list.",
			Details: "NetBox returns no more than its own `MAX_PAGE_SIZE`, 1000 by default.",
			Default: "500",
		}, 1000),
		intKey(&c.NetBox.Concurrency, Key{
			Name:    "netbox.concurrency",
			Summary: "How many requests to NetBox may be in flight at once.",
			Default: "4",
		}, 32),
		durationKey(&c.PowerDNS.Timeout, Key{
			Name:    "powerdns.timeout",
			Summary: "How long one request to a PowerDNS API may take.",
			Details: "Write it with a unit, such as `30s` or `2m`.",
			Default: "30s",
		}),
		intKey(&c.PowerDNS.Concurrency, Key{
			Name:    "powerdns.concurrency",
			Summary: "How many requests to each PowerDNS API may be in flight at once.",
			Default: "4",
		}, 32),
		intKey(&c.Drift.GroupConcurrency, Key{
			Name:    "drift.group_concurrency",
			Summary: "How many server groups are read and compared at once.",
			Details: "Each group's primary also takes up to `powerdns.concurrency` requests at once. " +
				"NetBox is read once, for all the groups together, whatever this is.",
			Default: "4",
		}, 32),
		durationKeyAtLeast(&c.Drift.Interval, Key{
			Name:    "drift.interval",
			Summary: "How often `nbpdns serve` refreshes the drift report, from the start of one refresh to the start of the next.",
			Details: "A refresh that takes longer delays the next one, so refreshes never overlap. " +
				"Each refresh reads every zone's records from NetBox and from each primary, so make it much longer than a refresh takes.",
			Default: "5m",
		}, 10*time.Second),
		durationKey(&c.Drift.Timeout, Key{
			Name:    "drift.timeout",
			Summary: "How long one refresh of `nbpdns serve` may take before it's stopped.",
			Details: "A refresh that's stopped counts as failed, and every server group keeps its last report: " +
				"a slow refresh says nothing about what PowerDNS serves. The next one starts on schedule.",
			Default: "10m",
		}),
		stringKey(&c.Server.Listen, "a TCP address", "address", Key{
			Name:    "server.listen",
			Summary: "The address `nbpdns serve` listens on, for `/livez`, `/readyz`, `/status` and `/metrics`.",
			Details: "Such as `:8080` for every interface, or `127.0.0.1:8080` for this host only.",
			Warning: "The listener has no authentication until M10, and its pages name your server groups, " +
				"zones, and URLs. Keep the port on a trusted network.",
			Default: ":8080",
		}, checkListen),
		stringKey(&c.OTLP.Endpoint, "an `http` or `https` URL", "url", Key{
			Name:    "otlp.endpoint",
			Summary: "The URL of the OpenTelemetry collector that nbpdns exports its spans to, over OTLP.",
			Details: "Such as `https://otel.example.com:4318`, or port 4317 for `grpc`. " +
				"For `http/protobuf`, nbpdns adds `/v1/traces` to the path of the URL, as the OTLP specification says. " +
				"If it's unset, nothing is exported. Every command exports its spans, and sends the last of them as it ends.",
			Warning: "With an `http://` URL, the spans, and any `otlp.headers`, such as a token, " +
				"cross the network unencrypted. nbpdns logs a warning each time it exports that way.",
		}, checkURL),
		enumKey(&c.OTLP.Protocol, Key{
			Name:    "otlp.protocol",
			Summary: "The OTLP protocol to export spans with.",
			Default: OTLPHTTP,
		}, OTLPHTTP, OTLPGRPC),
		secretKey(&c.OTLP.Headers, Key{
			Name:    "otlp.headers",
			Summary: "Headers to send with every export, such as the collector's token, as `name=value,name=value`.",
			Details: "Over `grpc`, they're sent as metadata. Percent-encode a comma in a value as `%2C`.",
		}, func(s string) error { _, err := ParseHeaders(s); return err }),
		stringKey(&c.OTLP.CAFile, "path", "path", Key{
			Name:    "otlp.ca_file",
			Summary: "A PEM file of CA certificates to trust for the collector, as well as the system's.",
		}, nil),
		durationKey(&c.OTLP.Timeout, Key{
			Name:    "otlp.timeout",
			Summary: "How long one export may take, and how long a command waits to send its last spans as it ends.",
			Details: "Write it with a unit, such as `10s`.",
			Default: "10s",
		}),
	}
	slices.SortFunc(ks, func(a, b Key) int { return cmp.Compare(a.Name, b.Name) })
	return ks
}

// Keys returns every configuration key, sorted by name, for the generated
// reference.
func Keys() []Key { return keys(&Config{}) }
