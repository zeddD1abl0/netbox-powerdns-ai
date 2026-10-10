package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

func newDriftHistoryCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show a server group's refreshes, or a zone's drift history",
		Long: "Show the drift history that nbpdns serve keeps in the database at\n" +
			"database.url, oldest first. Without --zone, it lists the latest refreshes,\n" +
			"each with the group's result. With --zone, it lists the zone's history: a row\n" +
			"for each refresh that found it in another state, or with other changes, than\n" +
			"the row before, which holds until the next row. The table counts each row's\n" +
			"changes, which the JSON output lists.",
		Args: usageArgs(cobra.NoArgs),
	}
	output := addOutputFlag(cmd)
	group := addGroupFlag(cmd, "The server group, such as site-a. Required.", "The server group, such as `site-a`. Required.")
	zone := cmd.Flags().String("zone", "", "Show this zone's history, such as example.com.")
	_ = cmd.Flags().SetAnnotation("zone", config.MarkdownUsage, []string{"Show this zone's history, such as `example.com`."})
	limit := cmd.Flags().Int("limit", 20, "Show at most this many, the latest.")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if *group == "" {
			return usageError{errors.New("--group is required")}
		}
		if *limit < 1 || *limit > 1000 {
			return usageError{fmt.Errorf("--limit %d isn't from 1 to 1000", *limit)}
		}
		var name string
		if *zone != "" {
			n, err := dns.ZoneName(*zone)
			if err != nil {
				return usageError{err}
			}
			name = n + "."
		}
		return a.run(cmd, func(ctx context.Context, s *session) error {
			st, err := s.currentStore(ctx)
			if err != nil {
				return err
			}
			defer st.Close()
			if name == "" {
				rows, err := st.Refreshes(ctx, *group, 0, *limit)
				if err != nil {
					return err
				}
				slices.Reverse(rows)
				if *output == outputJSON {
					return writeJSON(a.stdout, refreshesOut(rows))
				}
				return writeRefreshes(a.stdout, rows)
			}
			rows, err := st.ZoneHistory(ctx, *group, name, 0, *limit)
			if err != nil {
				return err
			}
			slices.Reverse(rows)
			if *output == outputJSON {
				return writeJSON(a.stdout, zoneHistoryOut(rows))
			}
			if len(rows) == 0 {
				fmt.Fprintf(a.stdout, "Server group %s has no history for %s.\n", *group, name)
				return nil
			}
			return writeZoneHistory(a.stdout, rows)
		})
	}
	return cmd
}

// zoneHistoryRow is a row of a zone's history, as --output json writes it.
type zoneHistoryRow struct {
	Time           time.Time      `json:"time"`
	RefreshID      int64          `json:"refresh_id"`
	State          string         `json:"state"`
	View           string         `json:"view"`
	Policy         string         `json:"policy"`
	NetBoxSerial   uint32         `json:"netbox_serial,omitempty"`
	PowerDNSSerial uint32         `json:"powerdns_serial,omitempty"`
	Changes        []drift.Change `json:"changes"`
}

func zoneHistoryOut(rows []store.ZoneHistoryRow) []zoneHistoryRow {
	out := make([]zoneHistoryRow, len(rows))
	for i, r := range rows {
		out[i] = zoneHistoryRow{Time: r.At, RefreshID: r.RefreshID, State: r.State, View: r.View, Policy: r.Policy,
			NetBoxSerial: r.NetBoxSerial, PowerDNSSerial: r.PowerDNSSerial, Changes: r.Changes}
		if out[i].Changes == nil {
			out[i].Changes = []drift.Change{}
		}
	}
	return out
}

func writeZoneHistory(w io.Writer, rows []store.ZoneHistoryRow) error {
	table := make([][]string, len(rows))
	for i, r := range rows {
		table[i] = []string{r.At.Format(time.RFC3339), r.State, strconv.Itoa(len(r.Changes)), r.Policy,
			serialText(r.NetBoxSerial), serialText(r.PowerDNSSerial), strconv.FormatInt(r.RefreshID, 10)}
	}
	return writeTable(w, []string{"TIME", "STATE", "CHANGES", "POLICY", "NETBOX SERIAL", "POWERDNS SERIAL", "REFRESH"}, table)
}

// serialText writes an SOA serial, or - for none.
func serialText(n uint32) string {
	if n == 0 {
		return "-"
	}
	return strconv.FormatUint(uint64(n), 10)
}

// refreshRow is a refresh, as --output json writes it.
type refreshRow struct {
	ID               int64         `json:"id"`
	Kind             string        `json:"kind"`
	Cause            string        `json:"cause"`
	Started          time.Time     `json:"started"`
	Finished         time.Time     `json:"finished"`
	Outcome          string        `json:"outcome"`
	Error            string        `json:"error,omitempty"`
	Zones            []string      `json:"zones,omitempty"`
	NetBoxRequestIDs []string      `json:"netbox_request_ids"`
	TraceID          string        `json:"trace_id"`
	Group            *groupOutcome `json:"group"`
}

type groupOutcome struct {
	Status string        `json:"status"`
	Error  string        `json:"error,omitempty"`
	Counts *drift.Counts `json:"counts,omitempty"`
}

func refreshesOut(rows []store.RefreshRow) []refreshRow {
	out := make([]refreshRow, len(rows))
	for i, r := range rows {
		out[i] = refreshRow{ID: r.ID, Kind: r.Kind, Cause: r.Cause, Started: r.Started, Finished: r.Finished, Outcome: r.Outcome,
			Error: r.Error, Zones: r.Zones, NetBoxRequestIDs: r.NetBoxRequestIDs, TraceID: r.TraceID}
		if out[i].NetBoxRequestIDs == nil {
			out[i].NetBoxRequestIDs = []string{}
		}
		if g := r.Group; g != nil {
			out[i].Group = &groupOutcome{Status: g.Status, Error: g.Error}
			if g.Status == drift.StatusOK {
				c := g.Counts
				out[i].Group.Counts = &c
			}
		}
	}
	return out
}

func writeRefreshes(w io.Writer, rows []store.RefreshRow) error {
	table := make([][]string, len(rows))
	for i, r := range rows {
		what := "every zone"
		if r.Kind == store.RefreshZones {
			what = strings.Join(r.Zones, ", ")
		}
		result, drifted := "-", "-"
		if g := r.Group; g != nil {
			result = g.Status
			if g.Status == drift.StatusOK {
				drifted = strconv.Itoa(g.Counts.Drift + g.Counts.Missing + g.Counts.Inactive)
			}
		}
		table[i] = []string{strconv.FormatInt(r.ID, 10), r.Finished.Format(time.RFC3339), r.Cause, r.Outcome, result, drifted, what}
	}
	return writeTable(w, []string{"ID", "FINISHED", "CAUSE", "OUTCOME", "GROUP", "DRIFTED", "ZONES"}, table)
}
