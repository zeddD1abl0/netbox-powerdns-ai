package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
)

func TestDriftResult(t *testing.T) {
	ok := drift.GroupReport{Group: "a", Status: drift.StatusOK}
	drifted := drift.GroupReport{Group: "b", Status: drift.StatusOK, Counts: drift.Counts{Drift: 1, Missing: 1, Inactive: 1}}
	failed := drift.GroupReport{Group: "c", Status: drift.StatusFailed, Error: "down"}
	tests := []struct {
		name   string
		report drift.Report
		want   int
		msg    string
	}{
		{"no drift", drift.Report{Complete: true, Groups: []drift.GroupReport{ok}}, exitOK, ""},
		{"drift", drift.Report{Complete: true, Drift: true, Groups: []drift.GroupReport{ok, drifted}}, exitDrift, "3 zones drifted"},
		{"a failed group wins over drift", drift.Report{Drift: true, Groups: []drift.GroupReport{drifted, failed}}, exitError,
			"1 of 2 server groups couldn't be read"},
	}
	for _, tt := range tests {
		err := driftResult(tt.report)
		if code := exitCode(err); code != tt.want || (err != nil && !strings.Contains(err.Error(), tt.msg)) {
			t.Errorf("%s: %v (exit %d), want exit %d with %q", tt.name, err, exitCode(err), tt.want, tt.msg)
		}
	}
}

func TestWriteDrift(t *testing.T) {
	r := drift.Report{Groups: []drift.GroupReport{
		{Group: "site-a", Status: drift.StatusOK, Counts: drift.Counts{InSync: 1, Drift: 1, Missing: 1, Ignored: 1, Unmanaged: 1},
			Zones: []drift.ZoneReport{
				{Zone: "a.example.", State: drift.StateInSync},
				{Zone: "b.example.", State: drift.StateDrift, Policy: "enforce", Changes: []drift.Change{
					{Name: "www.b.example.", Type: "A", Kind: drift.ChangeChanged,
						NetBox: &drift.Side{TTL: 300, Values: []string{"192.0.2.1", "192.0.2.2"}}, PowerDNS: &drift.Side{TTL: 600, Values: []string{"192.0.2.1"}}},
					{Name: "old.b.example.", Type: "TXT", Kind: drift.ChangeExtra, PowerDNS: &drift.Side{TTL: 60, Values: []string{`"x"`}}},
				}},
				{Zone: "c.example.", State: drift.StateMissing, Policy: "report"},
				{Zone: "d.example.", State: drift.StateIgnored},
			},
			Unmanaged: []string{"z.example."},
			Warnings:  []string{"zone_policies names gone.example., which isn't in any of the group's NetBox views"},
			Problems:  []dns.Problem{{Zone: "b.example.", Name: "bad.b.example.", Type: "A", Detail: "kept as given"}}},
		{Group: "site-b", Status: drift.StatusFailed, Error: "PowerDNS at http://x/ isn't reachable"},
	}}
	var b bytes.Buffer
	if err := writeDrift(&b, r); err != nil {
		t.Fatal(err)
	}
	out := oneSpace(b.String())
	for _, want := range []string{
		"site-a  ok      1        1      1        0         1        1",
		"site-b  failed  0        0      0        0         0        0",
		"\nDrift:\n",
		"GROUP ZONE POLICY CHANGE NAME TYPE NETBOX POWERDNS",
		"site-a b.example. enforce (from M13) changed www.b.example. A 300 192.0.2.1, 192.0.2.2 600 192.0.2.1",
		"site-a b.example. enforce (from M13) extra old.b.example. TXT -",
		"site-a c.example. report zone missing on the primary",
		"\nUnmanaged zones, on a primary but not in its group's NetBox views:\n",
		"site-a  z.example.",
		"\nIgnored zones, not compared:\n",
		"site-a  d.example.",
		"\nGroups that couldn't be read:\n",
		"site-b  PowerDNS at http://x/ isn't reachable",
		"\nProblems in the data, worked around:\n",
		"bad.b.example. A  kept as given",
		"\nWarnings about the configuration or NetBox's zones:\n",
		"site-a  zone_policies names gone.example., which isn't in any of the group's NetBox views",
	} {
		if !strings.Contains(out, oneSpace(want)) {
			t.Errorf("no %q in:\n%s", want, b.String())
		}
	}
	// A report with nothing to say past its summary has no sections.
	b.Reset()
	if err := writeDrift(&b, drift.Report{Groups: []drift.GroupReport{{Group: "site-a", Status: drift.StatusOK}}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), ":\n") {
		t.Errorf("sections in an empty report:\n%s", b.String())
	}
}

// oneSpace returns s with the spaces between the words of each line made
// one. Table columns are aligned to their widest value, so tests compare
// rows this way.
func oneSpace(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.Join(strings.Fields(l), " ")
	}
	return strings.Join(lines, "\n")
}

func TestDriftExitCodes(t *testing.T) {
	none := configFile(t, "log: {level: info}\n")
	two := configFile(t, twoGroups)
	tests := []struct {
		name       string
		env        map[string]string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{"no groups", none, []string{"drift"}, exitError, "no PowerDNS server groups are declared"},
		{"an unknown group", two, []string{"drift", "--group", "site-c"}, exitError, "no server group is named site-c"},
		{"a zone named in Unicode", two, []string{"drift", "--zone", "bücher.example"}, exitUsage, "ASCII form"},
		{"a stray argument", two, []string{"drift", "extra"}, exitUsage, "Run 'nbpdns --help'"},
		{"NetBox isn't configured", two, []string{"drift"}, exitError, "netbox.url isn't set"},
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
