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
}

// Source says where a setting's value came from.
type Source struct {
	// Kind is default, file, env or flag.
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
// precedence: the defaults, the config file, the environment and the flags.
// Add its flags to a command with AddFlags, and call Load once they're
// parsed.
type Loader struct {
	flags      *pflag.FlagSet
	configPath string
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
	if path != "" {
		v.SetConfigFile(path)
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err != nil {
			errs = append(errs, fmt.Errorf("config file %s: %w", path, err))
		}
	}

	errs = append(errs, unknownEnv(ks)...)
	errs = append(errs, unknownFileKeys(ks, v, path)...)

	settings := make([]Setting, 0, len(ks))
	for _, k := range ks {
		raw, src, err := l.resolve(k, v, path)
		if err == nil {
			err = k.parse(raw)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s (from %s): %w", k.Name, src, err))
		}
		settings = append(settings, Setting{Key: k.Name, Value: k.show(), Source: src})
	}
	return cfg, settings, errors.Join(errs...)
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
	known := []string{ConfigEnv}
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
	known := []string{}
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
