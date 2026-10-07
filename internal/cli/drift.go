package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/netbox"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/powerdns"
)

// driftError means the report found drift: everything was compared, and
// NetBox and PowerDNS differ. It exits with exitDrift.
type driftError struct{ zones int }

func (e driftError) Error() string {
	if e.zones == 1 {
		return "1 zone drifted"
	}
	return fmt.Sprintf("%d zones drifted", e.zones)
}

func newDriftCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Compare NetBox with each server group's primary, and report the drift",
		Long: "Compare the zones NetBox assigns to each PowerDNS server group, through its\n" +
			"views, with what the group's primary serves, and report every difference.\n" +
			"Only what each side serves is compared: NetBox's active records, and the\n" +
			"primary's records apart from those turned off. An SOA is compared without its\n" +
			"serial. Zones on a primary that NetBox doesn't assign to its group are listed\n" +
			"as unmanaged, and aren't drift. A zone whose drift policy is ignore isn't\n" +
			"compared. nbpdns only reads, and changes nothing.\n\n" +
			"nbpdns exits with status 0 if there's no drift, 3 if there is, and 1 if it\n" +
			"couldn't compare everything. Nothing is reported if NetBox can't be read, or\n" +
			"if the zone that --zone names is neither in a group's NetBox views nor on its\n" +
			"primary. If a primary can't be read, its group is marked failed, and the\n" +
			"other groups are still reported.",
		Args: usageArgs(cobra.NoArgs),
	}
	output := addOutputFlag(cmd)
	group := addGroupFlag(cmd, "Only compare this server group.", "Only compare this server group.")
	zone := cmd.Flags().String("zone", "", "Only compare this zone, such as example.com.")
	_ = cmd.Flags().SetAnnotation("zone", config.MarkdownUsage, []string{"Only compare this zone, such as `example.com`."})
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		var name string
		if *zone != "" {
			n, err := dns.ZoneName(*zone)
			if err != nil {
				return usageError{err}
			}
			name = n + "."
		}
		return a.run(cmd, func(ctx context.Context, s *session) error {
			groups, err := s.groups(*group)
			if err != nil {
				return err
			}
			nb, err := s.netbox(ctx)
			if err != nil {
				return err
			}
			defer nb.Close()
			if _, err := nb.Connect(ctx); err != nil {
				return err
			}
			sources := make([]drift.Group, len(groups))
			for i, g := range groups {
				sources[i] = drift.Group{Config: g}
				c, err := s.powerdns(ctx, g)
				if err == nil {
					defer c.Close()
					_, err = c.Connect(ctx)
				}
				if err != nil {
					sources[i].Err = err
					continue
				}
				sources[i].Primary = &primarySource{c: c}
			}
			r, err := drift.Run(ctx, &netboxSource{c: nb}, sources, drift.Options{Zone: name, Concurrency: s.cfg.Drift.GroupConcurrency})
			if err != nil {
				return err
			}
			for _, g := range r.Groups {
				for _, w := range g.Warnings {
					s.log.WarnContext(ctx, "the drift report worked around a problem", "group", g.Group, "warning", w)
				}
			}
			if *output == outputJSON {
				err = writeJSON(a.stdout, r)
			} else {
				err = writeDrift(a.stdout, r)
			}
			if err != nil {
				return err
			}
			return driftResult(r)
		})
	}
	return cmd
}

// driftResult returns the error that gives the report's exit status: one
// for a group that couldn't be read, which wins over drift, then one for
// drift.
func driftResult(r drift.Report) error {
	failed, drifted := 0, 0
	for _, g := range r.Groups {
		if g.Status != drift.StatusOK {
			failed++
		}
		drifted += g.Counts.DriftedZones()
	}
	switch {
	case !r.Complete:
		return fmt.Errorf("%d of %d server groups couldn't be read, so the report is incomplete", failed, len(r.Groups))
	case r.Drift:
		return driftError{zones: drifted}
	}
	return nil
}

