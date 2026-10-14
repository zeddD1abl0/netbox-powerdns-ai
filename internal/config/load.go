package config

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// A Setting is one key's effective value and where it came from, as
// `nbpdns config show` prints it. A secret's value is redacted.
type Setting struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Source Source `json:"source"`
	// Stored is the value that the database stores for a runtime key,
	// redacted if it's a secret, or nil if it stores none. Unless Source is
	// the database, the stored value is pinned: kept, but not used.
	Stored *string `json:"stored,omitempty"`
}

// SourceDatabase is the Kind of a value that the database stores.
const SourceDatabase = "database"

// Source says where a setting's value came from.
type Source struct {
	// Kind is default, database, file, env or flag.
	Kind string `json:"kind"`
	// Name is the config file's path, the environment variable or the flag.
	// It's empty for a default.
	Name string `json:"name,omitempty"`
}

func (s Source) String() string {
	if s.Name == "" {
		return s.Kind
	}
	return s.Kind + " " + s.Name
}

// A Loader reads the configuration from its sources, in increasing order of
// precedence: the defaults, the database, for a runtime key, the config
// file, the environment and the flags. Add its flags to a command with
// AddFlags, and call Load once they're parsed, which reads every source but
// the database; then WithStored adds the database's values.
type Loader struct {
	flags      *pflag.FlagSet
	configPath string
	// base is each key's raw value and source, from every source but the
	// database, as the last Load read them, and groups its groups.
	base          map[string]resolved
	groups        []Group
	groupSettings []Setting
	// path is the config file that the last Load read, if any.
	path string
}

// ConfigFile returns the config file that the last Load read, or empty if it
// read none.
func (l *Loader) ConfigFile() string { return l.path }

// resolved is a key's raw value and where it came from.
type resolved struct {
	raw any
	src Source
}

// NewLoader returns a Loader.
func NewLoader() *Loader { return &Loader{} }

// AddFlags adds --config, and a flag for every key, to fs. A secret key also
// gets a flag that reads it from a file.
func (l *Loader) AddFlags(fs *pflag.FlagSet) {
	l.flags = fs
	addFlag := func(name, usage, markdown string) {
		_ = fs.SetAnnotation(name, MarkdownUsage, []string{markdown})
		fs.Lookup(name).Usage = usage
	}
	fs.StringVar(&l.configPath, "config", "", "")
	addFlag("config", "The YAML config file to read, instead of $"+ConfigEnv+".",
		"The YAML config file to read, instead of `$"+ConfigEnv+"`.")
	for _, k := range Keys() {
		fs.Var(&flagValue{typ: k.flagType, value: k.Default}, k.Flag(), "")
		addFlag(k.Flag(), k.usage(), k.Summary)
		if k.Secret {
			fs.Var(&flagValue{typ: "path"}, k.FileFlag(), "")
			addFlag(k.FileFlag(), "Read "+k.Name+" from this file.", "Read `"+k.Name+"` from this file.")
		}
	}
}

