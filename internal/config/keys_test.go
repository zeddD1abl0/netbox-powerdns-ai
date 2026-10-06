package config

import (
	"regexp"
	"strings"
	"testing"
)

// TestKeysAreWellFormed checks the invariants that the loader, the flags and
// the generated reference rely on.
func TestKeysAreWellFormed(t *testing.T) {
	nameRE := regexp.MustCompile(`^[a-z]+\.[a-z][a-z_]*[a-z]$`)
	seen := map[string]string{} // every derived name -> the key it came from
	claim := func(kind, name, key string) {
		if prev, dup := seen[kind+" "+name]; dup {
			t.Errorf("%s %s is used by both %s and %s", kind, name, prev, key)
		}
		seen[kind+" "+name] = key
	}
	claim("env", ConfigEnv, "--config")
	claim("flag", "config", "--config")
	for _, k := range Keys() {
		// Two levels, which the example config file in the reference assumes.
		if !nameRE.MatchString(k.Name) {
			t.Errorf("%s: want group.name, in lowercase with underscores", k.Name)
		}
		if !strings.HasSuffix(k.Summary, ".") || strings.Contains(k.Summary, "\n") {
			t.Errorf("%s: the summary should be one sentence, ending with a period", k.Name)
		}
		if k.typ == "" || k.flagType == "" || k.parse == nil || k.show == nil {
			t.Errorf("%s: declared without a constructor", k.Name)
		}
		if k.Secret && k.Default != "" {
			t.Errorf("%s: a secret mustn't have a default", k.Name)
		}
		claim("key", k.Name, k.Name)
		claim("env", k.Env(), k.Name)
		claim("flag", k.Flag(), k.Name)
		if k.Secret {
			claim("key", k.FileName(), k.Name)
			claim("env", k.FileEnv(), k.Name)
			claim("flag", k.FileFlag(), k.Name)
		}
	}
}

func TestKeyNames(t *testing.T) {
	k := Key{Name: "netbox.ca_file"}
	if k.Env() != "NBPDNS_NETBOX_CA_FILE" || k.Flag() != "netbox-ca-file" {
		t.Errorf("netbox.ca_file: env %s, flag %s", k.Env(), k.Flag())
	}
	s := Key{Name: "netbox.token"}
	if s.FileName() != "netbox.token_file" || s.FileEnv() != "NBPDNS_NETBOX_TOKEN_FILE" || s.FileFlag() != "netbox-token-file" {
		t.Errorf("netbox.token: file key %s, env %s, flag %s", s.FileName(), s.FileEnv(), s.FileFlag())
	}
}

// TestKeysSorted checks that the registry, and so `config show` and the
// reference, list keys by name.
func TestKeysSorted(t *testing.T) {
	ks := Keys()
	for i := 1; i < len(ks); i++ {
		if ks[i-1].Name >= ks[i].Name {
			t.Errorf("%s comes before %s", ks[i-1].Name, ks[i].Name)
		}
	}
}

func TestUnsetError(t *testing.T) {
	tests := []struct{ key, want string }{
		{"netbox.url", "netbox.url isn't set; set it in the config file, set NBPDNS_NETBOX_URL, or pass --netbox-url"},
		{"netbox.token", "netbox.token isn't set; set it, or netbox.token_file for a file that holds it, in the config file; " +
			"or set NBPDNS_NETBOX_TOKEN or NBPDNS_NETBOX_TOKEN_FILE; or pass --netbox-token or --netbox-token-file"},
		{"no.such_key", "no.such_key isn't set"},
	}
	for _, tt := range tests {
		if got := UnsetError(tt.key).Error(); got != tt.want {
			t.Errorf("UnsetError(%s) = %q, want %q", tt.key, got, tt.want)
		}
	}
}
