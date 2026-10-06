package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// groupsYAML returns a config file declaring entries under powerdns.groups.
func groupsYAML(entries ...string) string {
	return "powerdns:\n  groups:\n" + strings.Join(entries, "")
}

// entry returns one group's YAML, with fields given as further lines, each
// indented under the entry.
func entry(lines ...string) string {
	var b strings.Builder
	for i, l := range lines {
		if i == 0 {
			fmt.Fprintf(&b, "    - %s\n", l)
		} else {
			fmt.Fprintf(&b, "      %s\n", l)
		}
	}
	return b.String()
}

func TestGroups(t *testing.T) {
	keyFile := secretFile(t, "pdns-key-from-file\n")
	cfg, settings, err := load(t, groupsYAML(
		entry("name: site-a", "views: [_default_, internal]", "primary:",
			"  url: https://pdns-a.example.com:8443", "  api_key: inline-key",
			"  ca_file: /etc/nbpdns/ca.pem", "  cert_file: /etc/nbpdns/client.pem", "  key_file: /etc/nbpdns/client-key.pem"),
		entry("name: site-b", "views: [_default_]", "primary:",
			"  url: http://pdns-b.example.com:8081", "  api_key_file: "+keyFile, "  server_id: other"),
	), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []Group{
		{Name: "site-a", Views: []string{"_default_", "internal"}, Primary: Primary{
			URL: "https://pdns-a.example.com:8443", APIKey: NewSecret("inline-key"), ServerID: "localhost",
			CAFile: "/etc/nbpdns/ca.pem", CertFile: "/etc/nbpdns/client.pem", KeyFile: "/etc/nbpdns/client-key.pem",
		}},
		{Name: "site-b", Views: []string{"_default_"}, Primary: Primary{
			URL: "http://pdns-b.example.com:8081", APIKey: NewSecret("pdns-key-from-file"), APIKeyFile: keyFile, ServerID: "other",
		}},
	}
	if !reflect.DeepEqual(cfg.PowerDNS.Groups, want) {
		t.Errorf("groups = %+v,\nwant %+v", cfg.PowerDNS.Groups, want)
	}
	if g, ok := cfg.PowerDNS.Group("site-b"); !ok || g.Primary.ServerID != "other" {
		t.Errorf("Group(site-b) = %+v, %v", g, ok)
	}
	if _, ok := cfg.PowerDNS.Group("site-c"); ok {
		t.Error("Group(site-c) found a group")
	}

	tests := []struct{ key, value, source string }{
		{"powerdns.groups.site-a.views", "_default_,internal", "file"},
		{"powerdns.groups.site-a.primary.api_key", Redacted, "file"},
		{"powerdns.groups.site-a.primary.api_key_file", "", "default"},
		{"powerdns.groups.site-a.primary.server_id", "localhost", "default"},
		{"powerdns.groups.site-b.primary.api_key", Redacted, "file"},
		{"powerdns.groups.site-b.primary.api_key_file", keyFile, "file"},
		{"powerdns.groups.site-b.primary.ca_file", "", "default"},
	}
	for _, tt := range tests {
		if s := setting(t, settings, tt.key); s.Value != tt.value || s.Source.Kind != tt.source {
			t.Errorf("%s = %+v, want %q from %s", tt.key, s, tt.value, tt.source)
		}
	}
	b, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "inline-key") || strings.Contains(string(b), "pdns-key-from-file") {
		t.Errorf("a key is in the settings: %s", b)
	}
	for i := 1; i < len(settings); i++ {
		if settings[i-1].Key > settings[i].Key {
			t.Errorf("settings aren't sorted: %s before %s", settings[i-1].Key, settings[i].Key)
		}
	}
}

func TestNoGroups(t *testing.T) {
	cfg, settings, err := load(t, "", nil)
	if err != nil || cfg.PowerDNS.Groups != nil {
		t.Fatalf("groups %v, error %v", cfg.PowerDNS.Groups, err)
	}
	if s := setting(t, settings, GroupsKey); s.Value != "" || s.Source.Kind != "default" {
		t.Errorf("%s = %+v", GroupsKey, s)
	}
}