// Load reads every source and returns the configuration and each key's
// setting. The error joins every problem found, so they can all be fixed at
// once.
func (l *Loader) Load() (*Config, []Setting, error) {
	cfg := &Config{}
	ks := keys(cfg)
	v := viper.New()
	var errs []error

	for _, k := range ks {
		v.SetDefault(k.Name, k.Default)
		errs = append(errs, v.BindEnv(k.Name, k.Env()))
		if l.flags != nil {
			errs = append(errs, v.BindPFlag(k.Name, l.flags.Lookup(k.Flag())))
		}
		if k.Secret {
			v.SetDefault(k.FileName(), "")
			errs = append(errs, v.BindEnv(k.FileName(), k.FileEnv()))
			if l.flags != nil {
				errs = append(errs, v.BindPFlag(k.FileName(), l.flags.Lookup(k.FileFlag())))
			}
		}
	}

	path := l.configPath
	if path == "" {
		path = os.Getenv(ConfigEnv)
	}
	l.path = path
	if path != "" {
		v.SetConfigFile(path)
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err != nil {
			errs = append(errs, fmt.Errorf("config file %s: %w", path, err))
		}
	}

	errs = append(errs, unknownEnv(ks)...)
	errs = append(errs, unknownFileKeys(ks, v, path)...)

	l.base = map[string]resolved{}
	settings := make([]Setting, 0, len(ks)+1)
	for _, k := range ks {
		raw, src, err := l.resolve(k, v, path)
		l.base[k.Name] = resolved{raw: raw, src: src}
		if err == nil {
			err = k.parse(raw)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s (from %s): %w", k.Name, src, err))
		}
		settings = append(settings, Setting{Key: k.Name, Value: k.show(), Source: src})
	}
	groups, groupSettings, groupErrs := loadGroups(v, path)
	l.groups, l.groupSettings = groups, groupSettings
	cfg.PowerDNS.Groups = groups
	settings = append(settings, groupSettings...)
	errs = append(errs, groupErrs...)
	slices.SortStableFunc(settings, func(a, b Setting) int { return strings.Compare(a.Key, b.Key) })
	return cfg, settings, errors.Join(errs...)
}

// WithStored returns the configuration of the last Load, which must have
// succeeded, with stored, the runtime settings that the database stores, by
// key, between the defaults and the config file (ADR-0040). A runtime key
// that the file, the environment or a flag sets is pinned: its stored
// value is kept, but not used. It fails on a stored value that isn't valid,
// or a key that isn't a runtime key's.
func (l *Loader) WithStored(stored map[string]string) (*Config, []Setting, error) {
	cfg := &Config{}
	ks := keys(cfg)
	var errs []error
	settings := make([]Setting, 0, len(ks)+len(l.groupSettings))
	known := map[string]bool{}
	for _, k := range ks {
		known[k.Name] = true
		r := l.base[k.Name]
		st := Setting{Key: k.Name, Source: r.src}
		raw := r.raw
		if value, ok := stored[k.Name]; ok && !k.Runtime {
			errs = append(errs, fmt.Errorf("%s (from %s): it's a bootstrap key, which the database can't set", k.Name, SourceDatabase))
		} else if ok {
			shown := value
			if k.Secret {
				shown = Redacted
			}
			st.Stored = &shown
			if r.src.Kind == "default" {
				raw, st.Source = value, Source{Kind: SourceDatabase}
			}
		}
		if err := k.parse(raw); err != nil {
			errs = append(errs, fmt.Errorf("%s (from %s): %w", k.Name, st.Source, err))
		}
		st.Value = k.show()
		settings = append(settings, st)
	}
	for key := range stored {
		if !known[key] {
			errs = append(errs, fmt.Errorf("the database stores %s, which isn't a key", key))
		}
	}
	cfg.PowerDNS.Groups = l.groups
	settings = append(settings, l.groupSettings...)
	slices.SortStableFunc(settings, func(a, b Setting) int { return strings.Compare(a.Key, b.Key) })
	return cfg, settings, errors.Join(errs...)
}

// BootstrapError is the error for a bootstrap key given where only a runtime
// key may go.
type BootstrapError struct{ Key Key }

func (e *BootstrapError) Error() string {
	return fmt.Sprintf("%s is a bootstrap key, which only the config file, %s or --%s sets", e.Key.Name, e.Key.Env(), e.Key.Flag())
}

// CheckSetting returns the runtime key named name, and checks that value is
// valid for it, as `nbpdns settings set` stores it. It returns a
// *BootstrapError for a bootstrap key. Its error never holds the value,
// which may be a secret.
func CheckSetting(name, value string) (Key, error) {
	for _, k := range keys(&Config{}) {
		if k.Name != name {
			continue
		}
		if !k.Runtime {
			return k, &BootstrapError{Key: k}
		}
		if err := k.parse(value); err != nil {
			return k, fmt.Errorf("%s: %w", name, err)
		}
		return k, nil
	}
	return Key{}, fmt.Errorf("there's no key %s; nbpdns settings list lists the runtime keys", name)
}

