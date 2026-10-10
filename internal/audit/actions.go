package audit

// An Action is what an event records. Each is declared once, in defs, with
// its target's type and what it means, and the reference,
// docs/reference/audit-events.md, is generated from them. The store refuses
// an event whose action isn't declared, or whose target is of another type.
type Action string

// The actions.
const (
	ServiceStarted Action = "service.started"
	ServiceStopped Action = "service.stopped"
	AuditPruned    Action = "audit.pruned"
	// ZoneDriftChanged is ADR-0008's drift detection.
	ZoneDriftChanged Action = "zone.drift_changed"
	SettingSet       Action = "setting.set"
	SettingUnset     Action = "setting.unset"
)

// Def declares an action.
type Def struct {
	Action Action
	// Target is the type of its events' targets.
	Target string
	// TargetID says what names the target, in Markdown.
	TargetID string
	// Actor says who makes it, in Markdown.
	Actor string
	// Summary says when it's recorded, in Markdown.
	Summary string
	// Before and After say what its events' Before and After hold, in
	// Markdown, or are empty if they hold nothing.
	Before, After string
}

// defs declares every action, in the reference's order.
var defs = []Def{
	{
		Action: ServiceStarted, Target: "service", TargetID: "the host's name",
		Actor:   "`system`",
		Summary: "`nbpdns serve` started, and opened its database.",
		After: "`version`, nbpdns's version, and `config_fingerprint`, the SHA-384 of its configuration's values and sources, " +
			"with each secret only as set or not, so that a change of configuration between two starts shows.",
	},
	{
		Action: ServiceStopped, Target: "service", TargetID: "the host's name",
		Actor: "`system`",
		Summary: "`nbpdns serve` stopped, on SIGINT or SIGTERM, or because it failed after it started, such as when it couldn't listen. " +
			"A serve that's killed, or crashes, records nothing.",
		After: "`uptime_seconds`, how long it ran, and `error`, why it failed, if it did.",
	},
	{
		Action: AuditPruned, Target: "audit", TargetID: "`trail`",
		Actor:   "`system`",
		Summary: "`nbpdns serve` deleted the events older than `audit.retention_days`, as it does daily.",
		Before:  "`first_seq`, the first event before the prune.",
		After: "`deleted`, how many events it deleted; `anchor_seq` and `anchor_hash`, the last one deleted, " +
			"which the first remaining one links to; and `retention_days`.",
	},
	{
		Action: ZoneDriftChanged, Target: "zone", TargetID: "the server group and the zone, as `<group>/<zone>`, such as `lab-a/example.com.`",
		Actor: "`system`",
		Summary: "A refresh of `nbpdns serve` found a zone in another drift state than its last, a new zone that drifted, " +
			"or that a zone left its group's report. A zone whose changes differ, in the same state, has a new row in its drift history, but no event.",
		Before: "`state`, the zone's last state, unless it's new.",
		After: "`state`, its state now, which is `removed` if it left the report; `changes`, how many of its RRsets differ; " +
			"and `policy`, its drift policy.",
	},
	{
		Action: SettingSet, Target: "setting", TargetID: "its key, such as `drift.interval`",
		Actor:   "`os-user:<name>`",
		Summary: "`nbpdns settings set` stored a runtime setting's value in the database, which `nbpdns serve` applies within seconds, unless the key is pinned.",
		Before:  "`value`, the stored value before, if there was one; `[redacted]` for a secret.",
		After:   "`value`, the value stored; `[redacted]` for a secret.",
	},
	{
		Action: SettingUnset, Target: "setting", TargetID: "its key",
		Actor:   "`os-user:<name>`",
		Summary: "`nbpdns settings unset` removed a runtime setting's stored value, so that the key takes its default, unless it's pinned.",
		Before:  "`value`, the stored value; `[redacted]` for a secret.",
	},
}

// Defs returns every action's declaration.
func Defs() []Def { return append([]Def(nil), defs...) }

// Lookup returns the declaration of action a.
func Lookup(a Action) (Def, bool) {
	for _, d := range defs {
		if d.Action == a {
			return d, true
		}
	}
	return Def{}, false
}
