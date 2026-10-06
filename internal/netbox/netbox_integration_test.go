//go:build integration

package netbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
)

var record = flag.Bool("record", false, "record the lab's responses into testdata/, for the unit tests")

// labClient returns a client for nb with token, logging to logs, with a
// small page size so that a fixture zone spans several pages.
func labClient(t *testing.T, nb lab.NetBox, token string, logs io.Writer, supported []Release) *Client {
	t.Helper()
	c, err := New(t.Context(), Options{
		URL: nb.URL(), Token: config.NewSecret(token), Timeout: 30 * time.Second,
		PageSize: 10, Concurrency: 4,
		Logger:    slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		supported: supported,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

// TestLab reads a fixture from each lab NetBox with a least-privilege v2
// token, and checks the failures a real NetBox gives.
func TestLab(t *testing.T) {
	for _, nb := range lab.NetBoxes {
		t.Run(nb.Name, func(t *testing.T) {
			t.Parallel()
			f := lab.NewFixture(t, nb)

			t.Run("reader", func(t *testing.T) {
				var logs bytes.Buffer
				c := labClient(t, nb, f.ReaderToken, &logs, nil)
				st, err := c.Connect(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if err := st.Check(); err != nil || !strings.HasPrefix(st.NetBoxVersion, nb.Version+".") {
					t.Errorf("status %+v: %v", st, err)
				}

				views, err := c.Views(t.Context())
				if err != nil || !slices.ContainsFunc(views, func(v View) bool { return v.Name == f.View }) {
					t.Errorf("views %+v: %v", views, err)
				}
				servers, err := c.Nameservers(t.Context())
				if err != nil || !slices.ContainsFunc(servers, func(n Nameserver) bool { return n.Name == f.Nameserver }) {
					t.Errorf("name servers %+v: %v", servers, err)
				}
				for _, typ := range ObjectTypes {
					if n, err := c.Count(t.Context(), typ); err != nil || n == 0 {
						t.Errorf("Count(%s) = %d, %v", typ, n, err)
					}
				}

				zones, err := c.Zones(t.Context(), ZoneFilter{Views: []string{f.View}, Status: "active"})
				if err != nil || len(zones) != 1 || zones[0].Name != f.Zone {
					t.Fatalf("zones in %s: %+v, %v", f.View, zones, err)
				}
				read, probs, err := c.ReadZones(t.Context(), zones)
				if err != nil {
					t.Fatal(err)
				}
				checkFixtureZone(t, f, read[0], probs)
				// 39 records at 10 a page.
				pages := 0
				for line := range strings.Lines(logs.String()) {
					if strings.Contains(line, `msg="http request" service=NetBox`) && strings.Contains(line, "/netbox-dns/records/?") && strings.Contains(line, "zone_id=") {
						pages++
					}
				}
				if pages != 4 {
					t.Errorf("read the records in %d pages, want 4:\n%s", pages, logs.String())
				}
				if strings.Contains(logs.String(), f.ReaderToken) {
					t.Error("the token is in the log")
				}
				if !strings.Contains(logs.String(), "crosses the network unencrypted") {
					t.Error("no warning about the lab's http:// URL")
				}
			})

			t.Run("find zone", func(t *testing.T) {
				c := labClient(t, nb, f.ReaderToken, io.Discard, nil)
				z, err := c.FindZone(t.Context(), f.Zone, f.OtherView)
				if err != nil || z.View.Name != f.OtherView {
					t.Errorf("FindZone in %s = %+v, %v", f.OtherView, z, err)
				}
				var ae *AmbiguousZoneError
				_, err = c.FindZone(t.Context(), f.Zone, "")
				if !errors.As(err, &ae) || !slices.Equal(ae.Views, []string{f.View, f.OtherView}) {
					t.Errorf("FindZone in any view: %v, want an AmbiguousZoneError naming both views", err)
				}
				if _, err = c.FindZone(t.Context(), "nowhere."+f.Zone, ""); err == nil || !strings.Contains(err.Error(), "no zone") {
					t.Errorf("FindZone of a missing zone: %v", err)
				}
				var api *APIError
				if _, err = c.Zones(t.Context(), ZoneFilter{Views: []string{"no-such-view-" + f.ID}}); !errors.As(err, &api) || api.Status != 400 ||
					!strings.Contains(api.Detail, "view") {
					t.Errorf("zones in a missing view: %v", err)
				}
			})

			t.Run("no permission", func(t *testing.T) {
				c := labClient(t, nb, f.NoAccessToken, io.Discard, nil)
				if _, err := c.Connect(t.Context()); err != nil {
					t.Fatalf("Connect needs only a valid token: %v", err)
				}
				for _, typ := range ObjectTypes {
					var pe *PermissionError
					if _, err := c.Count(t.Context(), typ); !errors.As(err, &pe) || pe.ObjectType != typ {
						t.Errorf("Count(%s): %v, want a PermissionError", typ, err)
					}
				}
				var pe *PermissionError
				if _, err := c.Zones(t.Context(), ZoneFilter{}); !errors.As(err, &pe) || pe.ObjectType != "netbox_dns.zone" ||
					!strings.Contains(pe.Detail, "permission") {
					t.Errorf("Zones: %v, want a PermissionError for netbox_dns.zone, with NetBox's detail", err)
				}
			})

			t.Run("bad token", func(t *testing.T) {
				for _, token := range []string{"nbt_nosuchkey.nosuchsecret", strings.Repeat("0", 40)} {
					c := labClient(t, nb, token, io.Discard, nil)
					var ae *AuthError
					if _, err := c.Connect(t.Context()); !errors.As(err, &ae) {
						t.Errorf("Connect with %s: %v, want an AuthError", token[:4], err)
					}
					if _, err := c.Views(t.Context()); !errors.As(err, &ae) {
						t.Errorf("Views with %s: %v, want an AuthError", token[:4], err)
					}
				}
			})

			t.Run("unsupported release", func(t *testing.T) {
				var logs bytes.Buffer
				c := labClient(t, nb, f.ReaderToken, &logs, []Release{{NetBox: "9.9", Plugin: "9.9"}})
				st, err := c.Connect(t.Context())
				if err != nil {
					t.Fatalf("an unsupported release only warns: %v", err)
				}
				var ve *VersionError
				if err := st.check(c.supported); !errors.As(err, &ve) || ve.Plugin != st.PluginVersion() {
					t.Errorf("check: %v", err)
				}
				if !strings.Contains(logs.String(), "isn't supported") || !strings.Contains(logs.String(), "nbpdns supports NetBox 9.9.x") {
					t.Errorf("no warning about the release:\n%s", logs.String())
				}
				if _, err := c.Views(t.Context()); err != nil {
					t.Errorf("reading after the warning: %v", err)
				}
			})
		})
	}
}

// TestRecord records the lab's responses for a fixture with the fixed ID
// "rec" into testdata/<NetBox>/, where the unit tests read them. Run it
// after changing the fixture or the lab's releases:
//
//	go test -tags integration -run TestRecord ./internal/netbox -record
func TestRecord(t *testing.T) {
	if !*record {
		t.Skip("run with -record to record the lab's responses")
	}
	for _, nb := range lab.NetBoxes {
		t.Run(nb.Name, func(t *testing.T) {
			f := lab.CreateFixture(t, nb, "rec")
			var zones page[Zone]
			files := []struct {
				name, path string
				query      url.Values
			}{
				{"status.json", "api/status/", nil},
				{"zones.json", pluginAPI + "zones/", url.Values{"view": {f.View}}},
				{"records.json", pluginAPI + "records/", url.Values{"limit": {"1000"}}},
			}
			dir := filepath.Join("testdata", nb.Name)
			if err := os.MkdirAll(dir, 0o750); err != nil {
				t.Fatal(err)
			}
			for _, file := range files {
				if file.name == "records.json" {
					file.query.Set("zone_id", strconv.Itoa(zones.Results[0].ID))
				}
				body := fetch(t, nb, f.ReaderToken, file.path, file.query)
				if file.name == "zones.json" {
					if err := json.Unmarshal(body, &zones); err != nil || len(zones.Results) != 1 {
						t.Fatalf("zones: %v", err)
					}
				}
				var out bytes.Buffer
				if err := json.Indent(&out, body, "", "  "); err != nil {
					t.Fatal(err)
				}
				out.WriteByte('\n')
				//nolint:gosec // Recorded responses are ordinary repository files.
				if err := os.WriteFile(filepath.Join(dir, file.name), out.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// fetch returns the body of a GET of path, under nb's URL, with token.
func fetch(t *testing.T, nb lab.NetBox, token, path string, query url.Values) []byte {
	t.Helper()
	u := nb.URL() + "/" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, u, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %s, %v: %s", u, resp.Status, err, body)
	}
	return body
}
