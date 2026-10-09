package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
)

// GroupsKey is the config file key that declares the PowerDNS server
// groups. Groups are structured resources, so only the config file sets
// them: there's no environment variable or flag (ADR-0026).
const GroupsKey = "powerdns.groups"

// The drift policies (ADR-0008, ADR-0027).
const (
	// PolicyEnforce corrects PowerDNS to match NetBox, from M14. Until then
	// it's reported like PolicyReport.
	PolicyEnforce = "enforce"
	// PolicyReport reports drift and changes nothing.
	PolicyReport = "report"
	// PolicyIgnore doesn't compare the zone.
	PolicyIgnore = "ignore"
)

// Policies are the drift policies, in the order the reference lists them.
var Policies = []string{PolicyEnforce, PolicyReport, PolicyIgnore}

// A Group is a PowerDNS server group (ADR-0007): the NetBox views whose zones
// it serves, its drift policies, and its primary, the one server nbpdns talks
// to.
type Group struct {
	// Name identifies the group, in --group and in config show.
	Name string
	// Views are the NetBox views whose zones the group serves.
	Views []string
	// DriftPolicy is the policy of the group's zones that ZonePolicies
	// doesn't name.
	DriftPolicy string
	// ZonePolicies maps absolute zone names, such as example.com., to their
	// policies.
	ZonePolicies map[string]string
	Primary      Primary
}

// Policy returns the drift policy of zone, an absolute name, in g.
func (g Group) Policy(zone string) string {
	if p, ok := g.ZonePolicies[zone]; ok {
		return p
	}
	return g.DriftPolicy
}

// checkPolicy accepts the name of a drift policy.
func checkPolicy(s string) error {
	if !slices.Contains(Policies, s) {
		return fmt.Errorf("%q isn't a drift policy; use %s", s, strings.Join(Policies, ", "))
	}
	return nil
}

// zoneKey returns a zone name, as dns.ZoneName checks it, as the absolute
// name the drift report compares.
func zoneKey(name string) (string, error) {
	n, err := dns.ZoneName(name)
	if err != nil {
		return "", err
	}
	return n + ".", nil
}

// Primary says how to reach a group's primary, through the PowerDNS API.
type Primary struct {
	URL    string
	APIKey Secret
	// APIKeyFile is the file the key was read from, if it came from one.
	APIKeyFile string
	ServerID   string
	// CAFile, CertFile and KeyFile are PEM files: CA certificates to trust
	// as well as the system's, and a client certificate with its key.
	CAFile, CertFile, KeyFile string
}

// A Field is one field of a server group's entry in the config file. The
// fields drive the entries' decoding, their validation, config show, and
// the generated reference.
type Field struct {
	// Name is the field's path within the entry, such as primary.url.
	Name string
	// Type describes its values, in Markdown, for the reference.
	Type string
	// Default is its value when the entry leaves it out, if it has one.
	Default string
	// Required marks a field every entry must set.
	Required bool
	// Secret marks a field whose value is redacted everywhere.
	Secret bool
	// Summary describes it, in Markdown, for the reference.
	Summary string

	parse func(g *Group, raw any) error
	show  func(g *Group) string
}