// resolve returns k's raw value and its source. For a secret, the file form
// counts at the same level as the plain one, and both at one level is an
// error.
func (l *Loader) resolve(k Key, v *viper.Viper, path string) (any, Source, error) {
	type layer struct {
		src               Source
		fileSrc           Source
		plainSet, fileSet bool
	}
	layers := []layer{
		{src: Source{"flag", "--" + k.Flag()}, fileSrc: Source{"flag", "--" + k.FileFlag()},
			plainSet: l.changed(k.Flag()), fileSet: k.Secret && l.changed(k.FileFlag())},
		{src: Source{"env", k.Env()}, fileSrc: Source{"env", k.FileEnv()},
			plainSet: os.Getenv(k.Env()) != "", fileSet: k.Secret && os.Getenv(k.FileEnv()) != ""},
		{src: Source{"file", path}, fileSrc: Source{"file", path},
			plainSet: v.InConfig(k.Name), fileSet: k.Secret && v.InConfig(k.FileName())},
	}
	for _, ly := range layers {
		switch {
		case ly.plainSet && ly.fileSet:
			return nil, ly.src, fmt.Errorf("both %s and %s are set; set one", ly.src.Name, ly.fileSrc.Name)
		case ly.fileSet:
			// No higher layer sets either form, so Viper's value is this one.
			s, err := readSecretFile(v.GetString(k.FileName()))
			return s, ly.fileSrc, err
		case ly.plainSet:
			return v.Get(k.Name), ly.src, nil
		}
	}
	return v.Get(k.Name), Source{Kind: "default"}, nil
}

func (l *Loader) changed(flag string) bool {
	if l.flags == nil {
		return false
	}
	f := l.flags.Lookup(flag)
	return f != nil && f.Changed
}

// readSecretFile returns a secret file's content, without a final newline.
func readSecretFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading the secret file: %w", err)
	}
	s := strings.TrimRight(string(b), "\r\n")
	if s == "" {
		return "", fmt.Errorf("the secret file %s is empty", path)
	}
	return s, nil
}

// unknownEnv reports every NBPDNS_ environment variable that names no key.
func unknownEnv(ks []Key) []error {
	known := []string{ConfigEnv, GroupAPIKeyEnv}
	for _, k := range ks {
		known = append(known, k.Env())
		if k.Secret {
			known = append(known, k.FileEnv())
		}
	}
	var errs []error
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, EnvPrefix) && !slices.Contains(known, name) {
			errs = append(errs, fmt.Errorf("unknown environment variable %s", name))
		}
	}
	slices.SortFunc(errs, func(a, b error) int { return strings.Compare(a.Error(), b.Error()) })
	return errs
}

// unknownFileKeys reports every key in the config file that isn't a
// configuration key. Keys from the defaults, the environment and the flags
// are all declared, so any other key came from the file.
func unknownFileKeys(ks []Key, v *viper.Viper, path string) []error {
	known := []string{GroupsKey}
	for _, k := range ks {
		known = append(known, k.Name)
		if k.Secret {
			known = append(known, k.FileName())
		}
	}
	var errs []error
	for _, key := range v.AllKeys() {
		if !slices.Contains(known, key) {
			errs = append(errs, fmt.Errorf("config file %s: unknown key %s", path, key))
		}
	}
	slices.SortFunc(errs, func(a, b error) int { return strings.Compare(a.Error(), b.Error()) })
	return errs
}

// flagValue is a flag that keeps its argument as given. Load parses it with
// its key's rules, so a flag, an environment variable and the config file
// are all validated the same way. Its type name is only for --help.
type flagValue struct {
	typ   string
	value string
}

func (f *flagValue) String() string     { return f.value }
func (f *flagValue) Set(s string) error { f.value = s; return nil }
func (f *flagValue) Type() string       { return f.typ }
