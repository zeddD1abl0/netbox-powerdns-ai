//go:build integration

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
)

// TestNetBoxCommands runs the netbox commands against each lab NetBox, as
// the least-privilege user an operator would set up.
func TestNetBoxCommands(t *testing.T) {
	for _, nb := range lab.NetBoxes {
		t.Run(nb.Name, func(t *testing.T) {
			f := lab.NewFixture(t, nb)
			env := func(token string) map[string]string {
				return map[string]string{"NBPDNS_NETBOX_URL": nb.URL(), "NBPDNS_NETBOX_TOKEN": token, "NBPDNS_NETBOX_PAGE_SIZE": "10"}
			}
			reader := env(f.ReaderToken)

			t.Run("check", func(t *testing.T) {
				code, out, stderr := run(t, reader, "netbox", "check", "-o", "json")
				var r checkReport
				if err := json.Unmarshal([]byte(out), &r); err != nil || code != exitOK {
					t.Fatalf("exit %d, %v: %s\n%s", code, err, out, stderr)
				}
				if !r.OK || !strings.HasPrefix(r.NetBoxVersion, nb.Version+".") || !strings.HasPrefix(r.PluginVersion, nb.PluginVersion+".") ||
					len(r.Checks) != 8 || r.Checks[0] != (checkResult{"connection", checkWarning, "http://, so the token crosses the network unencrypted"}) {
					t.Errorf("report: %+v", r)
				}
			})

			t.Run("check reads the token from a file", func(t *testing.T) {
				file := filepath.Join(t.TempDir(), "token")
				if err := os.WriteFile(file, []byte(f.ReaderToken+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				code, out, stderr := run(t, map[string]string{"NBPDNS_NETBOX_URL": nb.URL(), "NBPDNS_NETBOX_TOKEN_FILE": file}, "netbox", "check")
				if code != exitOK || !strings.Contains(out, "a v2 token, accepted") {
					t.Errorf("exit %d:\n%s\n%s", code, out, stderr)
				}
			})

			t.Run("check without permissions", func(t *testing.T) {
				code, out, stderr := run(t, env(f.NoAccessToken), "netbox", "check", "-o", "json")
				var r checkReport
				if err := json.Unmarshal([]byte(out), &r); err != nil || code != exitError {
					t.Fatalf("exit %d, %v: %s", code, err, out)
				}
				var failed []string
				for _, c := range r.Checks {
					if c.Result == checkFailed {
						failed = append(failed, c.Name)
						if !strings.Contains(c.Detail, "give it the view permission") {
							t.Errorf("%s: %s", c.Name, c.Detail)
						}
					}
				}
				if r.OK || !slices.Equal(failed, []string{"netbox_dns.view", "netbox_dns.zone", "netbox_dns.nameserver", "netbox_dns.record"}) {
					t.Errorf("failed checks %v in %+v", failed, r)
				}
				if !strings.Contains(stderr, "nbpdns: 4 of 8 checks failed") {
					t.Errorf("stderr:\n%s", stderr)
				}
			})

			t.Run("check with a bad token", func(t *testing.T) {
				code, out, stderr := run(t, env("nbt_nosuchkey.nosuchsecret"), "netbox", "check")
				if code != exitError || !strings.Contains(out, "NetBox rejected the token (Invalid v2 token)") ||
					!strings.Contains(stderr, "1 of 2 checks failed") {
					t.Errorf("exit %d:\n%s\n%s", code, out, stderr)
				}
			})

			t.Run("zones", func(t *testing.T) {
				code, out, stderr := run(t, reader, "netbox", "zones", "--view", f.View, "--status", "active", "-o", "json")
				var zones []dns.Zone
				if err := json.Unmarshal([]byte(out), &zones); err != nil || code != exitOK {
					t.Fatalf("exit %d, %v: %s\n%s", code, err, out, stderr)
				}
				if len(zones) != 1 || zones[0].Name != f.Zone+"." || zones[0].View != f.View ||
					!slices.Equal(zones[0].Nameservers, []string{f.Nameserver + "."}) || zones[0].RRsets != nil {
					t.Errorf("zones: %+v", zones)
				}
				code, out, _ = run(t, reader, "netbox", "zones")
				for _, view := range []string{f.View, f.OtherView} {
					want := view + " " + f.Zone + ". active"
					if code != exitOK || !slices.ContainsFunc(strings.Split(out, "\n"), func(line string) bool {
						return strings.HasPrefix(strings.Join(strings.Fields(line), " "), want)
					}) {
						t.Errorf("table has no row for %s in %s:\n%s", f.Zone, view, out)
					}
				}
			})

			t.Run("records", func(t *testing.T) {
				code, out, stderr := run(t, reader, "netbox", "records", "--zone", strings.ToUpper(f.Zone)+".", "--view", f.View, "-o", "json")
				var zr struct {
					dns.Zone
					Problems []dns.Problem `json:"problems"`
				}
				if err := json.Unmarshal([]byte(out), &zr); err != nil || code != exitOK {
					t.Fatalf("exit %d, %v: %s\n%s", code, err, out, stderr)
				}
				n := 0
				for _, s := range zr.RRsets {
					n += len(s.Records)
				}
				if zr.Name != f.Zone+"." || n != len(f.Records)+2 || zr.Problems == nil || len(zr.Problems) != 0 {
					t.Errorf("zone %s with %d records and problems %v", zr.Name, n, zr.Problems)
				}
				code, out, _ = run(t, reader, "netbox", "records", "--zone", f.Zone, "--view", f.View)
				for _, row := range []string{"www." + f.Zone + ".", "192.0.2.14", "inactive"} {
					if code != exitOK || !strings.Contains(out, row) {
						t.Errorf("table has no %q:\n%s", row, out)
					}
				}
			})

			t.Run("records of a zone in two views", func(t *testing.T) {
				code, _, stderr := run(t, reader, "netbox", "records", "--zone", f.Zone)
				want := "zone " + f.Zone + " is in more than one view: " + f.View + ", " + f.OtherView + "; name the view with --view"
				if code != exitError || !strings.Contains(stderr, want) {
					t.Errorf("exit %d, want %q in:\n%s", code, want, stderr)
				}
			})
		})
	}
}
