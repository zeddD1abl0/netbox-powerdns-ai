package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/secrets"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

// settingsPoll is how often serve checks whether the settings changed.
const settingsPoll = 5 * time.Second

func newSettingsCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Show and change the runtime settings that nbpdns serve applies while it runs",
		Long: "Show and change the runtime settings, such as drift.interval, that the database\n" +
			"at database.url stores, and that nbpdns serve applies within seconds, without a\n" +
			"restart (ADR-0040). Each change is an audit event, with the operating system's\n" +
			"user as its actor. A runtime key that a flag, the environment, or the config\n" +
			"file sets is pinned: that value wins, and the stored one isn't used until it's\n" +
			"unset there. Bootstrap keys, such as database.url, aren't settings.",
		Args: usageArgs(cobra.NoArgs),
		RunE: showHelp,
	}
	cmd.AddCommand(newSettingsListCmd(a), newSettingsGetCmd(a), newSettingsSetCmd(a), newSettingsUnsetCmd(a))
	return cmd
}

func newSettingsListCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List each runtime setting's value, where it comes from, and what's stored",
		Long: "List each runtime key's effective value, and where it comes from, with the\n" +
			"value that the database stores for it, if any. A stored value that a flag, the\n" +
			"environment or the config file overrides is marked pinned.",
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
			if _, err := s.applyStored(ctx, st); err != nil {
				return err
			}
			settings := runtimeSettings(s.settings)
			if *output == outputJSON {
				return writeJSON(a.stdout, settings)
			}
			rows := make([][]string, len(settings))
			for i, st := range settings {
				stored := "-"
				if st.Stored != nil {
					stored = *st.Stored
					if st.Source.Kind != config.SourceDatabase {
						stored += " (pinned)"
					}
				}
				rows[i] = []string{st.Key, st.Value, st.Source.String(), stored}
			}
			return writeTable(a.stdout, []string{"KEY", "VALUE", "SOURCE", "STORED"}, rows)
		})
	}
	return cmd
}

func newSettingsGetCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get KEY",
		Short: "Print a runtime setting's effective value",
		Long: "Print a runtime key's effective value, from whichever source sets it, alone\n" +
			"on a line. The JSON output adds its source, and its stored value.",
		Args: usageArgs(cobra.ExactArgs(1)),
	}
	output := addOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return a.run(cmd, func(ctx context.Context, s *session) error {
			st, err := s.currentStore(ctx)
			if err != nil {
				return err
			}
			defer st.Close()
			if _, err := s.applyStored(ctx, st); err != nil {
				return err
			}
			for _, setting := range runtimeSettings(s.settings) {
				if setting.Key != args[0] {
					continue
				}
				if *output == outputJSON {
					return writeJSON(a.stdout, setting)
				}
				_, err := fmt.Fprintln(a.stdout, setting.Value)
				return err
			}
			_, err = config.CheckSetting(args[0], "")
			return err
		})
	}
	return cmd
}

func newSettingsSetCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set KEY [VALUE]",
		Short: "Store a runtime setting's value, which nbpdns serve applies within seconds",
		Long: "Store a runtime key's value in the database, after checking it as the config\n" +
			"file's would be. A running nbpdns serve applies it within seconds, unless the\n" +
			"key is pinned. The change is an audit event, with --reason, if given.\n\n" +
			"A secret, such as netbox.token, is read from the file that --from-file names,\n" +
			"or from standard input with --from-file -, never from the command line, where\n" +
			"other users can see it. It's sealed with database.master_key before it's\n" +
			"stored, and its audit event shows it as [redacted].",
		Args: usageArgs(cobra.RangeArgs(1, 2)),
	}
	reason := addReasonFlag(cmd)
	from := cmd.Flags().String("from-file", "", "Read the value from this file, or from standard input if it's -.")
	_ = cmd.Flags().SetAnnotation("from-file", config.MarkdownUsage, []string{"Read the value from this file, or from standard input if it's `-`."})
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		key, arg := args[0], ""
		if len(args) > 1 {
			arg = args[len(args)-1]
		}
		value, err := settingValue(cmd, arg, *from)
		if err != nil {
			return usageError{err}
		}
		k, err := config.CheckSetting(key, value)
		if err != nil {
			return usageError{err}
		}
		if k.Secret && len(args) == 2 {
			return usageError{fmt.Errorf("%s is a secret: pass it with --from-file, not on the command line, where other users can see it", key)}
		}
		return a.run(cmd, func(ctx context.Context, s *session) error {
			c := store.SettingChange{Key: key, Value: value, Secret: k.Secret, Actor: audit.OSUser(), Reason: *reason}
			if k.Secret {
				mk, err := s.masterKey()
				if err != nil {
					return err
				}
				if !mk.IsSet() {
					return fmt.Errorf("%s is a secret, which the database stores sealed with database.master_key, which isn't set; "+
						"make one with openssl rand -base64 32", key)
				}
				if c.Value, err = secrets.Seal(mk, store.SettingPlace(key), value); err != nil {
					return err
				}
			}
			return s.changeSetting(ctx, a.stdout, c)
		})
	}
	return cmd
}

// settingValue returns the value that `nbpdns settings set` stores: arg, its
// second argument, if it has one, or what --from-file names, without a final
// newline.
func settingValue(cmd *cobra.Command, arg, from string) (string, error) {
	if arg != "" {
		if from != "" {
			return "", errors.New("give the value as an argument, or with --from-file, not both")
		}
		return arg, nil
	}
	if from == "" {
		return "", errors.New("give the value as an argument, or, for a secret, with --from-file")
	}
	return readValue(cmd, from)
}

// readValue returns what the file from holds, or standard input if from is
// -, without a final newline. It fails if that's empty.
func readValue(cmd *cobra.Command, from string) (string, error) {
	if from != "-" {
		return config.ReadSecretFile(from)
	}
	b, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 64<<10))
	if err != nil {
		return "", fmt.Errorf("reading the value: %w", err)
	}
	value := strings.TrimRight(string(b), "\r\n")
	if value == "" {
		return "", errors.New("the value is empty")
	}
	return value, nil
}

// masterKey returns database.master_key, or no key if it isn't set.
func (s *session) masterKey() (secrets.MasterKey, error) {
	if !s.cfg.Database.MasterKey.IsSet() {
		return secrets.MasterKey{}, nil
	}
	mk, err := secrets.ParseMasterKey(s.cfg.Database.MasterKey.Reveal())
	if err != nil {
		return mk, fmt.Errorf("database.master_key: %w", err)
	}
	return mk, nil
}

func newSettingsUnsetCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unset KEY",
		Short: "Remove a runtime setting's stored value",
		Long: "Remove a runtime key's stored value from the database, so that the key takes\n" +
			"its default, unless it's pinned. A running nbpdns serve applies it within\n" +
			"seconds. The change is an audit event, with --reason, if given.",
		Args: usageArgs(cobra.ExactArgs(1)),
	}
	reason := addReasonFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		key := args[0]
		k, err := config.CheckSetting(key, "")
		var be *config.BootstrapError
		if errors.As(err, &be) || k.Name == "" {
			return usageError{err}
		}
		return a.run(cmd, func(ctx context.Context, s *session) error {
			return s.changeSetting(ctx, a.stdout, store.SettingChange{Key: key, Unset: true, Secret: k.Secret, Actor: audit.OSUser(), Reason: *reason})
		})
	}
	return cmd
}

// addReasonFlag adds --reason to cmd.
func addReasonFlag(cmd *cobra.Command) *string {
	return cmd.Flags().String("reason", "", "Why, for the change's audit event.")
}