// writeDrift writes the report as tables: a summary per group, then the
// drift, then the unmanaged and ignored zones, the failed groups, the
// problems and the warnings, each under a heading, and only if there are
// any.
func writeDrift(w io.Writer, r drift.Report) error {
	var summary [][]string
	for _, g := range r.Groups {
		c := g.Counts
		summary = append(summary, []string{g.Group, g.Status, strconv.Itoa(c.InSync), strconv.Itoa(c.Drift),
			strconv.Itoa(c.Missing), strconv.Itoa(c.Inactive), strconv.Itoa(c.Ignored), strconv.Itoa(c.Unmanaged)})
	}
	if err := writeTable(w, []string{"GROUP", "STATUS", "IN SYNC", "DRIFT", "MISSING", "INACTIVE", "IGNORED", "UNMANAGED"}, summary); err != nil {
		return err
	}
	var changes, unmanaged, ignored, failed, problems, warnings [][]string
	for _, g := range r.Groups {
		if g.Status != drift.StatusOK {
			failed = append(failed, []string{g.Group, g.Error})
		}
		for _, z := range g.Zones {
			switch z.State {
			case drift.StateMissing:
				changes = append(changes, []string{g.Group, z.Zone, policy(z.Policy), "zone missing on the primary", "", "", "", ""})
			case drift.StateInactive:
				changes = append(changes, []string{g.Group, z.Zone, policy(z.Policy), "zone served, but not active in NetBox", "", "", "", ""})
			case drift.StateIgnored:
				ignored = append(ignored, []string{g.Group, z.Zone})
			}
			for _, c := range z.Changes {
				changes = append(changes, []string{g.Group, z.Zone, policy(z.Policy), c.Kind, c.Name, c.Type, side(c.NetBox), side(c.PowerDNS)})
			}
		}
		for _, z := range g.Unmanaged {
			unmanaged = append(unmanaged, []string{g.Group, z})
		}
		for _, p := range g.Problems {
			problems = append(problems, []string{g.Group, p.Zone, strings.TrimSpace(p.Name + " " + p.Type), p.Detail})
		}
		for _, w := range g.Warnings {
			warnings = append(warnings, []string{g.Group, w})
		}
	}
	sections := []struct {
		title  string
		header []string
		rows   [][]string
	}{
		{"Drift", []string{"GROUP", "ZONE", "POLICY", "CHANGE", "NAME", "TYPE", "NETBOX", "POWERDNS"}, changes},
		{"Unmanaged zones, on a primary but not in its group's NetBox views", []string{"GROUP", "ZONE"}, unmanaged},
		{"Ignored zones, not compared", []string{"GROUP", "ZONE"}, ignored},
		{"Groups that couldn't be read", []string{"GROUP", "ERROR"}, failed},
		{"Problems in the data, worked around", []string{"GROUP", "ZONE", "RRSET", "PROBLEM"}, problems},
		{"Warnings about the configuration or NetBox's zones", []string{"GROUP", "WARNING"}, warnings},
	}
	for _, sec := range sections {
		if len(sec.rows) == 0 {
			continue
		}
		if _, err := fmt.Fprintf(w, "\n%s:\n", sec.title); err != nil {
			return err
		}
		if err := writeTable(w, sec.header, sec.rows); err != nil {
			return err
		}
	}
	return nil
}

// policy writes a zone's drift policy. Nothing is written until M13, so
// enforce is marked as acting from then (ADR-0027).
func policy(p string) string {
	if p == config.PolicyEnforce {
		return p + " (from M13)"
	}
	return p
}

// side writes an RRset as one side serves it: its TTL, then its values.
func side(s *drift.Side) string {
	if s == nil {
		return "-"
	}
	return strconv.FormatUint(uint64(s.TTL), 10) + " " + strings.Join(s.Values, ", ")
}

// netboxSource reads NetBox's zones for the drift report.
type netboxSource struct {
	c     *netbox.Client
	zones map[[2]string]netbox.Zone // by view and absolute name
}

func (n *netboxSource) Zones(ctx context.Context, views []string, zone string) ([]dns.Zone, error) {
	zones, err := n.c.Zones(ctx, netbox.ZoneFilter{Name: strings.TrimSuffix(zone, "."), Views: views})
	if err != nil {
		return nil, err
	}
	n.zones = map[[2]string]netbox.Zone{}
	out := make([]dns.Zone, len(zones))
	for i, z := range zones {
		out[i] = z.DNS()
		n.zones[[2]string{out[i].View, out[i].Name}] = z
	}
	return out, nil
}

func (n *netboxSource) Read(ctx context.Context, zones []dns.Zone) ([]dns.Zone, []dns.Problem, error) {
	nb := make([]netbox.Zone, 0, len(zones))
	for _, z := range zones {
		if found, ok := n.zones[[2]string{z.View, z.Name}]; ok {
			nb = append(nb, found)
		}
	}
	return n.c.ReadZones(ctx, nb)
}

// primarySource reads a server group's primary for the drift report.
type primarySource struct {
	c     *powerdns.Client
	zones map[string]powerdns.Zone // by absolute name
}

func (p *primarySource) Zones(ctx context.Context, zone string) ([]dns.Zone, error) {
	var zones []powerdns.Zone
	if zone == "" {
		var err error
		if zones, err = p.c.Zones(ctx); err != nil {
			return nil, err
		}
	} else {
		z, err := p.c.FindZone(ctx, zone)
		var nf *powerdns.ZoneNotFoundError
		switch {
		case errors.As(err, &nf):
		case err != nil:
			return nil, err
		default:
			zones = []powerdns.Zone{z}
		}
	}
	p.zones = map[string]powerdns.Zone{}
	out := make([]dns.Zone, len(zones))
	for i, z := range zones {
		out[i] = z.DNS()
		p.zones[out[i].Name] = z
	}
	return out, nil
}

func (p *primarySource) Read(ctx context.Context, zones []dns.Zone) ([]dns.Zone, []dns.Problem, error) {
	pd := make([]powerdns.Zone, 0, len(zones))
	for _, z := range zones {
		if found, ok := p.zones[z.Name]; ok {
			pd = append(pd, found)
		}
	}
	return p.c.ReadZones(ctx, pd)
}
