package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestWithStored checks the database's place among the sources: above the
// defaults, below the file, the environment and the flags, which pin a
// stored value.
func TestWithStored(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		env    map[string]string
		stored map[string]string
		key    string
		value  string
		source string
		pinned string // the stored value, if it's kept but not used
	}{
		{"a stored value", "", nil, map[string]string{"drift.interval": "1m"}, "drift.interval", "1m0s", "database", ""},
		{"nothing stored", "", nil, nil, "drift.interval", "5m0s", "default", ""},
		{"pinned by the environment", "", map[string]string{"NBPDNS_DRIFT_INTERVAL": "2m"}, map[string]string{"drift.interval": "1m"},
			"drift.interval", "2m0s", "env NBPDNS_DRIFT_INTERVAL", "1m"},
		{"pinned by the file", "log:\n  level: warn\n", nil, map[string]string{"log.level": "debug"}, "log.level", "warn", "file", "debug"},
		{"a stored secret", "", nil, map[string]string{"netbox.token": "nbt_abc.s3cret"}, "netbox.token", Redacted, "database", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			var args []string
			if tt.file != "" {
				path := filepath.Join(t.TempDir(), "nbpdns.yaml")
				if err := os.WriteFile(path, []byte(tt.file), 0o600); err != nil {
					t.Fatal(err)
				}
				args = []string{"--config", path}
			}
			l := loader(t, args...)
			if _, _, err := l.Load(); err != nil {
				t.Fatal(err)
			}
			cfg, settings, err := l.WithStored(tt.stored)
			if err != nil {
				t.Fatal(err)
			}
			st := setting(t, settings, tt.key)
			if st.Value != tt.value || !strings.HasPrefix(st.Source.String(), tt.source) {
				t.Errorf("%s = %q from %s, want %q from %s", tt.key, st.Value, st.Source, tt.value, tt.source)
			}
			if stored, ok := tt.stored[tt.key]; ok {
				want := stored
				if strings.HasSuffix(tt.key, "token") {
					want = Redacted
				}
				if st.Stored == nil || *st.Stored != want {
					t.Errorf("%s's stored value = %v, want %q", tt.key, st.Stored, want)
				}
			} else if st.Stored != nil {
				t.Errorf("%s's stored value = %q, want none", tt.key, *st.Stored)
			}
			if tt.key == "drift.interval" && tt.source == "database" && cfg.Drift.Interval != time.Minute {
				t.Errorf("drift.interval = %v, want 1m", cfg.Drift.Interval)
			}
			if tt.key == "netbox.token" && cfg.NetBox.Token.Reveal() != "nbt_abc.s3cret" {
				t.Error("the stored token isn't the configuration's")
			}
		})
	}
}

func TestWithStoredErrors(t *testing.T) {
	tests := []struct {
		name   string
		stored map[string]string
		want   string
	}{
		{"a bootstrap key", map[string]string{"database.url": "sqlite:///x.db"}, "database.url (from database): it's a bootstrap key"},
		{"an unknown key", map[string]string{"drift.intervall": "1m"}, "the database stores drift.intervall, which isn't a key"},
		{"an invalid value", map[string]string{"drift.interval": "1s"}, "drift.interval (from database): 1s is shorter than 10s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			l := loader(t)
			if _, _, err := l.Load(); err != nil {
				t.Fatal(err)
			}
			_, _, err := l.WithStored(tt.stored)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("WithStored() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCheckSetting(t *testing.T) {
	tests := []struct {
		name, key, value string
		want             string // part of the error, or empty
	}{
		{"a runtime key", "drift.interval", "30s", ""},
		{"a secret", "netbox.webhook_secret", "0123456789abcdef", ""},
		{"a bootstrap key", "database.url", "sqlite:///x.db", "database.url is a bootstrap key, which only the config file, NBPDNS_DATABASE_URL or --database-url sets"},
		{"an unknown key", "drift.intervall", "30s", "there's no key drift.intervall"},
		{"an invalid value", "netbox.page_size", "5000", "netbox.page_size: 5000 is outside 1 to 1000"},
		{"a secret too short", "netbox.webhook_secret", "s3cret", "at least 16 characters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, err := CheckSetting(tt.key, tt.value)
			switch {
			case tt.want == "" && err != nil:
				t.Errorf("CheckSetting() = %v", err)
			case tt.want == "" && k.Name != tt.key:
				t.Errorf("CheckSetting() key = %q", k.Name)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Errorf("CheckSetting() = %v, want %q", err, tt.want)
			case err != nil && strings.Contains(err.Error(), "s3cret"):
				t.Errorf("the error shows the secret: %v", err)
			}
		})
	}
	var be *BootstrapError
	if _, err := CheckSetting("server.listen", ":9090"); !errors.As(err, &be) {
		t.Errorf("CheckSetting(server.listen) = %v, want a BootstrapError", err)
	}
}

// TestRuntimeKeys pins which keys are runtime keys, as ADR-0040 lists them.
func TestRuntimeKeys(t *testing.T) {
	var runtime []string
	for _, k := range Keys() {
		if k.Runtime {
			runtime = append(runtime, k.Name)
		}
	}
	want := "audit.retention_days drift.group_concurrency drift.history_retention_days drift.interval drift.timeout " +
		"drift.webhook_delay log.level netbox.concurrency netbox.page_size netbox.timeout netbox.token " +
		"netbox.webhook_secret powerdns.concurrency powerdns.timeout"
	if got := strings.Join(runtime, " "); got != want {
		t.Errorf("runtime keys = %s, want %s", got, want)
	}
}
