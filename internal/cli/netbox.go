package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/netbox"
)

func newNetBoxCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "netbox",
		Short: "Read DNS data from NetBox's DNS plugin",
		Long: "Read DNS data from NetBox's DNS plugin, the source of truth, through\n" +
			"NetBox's REST API. These commands only read: nbpdns never changes NetBox.",
		Args: usageArgs(cobra.NoArgs),
		RunE: showHelp,
	}
	cmd.AddCommand(newNetBoxCheckCmd(a), newNetBoxZonesCmd(a), newNetBoxRecordsCmd(a))
	return cmd
}

// netbox returns a client for the configured NetBox. Close it when done.
func (s *session) netbox(ctx context.Context) (*netbox.Client, error) {
	return netbox.New(ctx, netbox.OptionsFrom(s.cfg.NetBox, s.log, s.tracer))
}

// The results of a check.
const (
	checkOK      = "ok"
	checkWarning = "warning"
	checkFailed  = "failed"
)

// A checkResult is one thing that `nbpdns netbox check` checks.
type checkResult struct {
	Name   string `json:"name"`
	Result string `json:"result"`
	Detail string `json:"detail"`
}

// checkReport is the output of `nbpdns netbox check`.
type checkReport struct {
	NetBoxURL     string        `json:"netbox_url"`
	NetBoxVersion string        `json:"netbox_version,omitempty"`
	PluginVersion string        `json:"plugin_version,omitempty"`
	OK            bool          `json:"ok"`
	Checks        []checkResult `json:"checks"`
}

func (r *checkReport) add(name, result, detail string) {
	r.Checks = append(r.Checks, checkResult{Name: name, Result: result, Detail: detail})
}

func (r *checkReport) failed() int {
	n := 0
	for _, c := range r.Checks {
		if c.Result == checkFailed {
			n++
		}
	}
	return n
}

func newNetBoxCheckCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check that nbpdns can read DNS data from NetBox",
		Long: "Check that nbpdns can read DNS data from NetBox: that NetBox answers, runs a\n" +
			"supported release with the DNS plugin, accepts the token, and lets the\n" +
			"token's user view the plugin's views, zones, name servers, and records.\n\n" +
			"Each check passes, warns, or fails. If any check fails, nbpdns exits with\n" +
			"status 1. A warning, such as for an http:// URL or a v1 token, doesn't fail\n" +
			"the check.",
		Args: usageArgs(cobra.NoArgs),
	}
	output := addOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return a.run(cmd, func(ctx context.Context, s *session) error {
			c, err := s.netbox(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			r := runChecks(ctx, c)
			if *output == outputJSON {
				err = writeJSON(a.stdout, r)
			} else {
				err = writeCheckTable(a.stdout, r)
			}
			if err != nil {
				return err
			}
			if n := r.failed(); n > 0 {
				return fmt.Errorf("%d of %d checks failed", n, len(r.Checks))
			}
			return nil
		})
	}
	return cmd
}

// runChecks checks c's NetBox. It stops early when the checks left can't
// pass: when NetBox can't be reached or rejects the token, or when the DNS
// plugin isn't installed.
func runChecks(ctx context.Context, c *netbox.Client) checkReport {
	r := checkReport{NetBoxURL: c.URL()}
	if c.Encrypted() {
		r.add("connection", checkOK, "https://")
	} else {
		r.add("connection", checkWarning, "http://, so the token crosses the network unencrypted")
	}

	st, err := c.Status(ctx)
	var ae *netbox.AuthError
	switch {
	case errors.As(err, &ae):
		r.add("token", checkFailed, err.Error())
		return r
	case err != nil:
		r.add("netbox", checkFailed, err.Error())
		return r
	}
	r.NetBoxVersion, r.PluginVersion = st.NetBoxVersion, st.PluginVersion()

	nb, plugin := netbox.SupportedSeries()
	if st.NetBoxSupported() {
		r.add("netbox", checkOK, "NetBox "+st.NetBoxVersion)
	} else {
		r.add("netbox", checkFailed, fmt.Sprintf("NetBox %s isn't a supported release; nbpdns supports %s", st.NetBoxVersion, nb))
	}
	switch {
	case st.PluginVersion() == "":
		r.add("plugin", checkFailed, "the NetBox DNS plugin isn't installed")
	case st.PluginSupported():
		r.add("plugin", checkOK, netbox.PluginName+" "+st.PluginVersion())
	default:
		r.add("plugin", checkFailed, fmt.Sprintf("%s %s isn't a supported release; nbpdns supports %s",
			netbox.PluginName, st.PluginVersion(), plugin))
	}

	if c.TokenVersion() == 2 {
		r.add("token", checkOK, "a v2 token, accepted")
	} else {
		r.add("token", checkWarning, "a v1 token, accepted; NetBox recommends v2 tokens, which start with nbt_")
	}

	if st.PluginVersion() == "" {
		return r
	}
	for _, typ := range netbox.ObjectTypes {
		if n, err := c.Count(ctx, typ); err != nil {
			r.add(typ, checkFailed, err.Error())
		} else {
			r.add(typ, checkOK, fmt.Sprintf("can view %d", n))
		}
	}
	r.OK = r.failed() == 0
	return r
}

