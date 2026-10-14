package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/secrets"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

func newGroupsCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "groups",
		Short: "Show and change the PowerDNS server groups that nbpdns serve compares",
		Long: "Show and change the server groups that the database at database.url stores, and\n" +
			"that nbpdns serve compares (ADR-0040). Each group is managed by the config file,\n" +
			"whose groups serve mirrors into the database at each start, or by these\n" +
			"commands, which change only their own groups. A running nbpdns serve applies\n" +
			"their changes within seconds. Each change is an audit event, with the operating\n" +
			"system's user as its actor.",
		Args: usageArgs(cobra.NoArgs),
		RunE: showHelp,
	}
	cmd.AddCommand(newGroupsListCmd(a), newGroupsAddCmd(a), newGroupsSetCmd(a), newGroupsRemoveCmd(a), newGroupsRenameCmd(a))
	return cmd
}

// storedGroupOut is a group in the output of `nbpdns groups list`.
type storedGroupOut struct {
	Name         string            `json:"name"`
	ManagedBy    string            `json:"managed_by"`
	Views        []string          `json:"views"`
	DriftPolicy  string            `json:"drift_policy"`
	ZonePolicies map[string]string `json:"zone_policies"`
	Primary      primaryOut        `json:"primary"`
	CreatedAt    time.Time         `json:"created_at"`
	CreatedBy    string            `json:"created_by"`
	UpdatedAt    time.Time         `json:"updated_at"`
	UpdatedBy    string            `json:"updated_by"`
}

// primaryOut is a group's primary, in the output of `nbpdns groups list`.
type primaryOut struct {
	URL      string `json:"url"`
	ServerID string `json:"server_id"`
	// APIKey is [redacted] for a CLI group, whose key the database stores.
	APIKey     string `json:"api_key,omitempty"`
	APIKeyFile string `json:"api_key_file,omitempty"`
	CAFile     string `json:"ca_file,omitempty"`
	CertFile   string `json:"cert_file,omitempty"`
	KeyFile    string `json:"key_file,omitempty"`
}

func storedGroupOf(g store.StoredGroup) storedGroupOut {
	p := g.Group.Primary
	out := storedGroupOut{
		Name: g.Group.Name, ManagedBy: g.Group.ManagedBy, Views: g.Group.Views, DriftPolicy: g.Group.DriftPolicy,
		ZonePolicies: g.Group.ZonePolicies,
		Primary: primaryOut{URL: p.URL, ServerID: p.ServerID, APIKeyFile: p.APIKeyFile, CAFile: p.CAFile, CertFile: p.CertFile,
			KeyFile: p.KeyFile},
		CreatedAt: g.CreatedAt, CreatedBy: g.CreatedBy, UpdatedAt: g.UpdatedAt, UpdatedBy: g.UpdatedBy,
	}
	if g.APIKey != "" {
		out.Primary.APIKey = config.Redacted
	}
	if out.ZonePolicies == nil {
		out.ZonePolicies = map[string]string{}
	}
	return out
}

func newGroupsListCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the server groups, with what manages each, and who changed it last",
		Long: "List the server groups that the database stores: the config file's, as nbpdns\n" +
			"serve last mirrored them at its start, and those that nbpdns groups added. The\n" +
			"JSON output has each one's fields, with a stored API key as [redacted].",
		Args: usageArgs(cobra.NoArgs),
	}
	output := addOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return a.run(cmd, func(ctx context.Context, s *session) error {
			st, err := s.currentStore(ctx)
			if err != nil {
				return err
			}
			defer st.Close()
			groups, _, err := st.Groups(ctx)
			if err != nil {
				return err
			}
			out := make([]storedGroupOut, len(groups))
			for i, g := range groups {
				out[i] = storedGroupOf(g)
			}
			if *output == outputJSON {
				return writeJSON(a.stdout, out)
			}
			if len(out) == 0 {
				_, err := fmt.Fprintln(a.stdout, "The database stores no server groups.")
				return err
			}
			rows := make([][]string, len(out))
			for i, g := range out {
				rows[i] = []string{g.Name, g.ManagedBy, g.Primary.URL, strings.Join(g.Views, ","), g.DriftPolicy,
					g.UpdatedAt.UTC().Format(time.RFC3339), g.UpdatedBy}
			}
			return writeTable(a.stdout, []string{"NAME", "MANAGED BY", "PRIMARY", "VIEWS", "DRIFT POLICY", "UPDATED", "BY"}, rows)
		})
	}
	return cmd
}

