package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/netbox"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/powerdns"
)

func newPowerDNSCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "powerdns",
		Short: "Read the zones of each PowerDNS server group's primary",
		Long: "Read the zones and records of each PowerDNS server group's primary, through\n" +
			"the PowerDNS API. Server groups are declared in the config file. These\n" +
			"commands only read: nbpdns never changes PowerDNS.",
		Args: usageArgs(cobra.NoArgs),
		RunE: showHelp,
	}
	cmd.AddCommand(newPowerDNSCheckCmd(a), newPowerDNSZonesCmd(a), newPowerDNSRecordsCmd(a))
	return cmd
}

// addGroupFlag adds --group to cmd.
func addGroupFlag(cmd *cobra.Command, usage, markdown string) *string {
	group := cmd.Flags().String("group", "", usage)
	_ = cmd.Flags().SetAnnotation("group", config.MarkdownUsage, []string{markdown})
	return group
}

// groups returns the server group named name, or every group if name is
// empty. It's an error if no group is declared, or none has that name.
func (s *session) groups(name string) ([]config.Group, error) {
	all := s.cfg.PowerDNS.Groups
	if len(all) == 0 {
		return nil, fmt.Errorf("no PowerDNS server groups are declared; declare them under %s in the config file", config.GroupsKey)
	}
	if name == "" {
		return all, nil
	}
	if g, ok := s.cfg.PowerDNS.Group(name); ok {
		return []config.Group{g}, nil
	}
	names := make([]string, len(all))
	for i, g := range all {
		names[i] = g.Name
	}
	return nil, fmt.Errorf("no server group is named %s; the groups are %s", name, strings.Join(names, ", "))
}

// powerdns returns a client for group g's primary. Close it when done.
func (s *session) powerdns(ctx context.Context, g config.Group) (*powerdns.Client, error) {
	return powerdns.New(ctx, powerdns.OptionsFrom(g, s.cfg.PowerDNS, s.log, s.tracer))
}

// groupCheck is one group's part of `nbpdns powerdns check`.
type groupCheck struct {
	Group   string        `json:"group"`
	URL     string        `json:"url"`
	Version string        `json:"version,omitempty"`
	OK      bool          `json:"ok"`
	Checks  []checkResult `json:"checks"`
}

// powerDNSReport is the output of `nbpdns powerdns check`.
type powerDNSReport struct {
	OK     bool         `json:"ok"`
	Groups []groupCheck `json:"groups"`
}

func newPowerDNSCheckCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check that nbpdns can read each server group's primary",
		Long: "Check that nbpdns can read each server group's primary: that its API answers,\n" +
			"is a PowerDNS Authoritative Server of a supported release, accepts the API\n" +
			"key, and lists its zones.\n\n" +
			"Each check passes, warns, or fails. If any check fails, nbpdns exits with\n" +
			"status 1. A warning, such as for an http:// URL, doesn't fail the check.",
		Args: usageArgs(cobra.NoArgs),
	}
	output := addOutputFlag(cmd)
	group := addGroupFlag(cmd, "Only check this server group.", "Only check this server group.")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return a.run(cmd, func(ctx context.Context, s *session) error {
			groups, err := s.groups(*group)
			if err != nil {
				return err
			}
			r := powerDNSReport{OK: true}
			for _, g := range groups {
				gc := checkGroup(ctx, s, g)
				r.OK = r.OK && gc.OK
				r.Groups = append(r.Groups, gc)
			}
			if *output == outputJSON {
				err = writeJSON(a.stdout, r)
			} else {
				err = writePowerDNSCheckTable(a.stdout, r)
			}
			if err != nil {
				return err
			}
			failed, total := 0, 0
			for _, g := range r.Groups {
				for _, c := range g.Checks {
					total++
					if c.Result == checkFailed {
						failed++
					}
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d of %d checks failed", failed, total)
			}
			return nil
		})
	}
	return cmd
}