func writeCheckTable(w io.Writer, r checkReport) error {
	rows := make([][]string, len(r.Checks))
	for i, c := range r.Checks {
		rows[i] = []string{c.Name, c.Result, c.Detail}
	}
	return writeTable(w, []string{"CHECK", "RESULT", "DETAIL"}, rows)
}

func newNetBoxZonesCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "zones",
		Short: "List the zones in NetBox",
		Long: "List the zones in NetBox's DNS plugin, with each one's view, status, SOA\n" +
			"serial, default TTL, and name servers. Names are absolute and lowercase, in\n" +
			"their ASCII form, as nbpdns compares them.\n\n" +
			"With --group, list only the zones that a PowerDNS server group serves: those\n" +
			"in the views it lists. One PowerDNS server holds one zone of each name, so a\n" +
			"zone name in two of those views is logged as a problem.",
		Args: usageArgs(cobra.NoArgs),
	}
	output := addOutputFlag(cmd)
	view := cmd.Flags().String("view", "", "Only list the zones in this view.")
	status := cmd.Flags().String("status", "", "Only list the zones with this status, such as active.")
	_ = cmd.Flags().SetAnnotation("status", config.MarkdownUsage, []string{"Only list the zones with this status, such as `active`."})
	group := addGroupFlag(cmd, "Only list the zones this PowerDNS server group serves.", "Only list the zones this PowerDNS server group serves.")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if *view != "" && *group != "" {
			return usageError{errors.New("--view and --group can't be used together; a group chooses its views")}
		}
		return a.run(cmd, func(ctx context.Context, s *session) error {
			views := []string{*view}
			if *group != "" {
				groups, err := s.groups(*group)
				if err != nil {
					return err
				}
				views = groups[0].Views
			}
			c, err := s.netbox(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			if _, err := c.Connect(ctx); err != nil {
				return err
			}
			zones, err := c.Zones(ctx, netbox.ZoneFilter{Views: views, Status: *status})
			if err != nil {
				return err
			}
			if *group != "" {
				for _, p := range sharedNames(*group, zones) {
					s.log.WarnContext(ctx, "a PowerDNS server group serves two zones of one name, which one server can't hold",
						"group", *group, "zone", p.Zone, "problem", p.Detail)
				}
			}
			out := make([]dns.Zone, len(zones))
			for i, z := range zones {
				out[i] = z.DNS()
			}
			slices.SortFunc(out, func(a, b dns.Zone) int {
				return cmp.Or(strings.Compare(a.View, b.View), dns.CompareNames(a.Name, b.Name))
			})
			if *output == outputJSON {
				return writeJSON(a.stdout, out)
			}
			rows := make([][]string, len(out))
			for i, z := range out {
				rows[i] = []string{z.View, z.Name, z.Status, strconv.FormatUint(uint64(z.SOASerial), 10),
					strconv.FormatUint(uint64(z.DefaultTTL), 10), strings.Join(z.Nameservers, ",")}
			}
			return writeTable(a.stdout, []string{"VIEW", "ZONE", "STATUS", "SERIAL", "DEFAULT TTL", "NAMESERVERS"}, rows)
		})
	}
	return cmd
}