func TestGroupErrors(t *testing.T) {
	ok := []string{"views: [v]", "primary:", "  url: https://pdns.example.com", "  api_key: k"}
	group := func(name string, fields ...string) string {
		return entry(append([]string{"name: " + name}, fields...)...)
	}
	replace := func(old, repl string) []string {
		out := make([]string, 0, len(ok))
		for _, l := range ok {
			if l == old {
				if repl == "" {
					continue
				}
				l = repl
			}
			out = append(out, l)
		}
		return out
	}
	missing := secretFile(t, "")
	tests := []struct {
		name, yaml string
		want       []string
	}{
		{"not a list", "powerdns:\n  groups: site-a\n", []string{"powerdns.groups (from file", "want a list of groups"}},
		{"an entry that isn't a mapping", groupsYAML("    - site-a\n"), []string{"powerdns.groups[0]", "want a mapping"}},
		{"primary that isn't a mapping", groupsYAML(group("a", "views: [v]", "primary: https://x")), []string{"primary: want a mapping"}},
		{"an unknown key", groupsYAML(group("a", append(ok, "  port: 8081")...)), []string{"powerdns.groups[0] (a)", "unknown key primary.port"}},
		{"an unknown top-level key", groupsYAML(group("a", append(ok, "secondaries: []")...)), []string{"unknown key secondaries"}},
		{"no name", groupsYAML(entry(ok...)), []string{"name isn't set"}},
		{"a bad name", groupsYAML(group("Site_A", ok...)), []string{`name: "Site_A" isn't lowercase`}},
		{"a name ending in a hyphen", groupsYAML(group("a-", ok...)), []string{"name:"}},
		{"no views", groupsYAML(group("a", replace("views: [v]", "")...)), []string{"views isn't set"}},
		{"an empty views list", groupsYAML(group("a", replace("views: [v]", "views: []")...)), []string{"views: name at least one view"}},
		{"views as a string", groupsYAML(group("a", replace("views: [v]", "views: v")...)), []string{"views: want a list of views"}},
		{"a view listed twice", groupsYAML(group("a", replace("views: [v]", "views: [v, v]")...)), []string{"views: v is listed twice"}},
		{"no URL", groupsYAML(group("a", replace("  url: https://pdns.example.com", "")...)), []string{"primary.url isn't set"}},
		{"a URL without a scheme", groupsYAML(group("a", replace("  url: https://pdns.example.com", "  url: pdns.example.com")...)),
			[]string{"primary.url:", "needs an http:// or https:// scheme"}},
		{"a URL with credentials", groupsYAML(group("a", replace("  url: https://pdns.example.com", "  url: https://u:p@pdns.example.com")...)),
			[]string{"primary.url: the URL mustn't contain credentials"}},
		{"no API key", groupsYAML(group("a", replace("  api_key: k", "")...)), []string{"neither primary.api_key nor primary.api_key_file is set"}},
		{"both API key forms", groupsYAML(group("a", append(ok, "  api_key_file: /x")...)), []string{"both primary.api_key and primary.api_key_file are set"}},
		{"an empty key file", groupsYAML(group("a", replace("  api_key: k", "  api_key_file: "+missing)...)), []string{"primary.api_key_file:", "is empty"}},
		{"a missing key file", groupsYAML(group("a", replace("  api_key: k", "  api_key_file: /does/not/exist")...)), []string{"primary.api_key_file: reading the secret file"}},
		{"a certificate without its key", groupsYAML(group("a", append(ok, "  cert_file: /c.pem")...)), []string{"primary.cert_file and primary.key_file go together"}},
		{"a bad server ID", groupsYAML(group("a", append(ok, "  server_id: a/b")...)), []string{`primary.server_id: "a/b" isn't a server ID`}},
		{"a key that YAML reads as a number", groupsYAML(group("a", replace("  api_key: k", "  api_key: 1e10")...)),
			[]string{"primary.api_key: want a string, not a number; put it in quotes"}},
		{"two groups with one name", groupsYAML(group("a", ok...), group("a", ok...)), []string{"powerdns.groups[1] (a)", "another group is named a"}},
		{"every problem at once", groupsYAML(entry("views: v", "primary:", "  url: x"), group("Bad", ok...)),
			[]string{"powerdns.groups[0]", "name isn't set", "views: want a list", "primary.url:", "neither primary.api_key", "powerdns.groups[1] (Bad)"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := load(t, tt.yaml, nil)
			if err == nil {
				t.Fatal("Load succeeded")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error doesn't contain %q:\n%v", w, err)
				}
			}
			if strings.Contains(err.Error(), ": k\n") || strings.Contains(err.Error(), `"k"`) {
				t.Errorf("an API key is in the error:\n%v", err)
			}
		})
	}
}

func TestGroupsAreFileOnly(t *testing.T) {
	_, _, err := load(t, "", map[string]string{"NBPDNS_POWERDNS_GROUPS": "site-a"})
	if err == nil || !strings.Contains(err.Error(), "unknown environment variable NBPDNS_POWERDNS_GROUPS") {
		t.Errorf("Load: %v, want the variable to be unknown", err)
	}
}

func TestGroupFieldsAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range GroupFields() {
		switch {
		case seen[f.Name]:
			t.Errorf("%s is declared twice", f.Name)
		case f.Type == "" || f.Summary == "" || f.parse == nil || f.show == nil:
			t.Errorf("%s is missing its type, summary, parse or show", f.Name)
		case !strings.HasSuffix(f.Summary, "."):
			t.Errorf("%s's summary isn't a sentence", f.Name)
		}
		seen[f.Name] = true
	}
}

// TestSecretValueNotInErrors checks that a secret YAML reads as something
// other than a string is refused without its value in the error.
func TestSecretValueNotInErrors(t *testing.T) {
	for _, value := range []string{"8675309421", "0x7f3a9c11d2", "1e10", "123.456", "true"} {
		for _, yaml := range []string{
			"netbox:\n  token: " + value + "\n",
			groupsYAML(entry("name: a", "views: [v]", "primary:", "  url: https://pdns.example.com", "  api_key: "+value)),
		} {
			_, _, err := load(t, yaml, nil)
			if err == nil || !strings.Contains(err.Error(), "want a string") {
				t.Fatalf("%s: %v, want an error", value, err)
			}
			// "not true or false" doesn't say which one the value was.
			for _, leak := range []string{value, "8675309421", "546444153298", "1e+10", "123.456"} {
				if leak != "true" && strings.Contains(err.Error(), leak) {
					t.Errorf("the error shows the value %s:\n%v", leak, err)
				}
			}
		}
	}
}