// groupName is what a group's name may be: lowercase letters, digits and
// hyphens, starting and ending with a letter or digit.
var groupName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// GroupFields returns the fields of a server group's entry, in the order
// the reference lists them.
func GroupFields() []Field {
	str := func(dst func(*Group) *string, check func(string) error) func(*Group, any) error {
		return func(g *Group, raw any) error {
			s, err := toString(raw)
			if err == nil && check != nil {
				err = check(s)
			}
			if err == nil {
				*dst(g) = s
			}
			return err
		}
	}
	nonEmpty := func(s string) error {
		if strings.TrimSpace(s) == "" {
			return errors.New("it's empty")
		}
		return nil
	}
	path := func(dst func(*Group) *string) Field {
		return Field{Type: "path", parse: str(dst, nonEmpty), show: func(g *Group) string { return *dst(g) }}
	}
	fields := []Field{
		{
			Name: "name", Type: "string: lowercase letters, digits and `-`", Required: true,
			Summary: "The group's name, unique among the groups. Commands take it as `--group`.",
			parse: str(func(g *Group) *string { return &g.Name }, func(s string) error {
				if !groupName.MatchString(s) {
					return fmt.Errorf("%q isn't lowercase letters, digits and hyphens, starting and ending with a letter or digit", s)
				}
				return nil
			}),
			show: func(g *Group) string { return g.Name },
		},
		{
			Name: "views", Type: "list of strings", Required: true,
			Summary: "The NetBox views whose zones the group serves. A view may be served by more than one group.",
			parse: func(g *Group, raw any) error {
				list, ok := raw.([]any)
				if !ok {
					return fmt.Errorf("want a list of views, not %s", describe(raw))
				}
				for _, item := range list {
					s, err := toString(item)
					if err != nil {
						return err
					}
					if s == "" {
						return errors.New("a view's name is empty")
					}
					if slices.Contains(g.Views, s) {
						return fmt.Errorf("%s is listed twice", s)
					}
					g.Views = append(g.Views, s)
				}
				if len(g.Views) == 0 {
					return errors.New("name at least one view")
				}
				return nil
			},
			show: func(g *Group) string { return strings.Join(g.Views, ",") },
		},
		{
			Name: "drift_policy", Type: "`enforce`, `report` or `ignore`", Default: PolicyReport,
			Summary: "The drift policy of the group's zones that `zone_policies` doesn't name. `ignore` doesn't compare a zone; " +
				"`report` and `enforce` report its drift, and from M14, `enforce` also corrects it.",
			parse: str(func(g *Group) *string { return &g.DriftPolicy }, checkPolicy),
			show:  func(g *Group) string { return g.DriftPolicy },
		},
		{
			Name: "zone_policies", Type: "mapping of zone names to drift policies",
			Summary: "Drift policies for single zones, which override `drift_policy`, such as `{legacy.example.com: ignore}`. " +
				"A name the group doesn't serve is reported as a warning.",
			parse: func(g *Group, raw any) error {
				m, ok := raw.(map[string]any)
				if !ok {
					return fmt.Errorf("want a mapping of zone names to policies, not %s", describe(raw))
				}
				g.ZonePolicies = map[string]string{}
				for _, name := range slices.Sorted(maps.Keys(m)) {
					zone, err := zoneKey(name)
					if err != nil {
						return err
					}
					if _, dup := g.ZonePolicies[zone]; dup {
						return fmt.Errorf("zone %s is listed twice", zone)
					}
					p, err := toString(m[name])
					if err == nil {
						err = checkPolicy(p)
					}
					if err != nil {
						return fmt.Errorf("zone %s: %w", zone, err)
					}
					g.ZonePolicies[zone] = p
				}
				return nil
			},
			show: func(g *Group) string {
				var parts []string
				for _, zone := range slices.Sorted(maps.Keys(g.ZonePolicies)) {
					parts = append(parts, strings.TrimSuffix(zone, ".")+"="+g.ZonePolicies[zone])
				}
				return strings.Join(parts, ",")
			},
		},
		{
			Name: "primary.url", Type: "an `http` or `https` URL", Required: true,
			Summary: "The primary's PowerDNS API: its web server, or a TLS proxy in front of it, such as `https://pdns-a.example.com:8443`.",
			parse:   str(func(g *Group) *string { return &g.Primary.URL }, checkURL),
			show:    func(g *Group) string { return g.Primary.URL },
		},
		{
			Name: "primary.api_key", Type: "string, secret", Secret: true,
			Summary: "The PowerDNS API key, sent as `X-API-Key`. Set this or `primary.api_key_file`.",
			parse: func(g *Group, raw any) error {
				s, err := secretString(raw)
				if err == nil {
					err = nonEmpty(s)
				}
				if err == nil {
					g.Primary.APIKey = NewSecret(s)
				}
				return err
			},
			show: func(g *Group) string { return g.Primary.APIKey.String() },
		},
		{
			Name: "primary.api_key_file", Type: "path",
			Summary: "A file that holds the API key, without a final newline if it has one. Set this or `primary.api_key`.",
			parse:   str(func(g *Group) *string { return &g.Primary.APIKeyFile }, nonEmpty),
			show:    func(g *Group) string { return g.Primary.APIKeyFile },
		},
		{
			Name: "primary.server_id", Type: "string", Default: "localhost",
			Summary: "The server's ID, which the PowerDNS API puts in its paths. PowerDNS calls its own server `localhost`.",
			parse: str(func(g *Group) *string { return &g.Primary.ServerID }, func(s string) error {
				if s == "" || strings.ContainsAny(s, "/?#% \t") {
					return fmt.Errorf("%q isn't a server ID", s)
				}
				return nil
			}),
			show: func(g *Group) string { return g.Primary.ServerID },
		},
	}
	ca, cert, key := path(func(g *Group) *string { return &g.Primary.CAFile }),
		path(func(g *Group) *string { return &g.Primary.CertFile }),
		path(func(g *Group) *string { return &g.Primary.KeyFile })
	ca.Name, ca.Summary = "primary.ca_file", "A PEM file of CA certificates to trust for the primary, as well as the system's."
	cert.Name, cert.Summary = "primary.cert_file", "A PEM client certificate to present, for a TLS proxy that requires one. Set it with `primary.key_file`."
	key.Name, key.Summary = "primary.key_file", "The client certificate's PEM private key."
	return append(fields, ca, cert, key)
}