// checkGroup checks group g's primary. It stops early when the checks left
// can't pass: when there's no client for it, such as for a CA file that
// can't be read; when the API can't be reached or rejects the key; or when
// the server isn't an authoritative one.
func checkGroup(ctx context.Context, s *session, g config.Group) groupCheck {
	r := groupCheck{Group: g.Name, URL: g.Primary.URL}
	add := func(name, result, detail string) {
		r.Checks = append(r.Checks, checkResult{Name: name, Result: result, Detail: detail})
	}
	c, err := s.powerdns(ctx, g)
	if err != nil {
		add("connection", checkFailed, err.Error())
		return r
	}
	defer c.Close()
	r.URL = c.URL()
	if c.Encrypted() {
		add("connection", checkOK, "https://")
	} else {
		add("connection", checkWarning, "http://, so the API key, which can change every zone, crosses the network unencrypted")
	}

	server, err := c.Server(ctx)
	var ae *powerdns.AuthError
	switch {
	case errors.As(err, &ae):
		add("key", checkFailed, err.Error())
	case err != nil:
		add("server", checkFailed, err.Error())
	case !server.Authoritative():
		add("server", checkFailed, c.Check(server).Error())
	default:
		r.Version = server.Version
		if err := c.Check(server); err != nil {
			add("server", checkFailed, err.Error())
		} else {
			add("server", checkOK, "PowerDNS Authoritative Server "+server.Version)
		}
		add("key", checkOK, "accepted")
		if zones, err := c.Zones(ctx); err != nil {
			add("zones", checkFailed, err.Error())
		} else {
			add("zones", checkOK, fmt.Sprintf("can read %d", len(zones)))
		}
	}
	r.OK = !slices.ContainsFunc(r.Checks, func(c checkResult) bool { return c.Result == checkFailed })
	return r
}

func writePowerDNSCheckTable(w io.Writer, r powerDNSReport) error {
	var rows [][]string
	for _, g := range r.Groups {
		for _, c := range g.Checks {
			rows = append(rows, []string{g.Group, c.Name, c.Result, c.Detail})
		}
	}
	return writeTable(w, []string{"GROUP", "CHECK", "RESULT", "DETAIL"}, rows)
}