// changeSetting makes c, and says on out what it means for serve.
func (s *session) changeSetting(ctx context.Context, out io.Writer, c store.SettingChange) error {
	st, err := s.currentStore(ctx)
	if err != nil {
		return err
	}
	defer st.Close()
	changed, err := st.ChangeSetting(ctx, c)
	if err != nil {
		return err
	}
	// The settings from the flags, the environment and the file say whether
	// the key is pinned.
	var pin config.Source
	for _, setting := range s.settings {
		if setting.Key == c.Key {
			pin = setting.Source
		}
	}
	switch {
	case !changed && c.Unset:
		fmt.Fprintf(out, "The database stores no value for %s.\n", c.Key)
		return nil
	case !changed:
		fmt.Fprintf(out, "The database already stores %s for %s.\n", c.Value, c.Key)
		return nil
	case c.Unset:
		fmt.Fprintf(out, "Removed the stored value of %s.\n", c.Key)
	case c.Secret:
		fmt.Fprintf(out, "Stored %s for %s, sealed with the master key.\n", config.Redacted, c.Key)
	default:
		fmt.Fprintf(out, "Stored %s for %s.\n", c.Value, c.Key)
	}
	if pin.Kind != "default" {
		fmt.Fprintf(out, "%s pins %s, so nbpdns uses its value until it's unset there.\n", pin, c.Key)
		return nil
	}
	fmt.Fprintf(out, "A running nbpdns serve applies it within %s.\n", settingsPoll)
	return nil
}

// applyStored adds the settings that st stores to the session's
// configuration, as the source between the defaults and the config file,
// and returns their version.
func (s *session) applyStored(ctx context.Context, st *store.Store) (int64, error) {
	cfg, settings, version, err := s.storedConfig(ctx, st)
	if err != nil {
		return 0, err
	}
	s.cfg, s.settings = cfg, settings
	return version, s.level.UnmarshalText([]byte(cfg.Log.Level))
}

// storedConfig returns the configuration with the settings that st stores,
// and their version.
func (s *session) storedConfig(ctx context.Context, st *store.Store) (*config.Config, []config.Setting, int64, error) {
	stored, version, err := st.Settings(ctx)
	if err != nil {
		return nil, nil, 0, err
	}
	values := make(map[string]string, len(stored))
	var mk secrets.MasterKey
	for k, v := range stored {
		values[k] = v.Value
		if !v.Sealed {
			continue
		}
		if !mk.IsSet() {
			if mk, err = s.masterKey(); err != nil {
				return nil, nil, 0, err
			}
		}
		if values[k], err = secrets.Open(mk, store.SettingPlace(k), v.Value); err != nil {
			return nil, nil, 0, fmt.Errorf("the database stores a secret for %s, which doesn't open: %w", k, err)
		}
	}
	cfg, settings, err := s.loader.WithStored(values)
	if err != nil {
		return nil, nil, 0, err
	}
	return cfg, settings, version, nil
}

// storeProbeTimeout bounds how long applyStoredIfAny waits for PostgreSQL.
const storeProbeTimeout = 10 * time.Second

// applyStoredIfAny is applyStored, for a command that works without a
// database: if there's none, or it can't be read, the configuration stays
// as it is, and a warning says why. It waits for PostgreSQL for
// storeProbeTimeout at most.
func (s *session) applyStoredIfAny(ctx context.Context) {
	if path := s.cfg.Database.SQLitePath(); path == "" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, storeProbeTimeout)
		defer cancel()
	} else if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return
	}
	st, err := s.currentStore(ctx)
	if err == nil {
		defer st.Close()
		_, err = s.applyStored(ctx, st)
	}
	if err != nil {
		s.log.WarnContext(ctx, "couldn't read the settings that the database stores, so they're left out", "err", err)
	}
}

// runtimeSettings returns the settings of the runtime keys.
func runtimeSettings(settings []config.Setting) []config.Setting {
	runtime := map[string]bool{}
	for _, k := range config.Keys() {
		runtime[k.Name] = k.Runtime
	}
	var out []config.Setting
	for _, st := range settings {
		if runtime[st.Key] {
			out = append(out, st)
		}
	}
	return out
}