// loadGroups decodes and checks the server groups in the config file at
// path, and returns them with their settings.
func loadGroups(v *viper.Viper, path string) ([]Group, []Setting, []error) {
	if !v.InConfig(GroupsKey) {
		return nil, []Setting{{Key: GroupsKey, Source: Source{Kind: "default"}}}, nil
	}
	src := Source{Kind: "file", Name: path}
	list, ok := v.Get(GroupsKey).([]any)
	if !ok {
		return nil, nil, []error{fmt.Errorf("%s (from %s): want a list of groups, not %s", GroupsKey, src, describe(v.Get(GroupsKey)))}
	}
	var (
		groups   []Group
		settings []Setting
		errs     []error
	)
	collisions := zonePolicyCollisions(path)
	for i, raw := range list {
		g, set, gerrs := decodeGroup(raw)
		if err := collisions[i]; err != nil {
			gerrs = append(gerrs, err)
		}
		where := fmt.Sprintf("%s[%d]", GroupsKey, i)
		if entry, ok := raw.(map[string]any); ok {
			if name, ok := entry["name"].(string); ok && name != "" {
				where += " (" + name + ")"
			}
		}
		if g.Name != "" && slices.ContainsFunc(groups, func(o Group) bool { return o.Name == g.Name }) {
			gerrs = append(gerrs, fmt.Errorf("another group is named %s", g.Name))
		}
		for _, err := range gerrs {
			errs = append(errs, fmt.Errorf("%s (from %s): %w", where, src, err))
		}
		if len(gerrs) > 0 {
			continue
		}
		groups = append(groups, g)
		for _, f := range GroupFields() {
			s := Setting{Key: GroupsKey + "." + g.Name + "." + f.Name, Value: f.show(&g), Source: Source{Kind: "default"}}
			if set[f.Name] {
				s.Source = src
			}
			settings = append(settings, s)
		}
	}
	return groups, settings, errs
}

// zonePolicyCollisions returns an error for each group, by its index, in the
// config file at path whose zone_policies has two keys that differ only in
// case. Viper lowercases map keys, so decodeGroup sees such keys as one, with
// either one's policy; only the file shows both.
func zonePolicyCollisions(path string) map[int]error {
	b, err := os.ReadFile(path) //nolint:gosec // The config file the user named, which Viper has just read.
	if err != nil {
		return nil // Viper reports it.
	}
	var doc any
	if yaml.Unmarshal(b, &doc) != nil {
		return nil // Viper reports it.
	}
	list, _ := lookup(doc, "powerdns", "groups").([]any)
	out := map[int]error{}
	for i, g := range list {
		policies, _ := lookup(g, "zone_policies").(map[string]any)
		seen := map[string]string{}
		for _, k := range slices.Sorted(maps.Keys(policies)) {
			if first, ok := seen[strings.ToLower(k)]; ok {
				out[i] = fmt.Errorf("zone_policies: %s and %s are one zone, listed twice", first, k)
				break
			}
			seen[strings.ToLower(k)] = k
		}
	}
	return out
}

// lookup follows keys down through nested mappings, matching each one
// without regard to case, as Viper does, and returns the value found, or
// nil.
func lookup(v any, keys ...string) any {
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = nil
		for _, mk := range slices.Sorted(maps.Keys(m)) {
			if strings.EqualFold(mk, k) {
				v = m[mk]
				break
			}
		}
	}
	return v
}

// decodeGroup decodes one entry of the groups list, and returns the group,
// the fields the entry set, and every problem with it.
func decodeGroup(raw any) (Group, map[string]bool, []error) {
	var (
		g      = Group{DriftPolicy: PolicyReport, Primary: Primary{ServerID: "localhost"}}
		errs   []error
		values = map[string]any{}
	)
	entry, ok := raw.(map[string]any)
	if !ok {
		return g, nil, []error{fmt.Errorf("want a mapping of a group's fields, not %s", describe(raw))}
	}
	for k, v := range entry {
		if k != "primary" {
			values[k] = v
			continue
		}
		primary, ok := v.(map[string]any)
		if !ok {
			errs = append(errs, fmt.Errorf("primary: want a mapping, not %s", describe(v)))
			continue
		}
		for k2, v2 := range primary {
			values["primary."+k2] = v2
		}
	}
	fields := GroupFields()
	set := map[string]bool{}
	for _, name := range slices.Sorted(maps.Keys(values)) {
		i := slices.IndexFunc(fields, func(f Field) bool { return f.Name == name })
		if i < 0 {
			errs = append(errs, fmt.Errorf("unknown key %s", name))
			continue
		}
		if err := fields[i].parse(&g, values[name]); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		set[name] = true
	}
	for _, f := range fields {
		if _, given := values[f.Name]; f.Required && !given {
			errs = append(errs, fmt.Errorf("%s isn't set", f.Name))
		}
	}
	switch {
	case set["primary.api_key"] && set["primary.api_key_file"]:
		errs = append(errs, errors.New("both primary.api_key and primary.api_key_file are set; set one"))
	case set["primary.api_key_file"]:
		s, err := readSecretFile(g.Primary.APIKeyFile)
		if err != nil {
			errs = append(errs, fmt.Errorf("primary.api_key_file: %w", err))
		}
		g.Primary.APIKey = NewSecret(s)
		set["primary.api_key"] = true
	case values["primary.api_key"] == nil:
		errs = append(errs, errors.New("neither primary.api_key nor primary.api_key_file is set; set one"))
	}
	if set["primary.cert_file"] != set["primary.key_file"] {
		errs = append(errs, errors.New("primary.cert_file and primary.key_file go together; set both, or neither"))
	}
	return g, set, errs
}
