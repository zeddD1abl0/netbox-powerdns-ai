package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

// clearEnv unsets every NBPDNS_ variable for the test, so the host's
// environment can't leak in. t.Setenv restores them afterwards.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, EnvPrefix) {
			t.Setenv(name, "")
			if err := os.Unsetenv(name); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// load runs a Loader over a config file with fileYAML (none if empty), the
// environment env, and the command-line args.
func load(t *testing.T, fileYAML string, env map[string]string, args ...string) (*Config, []Setting, error) {
	t.Helper()
	clearEnv(t)
	for k, v := range env {
		t.Setenv(k, v)
	}
	if fileYAML != "" {
		path := filepath.Join(t.TempDir(), "nbpdns.yaml")
		if err := os.WriteFile(path, []byte(fileYAML), 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, "--config", path)
	}
	l := NewLoader()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	l.AddFlags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	return l.Load()
}

// secretFile writes content to a new file and returns its path.
func secretFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// setting returns the named key's setting.
func setting(t *testing.T, settings []Setting, key string) Setting {
	t.Helper()
	for _, s := range settings {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("no setting for %s", key)
	return Setting{}
}

func TestLoadDefaults(t *testing.T) {
	cfg, settings, err := load(t, "", nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Config{
		Log:      LogConfig{Level: "info", Format: "json"},
		NetBox:   NetBoxConfig{Timeout: 30 * time.Second, PageSize: 500, Concurrency: 4},
		PowerDNS: PowerDNSConfig{Timeout: 30 * time.Second, Concurrency: 4},
		Drift:    DriftConfig{GroupConcurrency: 4, Interval: 5 * time.Minute, Timeout: 10 * time.Minute, WebhookDelay: 3 * time.Second},
		Server:   ServerConfig{Listen: ":8080"},
		OTLP:     OTLPConfig{Protocol: OTLPHTTP, Timeout: 10 * time.Second},
	}
	if !reflect.DeepEqual(*cfg, want) {
		t.Errorf("Load() = %+v, want %+v", *cfg, want)
	}
	for _, s := range settings {
		if s.Source.Kind != "default" {
			t.Errorf("%s: source = %v, want default", s.Key, s.Source)
		}
	}
}

// TestLoadPrecedence checks defaults < file < env < flags, for a plain key
// and for a secret, whose file form counts at its plain form's level.
func TestLoadPrecedence(t *testing.T) {
	fileTok := secretFile(t, "from-file-form\n")
	tests := []struct {
		name      string
		file      string
		env       map[string]string
		args      []string
		key       string
		wantValue string
		wantKind  string
		wantName  string // the variable or flag; "" for the file and default
	}{
		{"default", "", nil, nil, "log.level", "info", "default", ""},
		{"file over default", "log:\n  level: warn\n", nil, nil, "log.level", "warn", "file", ""},
		{"env over file", "log:\n  level: warn\n", map[string]string{"NBPDNS_LOG_LEVEL": "error"}, nil, "log.level", "error", "env", "NBPDNS_LOG_LEVEL"},
		{"flag over env", "log:\n  level: warn\n", map[string]string{"NBPDNS_LOG_LEVEL": "error"}, []string{"--log-level", "debug"}, "log.level", "debug", "flag", "--log-level"},
		{"flag over file", "log:\n  level: warn\n", nil, []string{"--log-level=debug"}, "log.level", "debug", "flag", "--log-level"},
		{"empty env counts as unset", "log:\n  level: warn\n", map[string]string{"NBPDNS_LOG_LEVEL": ""}, nil, "log.level", "warn", "file", ""},
		{"null in file counts as unset", "netbox:\n  page_size:\n", nil, nil, "netbox.page_size", "500", "default", ""},

		{"secret from env", "", map[string]string{"NBPDNS_NETBOX_TOKEN": "t"}, nil, "netbox.token", "t", "env", "NBPDNS_NETBOX_TOKEN"},
		{"secret file form from env", "", map[string]string{"NBPDNS_NETBOX_TOKEN_FILE": fileTok}, nil, "netbox.token", "from-file-form", "env", "NBPDNS_NETBOX_TOKEN_FILE"},
		{"secret env file form over plain file key", "netbox:\n  token: in-file\n", map[string]string{"NBPDNS_NETBOX_TOKEN_FILE": fileTok}, nil, "netbox.token", "from-file-form", "env", "NBPDNS_NETBOX_TOKEN_FILE"},
		{"secret env over file form in file", "netbox:\n  token_file: " + fileTok + "\n", map[string]string{"NBPDNS_NETBOX_TOKEN": "t"}, nil, "netbox.token", "t", "env", "NBPDNS_NETBOX_TOKEN"},
		{"secret file form in file", "netbox:\n  token_file: " + fileTok + "\n", nil, nil, "netbox.token", "from-file-form", "file", ""},
		{"secret file-form flag over env", "", map[string]string{"NBPDNS_NETBOX_TOKEN": "t"}, []string{"--netbox-token-file", fileTok}, "netbox.token", "from-file-form", "flag", "--netbox-token-file"},
		{"secret flag over env file form", "", map[string]string{"NBPDNS_NETBOX_TOKEN_FILE": fileTok}, []string{"--netbox-token", "f"}, "netbox.token", "f", "flag", "--netbox-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, settings, err := load(t, tt.file, tt.env, tt.args...)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			got := setting(t, settings, tt.key)
			if tt.key == "netbox.token" {
				if cfg.NetBox.Token.Reveal() != tt.wantValue {
					t.Errorf("token = %q, want %q", cfg.NetBox.Token.Reveal(), tt.wantValue)
				}
				if got.Value != Redacted {
					t.Errorf("setting value = %q, want %q", got.Value, Redacted)
				}
			} else if got.Value != tt.wantValue {
				t.Errorf("value = %q, want %q", got.Value, tt.wantValue)
			}
			if got.Source.Kind != tt.wantKind {
				t.Errorf("source = %v, want kind %s", got.Source, tt.wantKind)
			}
			if tt.wantKind == "env" || tt.wantKind == "flag" {
				if got.Source.Name != tt.wantName {
					t.Errorf("source = %v, want %s %s", got.Source, tt.wantKind, tt.wantName)
				}
			}
			if tt.wantKind == "file" && !strings.HasSuffix(got.Source.Name, "nbpdns.yaml") {
				t.Errorf("source = %v, want the config file", got.Source)
			}
		})
	}
}

// TestLoadErrors checks that each invalid source is reported, with the key
// and where the value came from.
func TestLoadErrors(t *testing.T) {
	tok := secretFile(t, "t")
	empty := secretFile(t, "\n")
	tests := []struct {
		name string
		file string
		env  map[string]string
		args []string
		want []string // substrings of the error
	}{
		{"both secret forms in env", "", map[string]string{"NBPDNS_NETBOX_TOKEN": "t", "NBPDNS_NETBOX_TOKEN_FILE": tok}, nil,
			[]string{"netbox.token", "both NBPDNS_NETBOX_TOKEN and NBPDNS_NETBOX_TOKEN_FILE are set"}},
		{"both secret forms as flags", "", nil, []string{"--netbox-token", "t", "--netbox-token-file", tok},
			[]string{"both --netbox-token and --netbox-token-file are set"}},
		{"both secret forms in file", "netbox:\n  token: t\n  token_file: " + tok + "\n", nil, nil,
			[]string{"netbox.token (from file", "both"}},
		{"missing secret file", "", map[string]string{"NBPDNS_NETBOX_TOKEN_FILE": "/does/not/exist"}, nil,
			[]string{"netbox.token (from env NBPDNS_NETBOX_TOKEN_FILE)", "reading the secret file"}},
		{"empty secret file", "", map[string]string{"NBPDNS_NETBOX_TOKEN_FILE": empty}, nil,
			[]string{"is empty"}},
		{"unknown file key", "netbox:\n  tokn: t\n", nil, nil,
			[]string{"unknown key netbox.tokn"}},
		{"unknown top-level file key", "logging:\n  level: debug\n", nil, nil,
			[]string{"unknown key logging.level"}},
		{"scalar where a mapping belongs", "netbox: 5\n", nil, nil,
			[]string{"unknown key netbox"}},
		{"unknown env", "", map[string]string{"NBPDNS_NETBOX_TOKN": "t"}, nil,
			[]string{"unknown environment variable NBPDNS_NETBOX_TOKN"}},
		{"unknown empty env", "", map[string]string{"NBPDNS_TYPO": ""}, nil,
			[]string{"unknown environment variable NBPDNS_TYPO"}},
		{"missing config file", "", map[string]string{"NBPDNS_CONFIG": "/does/not/exist.yaml"}, nil,
			[]string{"config file /does/not/exist.yaml"}},
		{"invalid YAML", "netbox: [\n", nil, nil,
			[]string{"config file"}},
		{"bad level", "", map[string]string{"NBPDNS_LOG_LEVEL": "verbose"}, nil,
			[]string{`log.level (from env NBPDNS_LOG_LEVEL): "verbose" isn't one of debug, info, warn, error`}},
		{"bad format flag", "", nil, []string{"--log-format", "xml"},
			[]string{"log.format (from flag --log-format)"}},
		{"page size not a number", "", map[string]string{"NBPDNS_NETBOX_PAGE_SIZE": "lots"}, nil,
			[]string{`"lots" isn't an integer`}},
		{"page size too big", "netbox:\n  page_size: 1001\n", nil, nil,
			[]string{"1001 is outside 1 to 1000"}},
		{"concurrency zero", "", nil, []string{"--netbox-concurrency", "0"},
			[]string{"0 is outside 1 to 32"}},
		{"page size as a float", "netbox:\n  page_size: 1.5\n", nil, nil,
			[]string{"want an integer"}},
		{"timeout without a unit", "", map[string]string{"NBPDNS_NETBOX_TIMEOUT": "30"}, nil,
			[]string{`"30" isn't a duration`}},
		{"timeout as a YAML number", "netbox:\n  timeout: 30\n", nil, nil,
			[]string{"want a duration with a unit"}},
		{"negative timeout", "", nil, []string{"--netbox-timeout", "-1s"},
			[]string{"isn't positive"}},
		{"a drift interval too short", "", map[string]string{"NBPDNS_DRIFT_INTERVAL": "5s"}, nil,
			[]string{"drift.interval", "5s is shorter than 10s"}},
		{"a webhook delay too long", "", map[string]string{"NBPDNS_DRIFT_WEBHOOK_DELAY": "1m"}, nil,
			[]string{"drift.webhook_delay", "1m0s is longer than 30s"}},
		{"a webhook delay too short", "", nil, []string{"--drift-webhook-delay", "10ms"},
			[]string{"drift.webhook_delay", "10ms is shorter than 100ms"}},
		{"a listen address without a port", "", map[string]string{"NBPDNS_SERVER_LISTEN": "localhost"}, nil,
			[]string{"server.listen", `"localhost" isn't an address`}},
		{"a listen port out of range", "", nil, []string{"--server-listen", ":70000"},
			[]string{"has no port number from 0 to 65535"}},
		{"URL with credentials and another scheme", "", map[string]string{"NBPDNS_OTLP_ENDPOINT": "grpc://user:s3cret@otel.example.com"}, nil,
			[]string{"otlp.endpoint", "mustn't contain credentials"}},
		{"URL without a scheme", "", map[string]string{"NBPDNS_NETBOX_URL": "netbox.example.com"}, nil,
			[]string{"needs an http:// or https:// scheme"}},
		{"URL with another scheme", "", map[string]string{"NBPDNS_NETBOX_URL": "ftp://netbox.example.com"}, nil,
			[]string{"needs an http:// or https:// scheme"}},
		{"URL with credentials", "", map[string]string{"NBPDNS_NETBOX_URL": "https://user:pw@netbox.example.com"}, nil,
			[]string{"mustn't contain credentials; set the token or API key in its own key"}},
		{"a token that YAML reads as a number", "netbox:\n  token: 12345\n", nil, nil,
			[]string{"netbox.token (from file", "want a string, not a number; put it in quotes"}},
		{"a webhook secret too short to be safe", "", map[string]string{"NBPDNS_NETBOX_WEBHOOK_SECRET": "s3cret-is-short"}, nil,
			[]string{"netbox.webhook_secret (from env NBPDNS_NETBOX_WEBHOOK_SECRET)", "at least 16 characters"}},
		{"URL with a query", "", map[string]string{"NBPDNS_NETBOX_URL": "https://netbox.example.com/?x=1"}, nil,
			[]string{"mustn't have a query"}},
		{"a mapping for a string", "netbox:\n  url:\n    host: x\n", nil, nil,
			[]string{"netbox.url (from file", "want a string, not a mapping"}},
		{
			"every problem at once", "netbox:\n  tokn: t\n",
			map[string]string{"NBPDNS_LOG_LEVEL": "loud", "NBPDNS_TYPO": "x"}, []string{"--netbox-page-size", "0"},
			[]string{"unknown key netbox.tokn", "unknown environment variable NBPDNS_TYPO", `"loud" isn't one of`, "0 is outside"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := load(t, tt.file, tt.env, tt.args...)
			if err == nil {
				t.Fatal("Load() succeeded, want an error")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error doesn't contain %q:\n%v", w, err)
				}
			}
			if strings.Contains(err.Error(), "s3cret") {
				t.Errorf("the error shows a secret:\n%v", err)
			}
		})
	}
}