// groupFlags are the flags of `nbpdns groups add` and `nbpdns groups set`:
// one for each of a group's fields, and the key's file.
type groupFlags struct {
	url, driftPolicy, zonePolicies, serverID, caFile, certFile, keyFile, apiKeyFile string
	views                                                                           []string
	noCheck                                                                         bool
	reason                                                                          *string
}

// fieldFlags names the flags that set a group's fields, by the fields'
// names in the config file.
var fieldFlags = map[string]string{
	"primary.url": "url", "views": "views", "drift_policy": "drift-policy", "zone_policies": "zone-policies",
	"primary.server_id": "server-id", "primary.ca_file": "ca-file", "primary.cert_file": "cert-file", "primary.key_file": "key-file",
}

// addGroupFlags adds a group's flags to cmd.
func addGroupFlags(cmd *cobra.Command) *groupFlags {
	f := &groupFlags{}
	fs := cmd.Flags()
	for _, fl := range []struct {
		p               *string
		name, usage, md string
	}{
		{&f.url, "url", "The primary's PowerDNS API, such as https://pdns-a.example.com:8443.",
			"The primary's PowerDNS API, such as `https://pdns-a.example.com:8443`."},
		{&f.driftPolicy, "drift-policy", "The drift policy of the group's zones: enforce, report or ignore. By default, report.",
			"The drift policy of the group's zones: `enforce`, `report` or `ignore`. By default, `report`."},
		{&f.zonePolicies, "zone-policies", "Drift policies for single zones, as zone=policy separated by commas, such as legacy.example.com=ignore; empty for none.",
			"Drift policies for single zones, as `zone=policy` separated by commas, such as `legacy.example.com=ignore`; empty for none."},
		{&f.serverID, "server-id", "The server's ID in the PowerDNS API's paths. By default, localhost.",
			"The server's ID in the PowerDNS API's paths. By default, `localhost`."},
		{&f.caFile, "ca-file", "A PEM file of CA certificates to trust for the primary, as well as the system's; empty for none.", ""},
		{&f.certFile, "cert-file", "A PEM client certificate to present, with --key-file; empty for none.",
			"A PEM client certificate to present, with `--key-file`; empty for none."},
		{&f.keyFile, "key-file", "The client certificate's PEM private key; empty for none.", ""},
		{&f.apiKeyFile, "api-key-file", "Read the PowerDNS API key from this file, or from standard input if it's -. Without it, the key comes from " +
			config.GroupAPIKeyEnv + ", if it's set.",
			"Read the PowerDNS API key from this file, or from standard input if it's `-`. Without it, the key comes from `" +
				config.GroupAPIKeyEnv + "`, if it's set."},
	} {
		fs.StringVar(fl.p, fl.name, "", fl.usage)
		if fl.md != "" {
			_ = fs.SetAnnotation(fl.name, config.MarkdownUsage, []string{fl.md})
		}
	}
	fs.StringSliceVar(&f.views, "views", nil, "The NetBox views whose zones the group serves, separated by commas.")
	fs.BoolVar(&f.noCheck, "no-check", false, "Don't check that the primary answers, such as for a server that isn't up yet.")
	f.reason = addReasonFlag(cmd)
	return f
}

