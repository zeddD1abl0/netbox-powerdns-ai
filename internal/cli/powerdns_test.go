package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/netbox"
)

// configFile writes yaml to a config file and returns the environment that
// names it.
func configFile(t *testing.T, yaml string) map[string]string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nbpdns.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return map[string]string{"NBPDNS_CONFIG": path}
}

const twoGroups = `powerdns:
  groups:
    - name: site-a
      views: [_default_]
      primary: {url: "http://127.0.0.1:1", api_key: k}
    - name: site-b
      views: [_default_]
      primary: {url: "http://127.0.0.1:1", api_key: k}
`

func TestPowerDNSExitCodes(t *testing.T) {
	none := configFile(t, "log: {level: info}\n")
	two := configFile(t, twoGroups)
	tests := []struct {
		name       string
		env        map[string]string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{"stray argument to powerdns", nil, []string{"powerdns", "extra"}, exitUsage, "Run 'nbpdns --help'"},
		{"no groups", none, []string{"powerdns", "zones"}, exitError,
			"no PowerDNS server groups are declared; declare them under powerdns.groups in the config file"},
		{"an unknown group", two, []string{"powerdns", "check", "--group", "site-c"}, exitError,
			"no server group is named site-c; the groups are site-a, site-b"},
		{"records without a zone", two, []string{"powerdns", "records"}, exitUsage, "--zone is required"},
		{"records of a zone named in Unicode", two, []string{"powerdns", "records", "--zone", "bücher.example"}, exitUsage, "ASCII form"},
		{"records without a group, with two", two, []string{"powerdns", "records", "--zone", "example.com"}, exitError,
			"2 server groups are declared; name one with --group"},
		{"an unreachable primary", two, []string{"powerdns", "zones", "--group", "site-a"}, exitError, "PowerDNS at http://127.0.0.1:1/ isn't reachable"},
		{"netbox zones with --view and --group", two, []string{"netbox", "zones", "--view", "v", "--group", "site-a"}, exitUsage,
			"--view and --group can't be used together"},
		{"netbox zones with an unknown group", two, []string{"netbox", "zones", "--group", "site-c"}, exitError,
			"no server group is named site-c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := run(t, tt.env, tt.args...)
			if code != tt.wantCode || !strings.Contains(stderr, tt.wantStderr) {
				t.Errorf("exit %d, want %d, and %q in stderr:\n%s", code, tt.wantCode, tt.wantStderr, stderr)
			}
		})
	}
}

func TestPowerDNSCheckUnreachable(t *testing.T) {
	code, out, stderr := run(t, configFile(t, twoGroups), "powerdns", "check", "--group", "site-b")
	if code != exitError || !strings.Contains(stderr, "1 of 2 checks failed") || strings.Contains(out, "site-a") {
		t.Errorf("exit %d:\n%s\n%s", code, out, stderr)
	}
	for _, row := range []string{"site-b  connection  warning", "site-b  server      failed   PowerDNS at http://127.0.0.1:1/ isn't reachable"} {
		if !strings.Contains(out, row) {
			t.Errorf("no %q in:\n%s", row, out)
		}
	}
}

func TestWritePowerDNSRecords(t *testing.T) {
	zone := dns.Zone{Name: "example.com.", Active: true, RRsets: []dns.RRset{
		{Name: "www.example.com.", Type: "A", TTL: 300, Records: []dns.Record{
			{Value: "192.0.2.10", TTL: 300, Status: "active", Active: true},
			{Value: "192.0.2.11", TTL: 300, Status: "disabled"},
		}},
	}}
	var b bytes.Buffer
	if err := writePowerDNSRecords(&b, zone); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"NAME TTL TYPE VALUE STATUS",
		"www.example.com. 300 A 192.0.2.10 active",
		"www.example.com. 300 A 192.0.2.11 disabled",
	}
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	for i, line := range lines {
		if i >= len(want) || strings.Join(strings.Fields(line), " ") != want[i] {
			t.Errorf("row %d = %q", i, line)
		}
	}
}

func TestSharedNames(t *testing.T) {
	zone := func(name, view string) netbox.Zone { return netbox.Zone{Name: name, View: netbox.View{Name: view}} }
	probs := sharedNames("site-a", []netbox.Zone{
		zone("example.com", "internal"), zone("example.org", "internal"), zone("example.com", "external"),
	})
	if len(probs) != 1 || probs[0].Zone != "example.com." || probs[0].Detail != "server group site-a serves it from the views external, internal" {
		t.Errorf("problems = %+v", probs)
	}
}

// TestPowerDNSCheckGroupWithoutClient checks that a group nbpdns can't build
// a client for, such as one whose CA file is missing, fails its own check,
// and doesn't stop the other groups being checked.
func TestPowerDNSCheckGroupWithoutClient(t *testing.T) {
	env := configFile(t, `powerdns:
  groups:
    - name: site-a
      views: [_default_]
      primary: {url: "https://127.0.0.1:1", api_key: k, ca_file: /does/not/exist.pem}
    - name: site-b
      views: [_default_]
      primary: {url: "https://127.0.0.1:1", api_key: k, ca_file: /does/not/exist/either.pem}
`)
	code, out, stderr := run(t, env, "powerdns", "check")
	if code != exitError || !strings.Contains(stderr, "2 of 2 checks failed") {
		t.Errorf("exit %d:\n%s\n%s", code, out, stderr)
	}
	for _, row := range []string{
		"site-a  connection  failed  powerdns.groups.site-a.primary.ca_file:",
		"site-b  connection  failed  powerdns.groups.site-b.primary.ca_file:",
	} {
		if !strings.Contains(out, row) {
			t.Errorf("no %q in:\n%s", row, out)
		}
	}
}