// TestLoadAcceptsGoodValues covers valid values of each kind.
func TestLoadAcceptsGoodValues(t *testing.T) {
	hookSecret := secretFile(t, "0123456789abcdef\n")
	cfg, _, err := load(t,
		"log:\n  format: text\nnetbox:\n  url: http://netbox.lab:8047\n  page_size: \"1000\"\n  timeout: 2m\n  ca_file: /etc/ssl/netbox.pem\n",
		map[string]string{"NBPDNS_NETBOX_CONCURRENCY": " 32 ", "NBPDNS_NETBOX_WEBHOOK_SECRET_FILE": hookSecret})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	n := cfg.NetBox
	if cfg.Log.Format != "text" || n.URL != "http://netbox.lab:8047" || n.PageSize != 1000 ||
		n.Timeout != 2*time.Minute || n.CAFile != "/etc/ssl/netbox.pem" || n.Concurrency != 32 ||
		n.WebhookSecret.Reveal() != "0123456789abcdef" {
		t.Errorf("Load() = %+v", *cfg)
	}
}

// TestLoadWithoutFlags checks that a Loader with no flag set still reads the
// other sources.
func TestLoadWithoutFlags(t *testing.T) {
	clearEnv(t)
	t.Setenv("NBPDNS_LOG_LEVEL", "warn")
	cfg, _, err := NewLoader().Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Log.Level != "warn" {
		t.Errorf("log.level = %q, want warn", cfg.Log.Level)
	}
}