// fieldsGiven reports whether cmd's flags set any of a group's fields.
func fieldsGiven(cmd *cobra.Command) bool {
	for _, name := range fieldFlags {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

// apply sets the fields of entry, a group as the config file would hold it,
// that cmd's flags give. An empty value removes the field, so that it takes
// its default.
func (f *groupFlags) apply(cmd *cobra.Command, entry map[string]any) error {
	fs := cmd.Flags()
	primary, _ := entry["primary"].(map[string]any)
	if primary == nil {
		primary = map[string]any{}
		entry["primary"] = primary
	}
	set := func(flag string, dst map[string]any, key, value string) {
		switch {
		case !fs.Changed(flag):
		case value == "":
			delete(dst, key)
		default:
			dst[key] = value
		}
	}
	set("url", primary, "url", f.url)
	set("drift-policy", entry, "drift_policy", f.driftPolicy)
	set("server-id", primary, "server_id", f.serverID)
	set("ca-file", primary, "ca_file", f.caFile)
	set("cert-file", primary, "cert_file", f.certFile)
	set("key-file", primary, "key_file", f.keyFile)
	if fs.Changed("views") {
		views := make([]any, len(f.views))
		for i, v := range f.views {
			views[i] = v
		}
		entry["views"] = views
	}
	if fs.Changed("zone-policies") {
		delete(entry, "zone_policies")
		if f.zonePolicies != "" {
			policies := map[string]any{}
			for item := range strings.SplitSeq(f.zonePolicies, ",") {
				zone, policy, ok := strings.Cut(item, "=")
				if !ok {
					return fmt.Errorf("--zone-policies: %q isn't zone=policy", item)
				}
				policies[strings.TrimSpace(zone)] = strings.TrimSpace(policy)
			}
			entry["zone_policies"] = policies
		}
	}
	return nil
}

// apiKey returns the API key that add and set store, and whether one was
// given: from the file that --api-key-file names, or standard input, or,
// without it, from NBPDNS_GROUP_API_KEY, if it's set.
func (f *groupFlags) apiKey(cmd *cobra.Command) (string, bool, error) {
	if f.apiKeyFile != "" {
		v, err := readValue(cmd, f.apiKeyFile)
		if err != nil {
			return "", false, fmt.Errorf("--api-key-file: %w", err)
		}
		return v, true, nil
	}
	if v := strings.TrimRight(os.Getenv(config.GroupAPIKeyEnv), "\r\n"); v != "" {
		return v, true, nil
	}
	return "", false, nil
}

// decodeGroup decodes and checks entry, a group named name, as the config
// file's are.
func decodeGroup(name string, entry map[string]any) (config.Group, error) {
	g, err := config.DecodeGroup(entry)
	if err != nil {
		return g, usageError{fmt.Errorf("server group %s: %w", name, err)}
	}
	g.ManagedBy = config.ManagedByCLI
	return g, nil
}

// groupMasterKey returns database.master_key, which seals a CLI group's API
// key, and fails if it isn't set.
func (s *session) groupMasterKey() (secrets.MasterKey, error) {
	mk, err := s.masterKey()
	if err == nil && !mk.IsSet() {
		err = errors.New("a CLI group's API key is stored sealed with database.master_key, which isn't set; make one with openssl rand -base64 32")
	}
	return mk, err
}

// sealKey returns a Seal of key, with mk.
func sealKey(mk secrets.MasterKey, key string) store.Seal {
	return func(place string) (string, error) { return secrets.Seal(mk, place, key) }
}

// checkFileGroups fails if the config file declares a group named name, which
// would stop serve at its next start (ADR-0040).
func (s *session) checkFileGroups(name string) error {
	if slices.ContainsFunc(s.cfg.PowerDNS.Groups, func(g config.Group) bool { return g.Name == name }) {
		return fmt.Errorf("the config file, %s, declares a server group named %s, so nbpdns groups can't add one", s.loader.ConfigFile(), name)
	}
	return nil
}

// checkPrimary checks that g's primary answers, as `nbpdns powerdns check`
// does, unless noCheck is set, and says how, as store.Change.Check does. If a
// check fails, it writes the checks to w, and fails, so that nothing
// changes.
func checkPrimary(ctx context.Context, s *session, w io.Writer, g config.Group, noCheck bool) (string, error) {
	if noCheck {
		return store.CheckSkipped, nil
	}
	gc := checkGroup(ctx, s, g)
	if gc.OK {
		return store.CheckPassed, nil
	}
	if err := writePowerDNSCheckTable(w, powerDNSReport{Groups: []groupCheck{gc}}); err != nil {
		return "", err
	}
	return "", fmt.Errorf("server group %s's primary failed a check, so nothing changed; fix it, or pass --no-check", g.Name)
}

// reaches reports whether a and b differ in how nbpdns reaches their
// primary, but for the API key.
func reaches(a, b config.Group) bool {
	p, q := a.Primary, b.Primary
	return p.URL != q.URL || p.ServerID != q.ServerID || p.CAFile != q.CAFile || p.CertFile != q.CertFile || p.KeyFile != q.KeyFile
}

func newGroupsAddCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add NAME",
		Short: "Add a server group, which nbpdns serve compares within seconds",
		Long: "Add a server group, managed by the command line, after checking its fields as\n" +
			"the config file's are, and checking that its primary's API answers and accepts\n" +
			"the key, unless --no-check is given. The primary's URL and the group's views\n" +
			"are required.\n\n" +
			"The PowerDNS API key is read from the file that --api-key-file names, or from\n" +
			"standard input with --api-key-file -, or, without it, from " + config.GroupAPIKeyEnv + ";\n" +
			"never from the command line, where other users can see it. It's sealed with\n" +
			"database.master_key before it's stored. The change is an audit event, with\n" +
			"--reason, if given. A name that the config file declares is refused, since it\n" +
			"would stop nbpdns serve.",
		Args: usageArgs(cobra.ExactArgs(1)),
	}
	f := addGroupFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := config.CheckGroupName(name); err != nil {
			return usageError{fmt.Errorf("the group's name: %w", err)}
		}
		for _, required := range []string{"url", "views"} {
			if !cmd.Flags().Changed(required) {
				return usageError{fmt.Errorf("--%s is required", required)}
			}
		}
		entry := map[string]any{"name": name}
		if err := f.apply(cmd, entry); err != nil {
			return usageError{err}
		}
		key, ok, err := f.apiKey(cmd)
		if err != nil {
			return usageError{err}
		}
		if !ok {
			return usageError{fmt.Errorf("give the group's API key with --api-key-file, or in %s", config.GroupAPIKeyEnv)}
		}
		entry["primary"].(map[string]any)["api_key"] = key
		g, err := decodeGroup(name, entry)
		if err != nil {
			return err
		}
		return a.run(cmd, func(ctx context.Context, s *session) error {
			if err := s.checkFileGroups(name); err != nil {
				return err
			}
			mk, err := s.groupMasterKey()
			if err != nil {
				return err
			}
			st, err := s.currentStore(ctx)
			if err != nil {
				return err
			}
			defer st.Close()
			// A name that's taken fails before the primary is checked.
			var ne *store.NoGroupError
			switch o, err := st.Group(ctx, name); {
			case err == nil:
				return fmt.Errorf("server group %s exists, managed by %s", name, o.Group.ManagedBy)
			case !errors.As(err, &ne):
				return err
			}
			check, err := checkPrimary(ctx, s, a.stdout, g, f.noCheck)
			if err != nil {
				return err
			}
			if _, err := st.AddGroup(ctx, g, sealKey(mk, key), store.Change{Actor: audit.OSUser(), Reason: *f.reason, Check: check}); err != nil {
				return err
			}
			fmt.Fprintf(a.stdout, "Added server group %s, its API key sealed with the master key.\n", name)
			fmt.Fprintf(a.stdout, "A running nbpdns serve compares it within %s.\n", settingsPoll)
			return nil
		})
	}
	return cmd
}

func newGroupsSetCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set NAME",
		Short: "Change a server group's fields, which nbpdns serve applies within seconds",
		Long: "Change the fields of a server group that nbpdns groups added: each flag given\n" +
			"replaces its field, and an empty value returns it to its default, or removes\n" +
			"it. The group is checked as the config file's are. If the change touches how\n" +
			"nbpdns reaches the primary, such as its URL or API key, the primary's API is\n" +
			"checked too, as nbpdns groups add checks it, unless --no-check is given.\n\n" +
			"A new API key is read as nbpdns groups add reads it: from --api-key-file, or\n" +
			config.GroupAPIKeyEnv + ". Opening the stored key needs database.master_key. The\n" +
			"change is an audit event, with --reason, if given.",
		Args: usageArgs(cobra.ExactArgs(1)),
	}
	f := addGroupFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		name := args[0]
		key, keyGiven, err := f.apiKey(cmd)
		if err != nil {
			return usageError{err}
		}
		if !keyGiven && !fieldsGiven(cmd) {
			return usageError{fmt.Errorf("give a field to change, such as --url, or a new API key with --api-key-file or in %s", config.GroupAPIKeyEnv)}
		}
		return a.run(cmd, func(ctx context.Context, s *session) error {
			mk, err := s.groupMasterKey()
			if err != nil {
				return err
			}
			st, err := s.currentStore(ctx)
			if err != nil {
				return err
			}
			defer st.Close()
			cur, err := st.Group(ctx, name)
			if err != nil {
				return err
			}
			if cur.Group.ManagedBy == config.ManagedByFile {
				return &store.FileGroupError{Name: name, File: audit.ParseActor(cur.UpdatedBy).ID}
			}
			old, err := secrets.Open(mk, store.GroupPlace(cur.ID), cur.APIKey)
			if err != nil {
				return fmt.Errorf("server group %s's API key doesn't open: %w", name, err)
			}
			g0 := cur.Group
			g0.Primary.APIKey = config.NewSecret(old)
			entry := config.GroupEntry(g0)
			if err := f.apply(cmd, entry); err != nil {
				return usageError{err}
			}
			if keyGiven {
				entry["primary"].(map[string]any)["api_key"] = key
			}
			g, err := decodeGroup(name, entry)
			if err != nil {
				return err
			}
			var seal store.Seal
			keyChanged := keyGiven && key != old
			if keyChanged {
				seal = sealKey(mk, key)
			}
			check := ""
			if keyChanged || reaches(cur.Group, g) {
				if check, err = checkPrimary(ctx, s, a.stdout, g, f.noCheck); err != nil {
					return err
				}
			}
			fields, err := st.SetGroup(ctx, cur, g, seal, store.Change{Actor: audit.OSUser(), Reason: *f.reason, Check: check})
			if err != nil {
				return err
			}
			if len(fields) == 0 {
				fmt.Fprintf(a.stdout, "Server group %s already has those values.\n", name)
				return nil
			}
			fmt.Fprintf(a.stdout, "Changed %s of server group %s.\n", strings.Join(fields, ", "), name)
			fmt.Fprintf(a.stdout, "A running nbpdns serve applies it within %s.\n", settingsPoll)
			return nil
		})
	}
	return cmd
}

func newGroupsRemoveCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove NAME",
		Short: "Remove a server group, which nbpdns serve stops comparing within seconds",
		Long: "Remove a server group that nbpdns groups added. A running nbpdns serve drops\n" +
			"its state and metrics within seconds. Its drift history is kept until\n" +
			"drift.history_retention_days prunes it, and nbpdns drift history still reads it.\n" +
			"The change is an audit event, with --reason, if given.",
		Args: usageArgs(cobra.ExactArgs(1)),
	}
	reason := addReasonFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return a.run(cmd, func(ctx context.Context, s *session) error {
			st, err := s.currentStore(ctx)
			if err != nil {
				return err
			}
			defer st.Close()
			if err := st.RemoveGroup(ctx, args[0], store.Change{Actor: audit.OSUser(), Reason: *reason}); err != nil {
				return err
			}
			fmt.Fprintf(a.stdout, "Removed server group %s. Its drift history is kept until drift.history_retention_days prunes it.\n", args[0])
			fmt.Fprintf(a.stdout, "A running nbpdns serve stops comparing it within %s.\n", settingsPoll)
			return nil
		})
	}
	return cmd
}

func newGroupsRenameCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename NAME NEW-NAME",
		Short: "Rename a server group, with its drift history",
		Long: "Rename a server group that nbpdns groups added. Its drift history and its last\n" +
			"report carry on under its new name (ADR-0045). A running nbpdns serve applies it\n" +
			"within seconds, and moves the group's metrics to the new name. The audit trail\n" +
			"keeps the events before the rename under the old name, and the event of the\n" +
			"rename links the two. The change is an audit event, with --reason, if given.\n\n" +
			"A new name that another group has, that the config file declares, or that a\n" +
			"removed group's drift history still uses, is refused.",
		Args: usageArgs(cobra.ExactArgs(2)),
	}
	reason := addReasonFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		old, name := args[0], args[1]
		if err := config.CheckGroupName(name); err != nil {
			return usageError{fmt.Errorf("the new name: %w", err)}
		}
		return a.run(cmd, func(ctx context.Context, s *session) error {
			if err := s.checkFileGroups(name); err != nil {
				return err
			}
			st, err := s.currentStore(ctx)
			if err != nil {
				return err
			}
			defer st.Close()
			if err := st.RenameGroup(ctx, old, name, store.Change{Actor: audit.OSUser(), Reason: *reason}); err != nil {
				return err
			}
			fmt.Fprintf(a.stdout, "Renamed server group %s to %s, with its drift history.\n", old, name)
			fmt.Fprintf(a.stdout, "A running nbpdns serve applies it within %s.\n", settingsPoll)
			return nil
		})
	}
	return cmd
}