// sharedNames returns a problem for each zone name that's in more than one
// of zones' views, which group serves.
func sharedNames(group string, zones []netbox.Zone) []dns.Problem {
	views := map[string][]string{}
	for _, z := range zones {
		name := dns.Name(z.Name, ".")
		views[name] = append(views[name], z.View.Name)
	}
	var probs []dns.Problem
	for _, name := range slices.Sorted(maps.Keys(views)) {
		if vs := views[name]; len(vs) > 1 {
			slices.Sort(vs)
			probs = append(probs, dns.Problem{Zone: name,
				Detail: fmt.Sprintf("server group %s serves it from the views %s", group, strings.Join(vs, ", "))})
		}
	}
	return probs
}

// zoneRecords is the JSON output of `nbpdns netbox records`.
type zoneRecords struct {
	dns.Zone
	// Problems are what nbpdns worked around in NetBox's data. They're
	// never null, so scripts can count them.
	Problems []dns.Problem `json:"problems"`
}

func newNetBoxRecordsCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "records --zone NAME",
		Short: "List a zone's records, as nbpdns normalizes them",
		Long: "List the records of one zone in NetBox's DNS plugin, as nbpdns normalizes\n" +
			"them: grouped into RRsets, with absolute lowercase names, canonical values,\n" +
			"and each RRset's TTL. Inactive records are listed too, with their status.\n" +
			"The table's TTL is the RRset's, which DNS serves, except for an inactive\n" +
			"record, which shows its own.\n\n" +
			"Give the zone's name in its ASCII form. If the name is in more than one view,\n" +
			"choose one with --view.\n\n" +
			"Where NetBox's data has a problem that nbpdns works around, such as active\n" +
			"records in one RRset with different TTLs, nbpdns logs a warning for each one\n" +
			"and lists it under \"problems\" in JSON output. Problems don't make the\n" +
			"command fail.",
		Args: usageArgs(cobra.NoArgs),
	}
	output := addOutputFlag(cmd)
	zone := cmd.Flags().String("zone", "", "The zone's name, such as example.com. Required.")
	_ = cmd.Flags().SetAnnotation("zone", config.MarkdownUsage, []string{"The zone's name, such as `example.com`. Required."})
	view := cmd.Flags().String("view", "", "The zone's view, if its name is in more than one.")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if *zone == "" {
			return usageError{errors.New("--zone is required")}
		}
		name, err := dns.ZoneName(*zone)
		if err != nil {
			return usageError{err}
		}
		return a.run(cmd, func(ctx context.Context, s *session) error {
			c, err := s.netbox(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			if _, err := c.Connect(ctx); err != nil {
				return err
			}
			z, err := c.FindZone(ctx, name, *view)
			var ae *netbox.AmbiguousZoneError
			if errors.As(err, &ae) {
				return fmt.Errorf("%w with --view", err)
			} else if err != nil {
				return err
			}
			zones, probs, err := c.ReadZones(ctx, []netbox.Zone{z})
			if err != nil {
				return err
			}
			for _, p := range probs {
				s.log.WarnContext(ctx, "nbpdns worked around a problem in NetBox's data; fix it in NetBox",
					"zone", p.Zone, "name", p.Name, "type", p.Type, "problem", p.Detail)
			}
			return writeRecords(a.stdout, *output, zoneRecords{Zone: zones[0], Problems: append([]dns.Problem{}, probs...)})
		})
	}
	return cmd
}

func writeRecords(w io.Writer, format outputFormat, zr zoneRecords) error {
	if format == outputJSON {
		return writeJSON(w, zr)
	}
	var rows [][]string
	for _, s := range zr.RRsets {
		for _, r := range s.Records {
			managed := "no"
			if r.Managed {
				managed = "yes"
			}
			// An inactive record isn't served, so the RRset's TTL isn't its.
			ttl := s.TTL
			if !r.Active {
				ttl = r.TTL
			}
			rows = append(rows, []string{s.Name, strconv.FormatUint(uint64(ttl), 10), s.Type, r.Value, r.Status, managed})
		}
	}
	return writeTable(w, []string{"NAME", "TTL", "TYPE", "VALUE", "STATUS", "MANAGED"}, rows)
}