// groupZone is one zone in the output of `nbpdns powerdns zones`.
type groupZone struct {
	Group   string `json:"group"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Serial  uint32 `json:"serial"`
	Catalog string `json:"catalog,omitempty"`
}

func newPowerDNSZonesCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "zones",
		Short: "List the zones on each server group's primary",
		Long: "List the zones on each server group's primary, with each one's kind, SOA\n" +
			"serial, and the catalog zone it's a member of, if any. Names are absolute\n" +
			"and lowercase, as nbpdns compares them.",
		Args: usageArgs(cobra.NoArgs),
	}
	output := addOutputFlag(cmd)
	group := addGroupFlag(cmd, "Only list this server group's zones.", "Only list this server group's zones.")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return a.run(cmd, func(ctx context.Context, s *session) error {
			groups, err := s.groups(*group)
			if err != nil {
				return err
			}
			out := []groupZone{}
			for _, g := range groups {
				zones, err := groupZones(ctx, s, g)
				if err != nil {
					return err
				}
				for _, z := range zones {
					out = append(out, groupZone{Group: g.Name, Name: dns.Name(z.Name, "."), Kind: z.Kind, Serial: z.Serial,
						Catalog: z.Catalog})
				}
			}
			slices.SortFunc(out, func(a, b groupZone) int {
				return cmp.Or(strings.Compare(a.Group, b.Group), dns.CompareNames(a.Name, b.Name))
			})
			if *output == outputJSON {
				return writeJSON(a.stdout, out)
			}
			rows := make([][]string, len(out))
			for i, z := range out {
				rows[i] = []string{z.Group, z.Name, z.Kind, strconv.FormatUint(uint64(z.Serial), 10), z.Catalog}
			}
			return writeTable(a.stdout, []string{"GROUP", "ZONE", "KIND", "SERIAL", "CATALOG"}, rows)
		})
	}
	return cmd
}

// groupZones lists the zones on group g's primary.
func groupZones(ctx context.Context, s *session, g config.Group) ([]powerdns.Zone, error) {
	c, err := s.powerdns(ctx, g)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if _, err := c.Connect(ctx); err != nil {
		return nil, err
	}
	return c.Zones(ctx)
}

// groupRecords is the JSON output of `nbpdns powerdns records`.
type groupRecords struct {
	Group string `json:"group"`
	dns.Zone
	// Problems are what nbpdns worked around in PowerDNS's data. They're
	// never null, so scripts can count them.
	Problems []dns.Problem `json:"problems"`
}

func newPowerDNSRecordsCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "records --zone NAME [--group GROUP]",
		Short: "List a zone's records on a server group's primary, as nbpdns normalizes them",
		Long: "List the records of one zone on a server group's primary, as nbpdns normalizes\n" +
			"them: grouped into RRsets, with absolute lowercase names, canonical values,\n" +
			"and each RRset's TTL. Records that PowerDNS keeps but doesn't serve are\n" +
			"listed too, with their status.\n\n" +
			"Give the zone's name in its ASCII form. If more than one server group is\n" +
			"declared, choose one with --group.\n\n" +
			"Where PowerDNS's data has a problem that nbpdns works around, such as a value\n" +
			"that doesn't parse as its type, nbpdns logs a warning for each one and lists\n" +
			"it under \"problems\" in JSON output. Problems don't make the command fail.",
		Args: usageArgs(cobra.NoArgs),
	}
	output := addOutputFlag(cmd)
	zone := cmd.Flags().String("zone", "", "The zone's name, such as example.com. Required.")
	_ = cmd.Flags().SetAnnotation("zone", config.MarkdownUsage, []string{"The zone's name, such as `example.com`. Required."})
	group := addGroupFlag(cmd, "The server group, if more than one is declared.", "The server group, if more than one is declared.")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if *zone == "" {
			return usageError{errors.New("--zone is required")}
		}
		name, err := netbox.ZoneName(*zone)
		if err != nil {
			return usageError{err}
		}
		return a.run(cmd, func(ctx context.Context, s *session) error {
			groups, err := s.groups(*group)
			if err != nil {
				return err
			}
			if len(groups) > 1 {
				return fmt.Errorf("%d server groups are declared; name one with --group", len(groups))
			}
			g := groups[0]
			c, err := s.powerdns(ctx, g)
			if err != nil {
				return err
			}
			defer c.Close()
			if _, err := c.Connect(ctx); err != nil {
				return err
			}
			z, err := c.FindZone(ctx, name)
			if err != nil {
				return err
			}
			zones, probs, err := c.ReadZones(ctx, []powerdns.Zone{z})
			if err != nil {
				return err
			}
			for _, p := range probs {
				s.log.WarnContext(ctx, "nbpdns worked around a problem in PowerDNS's data",
					"group", g.Name, "zone", p.Zone, "name", p.Name, "type", p.Type, "problem", p.Detail)
			}
			if *output == outputJSON {
				return writeJSON(a.stdout, groupRecords{Group: g.Name, Zone: zones[0], Problems: append([]dns.Problem{}, probs...)})
			}
			return writePowerDNSRecords(a.stdout, zones[0])
		})
	}
	return cmd
}

func writePowerDNSRecords(w io.Writer, z dns.Zone) error {
	var rows [][]string
	for _, s := range z.RRsets {
		for _, r := range s.Records {
			rows = append(rows, []string{s.Name, strconv.FormatUint(uint64(s.TTL), 10), s.Type, r.Value, r.Status})
		}
	}
	return writeTable(w, []string{"NAME", "TTL", "TYPE", "VALUE", "STATUS"}, rows)
}
