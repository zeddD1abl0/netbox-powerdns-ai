package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

func newAuditCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Read and verify the audit trail of nbpdns's changes",
		Long: "Read and verify the audit trail in the database at database.url: an event for\n" +
			"every change of nbpdns's state, each holding the SHA-384 of its contents and\n" +
			"of the event before it, so that a changed, removed, or inserted event shows.",
		Args: usageArgs(cobra.NoArgs),
		RunE: showHelp,
	}
	cmd.AddCommand(newAuditListCmd(a), newAuditVerifyCmd(a))
	return cmd
}

func newAuditListCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the latest audit events, oldest first",
		Long: "List the latest audit events, oldest first, that the flags select: by default,\n" +
			"the last 100. The table leaves out each event's before and after values,\n" +
			"and its hashes, which the JSON output includes.",
		Args: usageArgs(cobra.NoArgs),
	}
	output := addOutputFlag(cmd)
	f := cmd.Flags()
	since := f.String("since", "", "List events from this time, in RFC 3339 such as 2026-10-01T00:00:00Z, or this long ago, such as 24h.")
	until := f.String("until", "", "List events before this time, in RFC 3339, or this long ago.")
	action := f.String("action", "", "List the events of this action, such as service.started.")
	actor := f.String("actor", "", "List the events of this actor, such as os-user:jordan or system.")
	target := f.String("target", "", "List the events about this target, as type:id, or about every target of a type, such as setting.")
	limit := f.Int("limit", 100, "List at most this many events, the latest.")
	for name, md := range map[string]string{
		"since":  "List events from this time, in RFC 3339 such as `2026-10-01T00:00:00Z`, or this long ago, such as `24h`.",
		"until":  "List events before this time, in RFC 3339, or this long ago.",
		"action": "List the events of this action, such as `service.started`.",
		"actor":  "List the events of this actor, such as `os-user:jordan` or `system`.",
		"target": "List the events about this target, as `type:id`, or about every target of a type, such as `setting`.",
	} {
		_ = f.SetAnnotation(name, config.MarkdownUsage, []string{md})
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		now := time.Now()
		filter := store.AuditFilter{Action: audit.Action(*action), Actor: *actor, Limit: *limit}
		var err error
		if filter.Since, err = parseWhen("--since", *since, now); err != nil {
			return err
		}
		if filter.Until, err = parseWhen("--until", *until, now); err != nil {
			return err
		}
		if *limit < 1 {
			return usageError{fmt.Errorf("--limit %d isn't at least 1", *limit)}
		}
		filter.TargetType, filter.TargetID, _ = strings.Cut(*target, ":")
		return a.run(cmd, func(ctx context.Context, s *session) error {
			st, err := s.currentStore(ctx)
			if err != nil {
				return err
			}
			defer st.Close()
			events, err := st.ListAudit(ctx, filter)
			if err != nil {
				return err
			}
			if *output == outputJSON {
				out := make([]auditEvent, len(events))
				for i, e := range events {
					out[i] = auditEventOf(e)
				}
				return writeJSON(a.stdout, out)
			}
			return writeAuditEvents(a.stdout, events)
		})
	}
	return cmd
}

func newAuditVerifyCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Check that no audit event was changed, removed or inserted",
		Long: "Walk the whole audit trail, in order, and check each event's hash, and its\n" +
			"link to the event before it. It fails, with exit status 1, at the first event\n" +
			"where the chain breaks. The chain can't show that the latest events were\n" +
			"removed, or that the whole trail was rewritten: a copy kept elsewhere can.",
		Args: usageArgs(cobra.NoArgs),
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return a.run(cmd, func(ctx context.Context, s *session) error {
			st, err := s.currentStore(ctx)
			if err != nil {
				return err
			}
			defer st.Close()
			sum, err := st.VerifyAudit(ctx)
			if err != nil {
				return err
			}
			if sum.Count == 0 {
				fmt.Fprintln(a.stdout, "The audit trail is empty.")
				return nil
			}
			fmt.Fprintf(a.stdout, "The audit trail is intact: %d events, from event %d, at %s, to event %d, at %s.\n",
				sum.Count, sum.First.Seq, sum.First.Time.Format(time.RFC3339), sum.Last.Seq, sum.Last.Time.Format(time.RFC3339))
			if sum.Anchor != nil {
				fmt.Fprintf(a.stdout, "It starts after event %d, the last one that a prune deleted.\n", sum.Anchor.Seq)
			}
			return nil
		})
	}
	return cmd
}

// currentStore opens the database at database.url, which must exist and be
// migrated.
func (s *session) currentStore(ctx context.Context) (*store.Store, error) {
	st, err := s.openStore(ctx, false)
	if err != nil {
		return nil, err
	}
	ss, err := st.Status(ctx)
	switch {
	case err != nil:
		_ = st.Close()
		return nil, err
	case ss.Version > ss.Latest:
		_ = st.Close()
		return nil, &store.TooNewError{Version: ss.Version, Latest: ss.Latest}
	case ss.Pending():
		_ = st.Close()
		return nil, fmt.Errorf("the database's schema is at version %d, and this nbpdns needs %d; "+
			"run nbpdns db migrate, or start nbpdns serve, which migrates it", ss.Version, ss.Latest)
	}
	return st, nil
}

// parseWhen reads a flag's time: in RFC 3339, or as a duration ago from now.
// Empty is the zero time.
func parseWhen(flag, s string, now time.Time) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return now.Add(-d), nil
	}
	return time.Time{}, usageError{fmt.Errorf("%s %q isn't a time in RFC 3339, such as 2026-10-01T00:00:00Z, or a duration, such as 24h", flag, s)}
}

// auditEvent is an event as --output json writes it.
type auditEvent struct {
	Seq       int64           `json:"seq"`
	Time      time.Time       `json:"time"`
	Actor     string          `json:"actor"`
	Action    string          `json:"action"`
	Target    string          `json:"target"`
	Before    json.RawMessage `json:"before"`
	After     json.RawMessage `json:"after"`
	Reason    string          `json:"reason"`
	Outcome   string          `json:"outcome"`
	TraceID   string          `json:"trace_id"`
	RequestID string          `json:"request_id"`
	SourceIP  string          `json:"source_ip"`
	PrevHash  string          `json:"prev_hash"`
	Hash      string          `json:"hash"`
}

func auditEventOf(e audit.Event) auditEvent {
	return auditEvent{
		Seq: e.Seq, Time: e.Time, Actor: e.Actor.String(), Action: string(e.Action), Target: e.Target.String(),
		Before: e.Before, After: e.After, Reason: e.Reason, Outcome: string(e.Outcome), TraceID: e.TraceID,
		RequestID: e.RequestID, SourceIP: e.SourceIP, PrevHash: e.PrevHash, Hash: e.Hash,
	}
}

// writeAuditEvents writes events as a table.
func writeAuditEvents(w io.Writer, events []audit.Event) error {
	rows := make([][]string, len(events))
	for i, e := range events {
		reason := e.Reason
		if reason == "" {
			reason = "-"
		}
		rows[i] = []string{strconv.FormatInt(e.Seq, 10), e.Time.Format(time.RFC3339), e.Actor.String(), string(e.Action),
			e.Target.String(), string(e.Outcome), reason}
	}
	return writeTable(w, []string{"SEQ", "TIME", "ACTOR", "ACTION", "TARGET", "OUTCOME", "REASON"}, rows)
}
