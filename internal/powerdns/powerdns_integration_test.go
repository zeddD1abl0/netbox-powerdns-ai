//go:build integration

package powerdns

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
	"strings"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
)

var record = flag.Bool("record", false, "record the lab's responses into testdata/, for the unit tests")

// labClient returns a client for p with key and serverID, logging to logs.
func labClient(t *testing.T, p lab.PowerDNS, key, serverID string, logs io.Writer, supported []string) *Client {
	t.Helper()
	c, err := New(t.Context(), Options{
		Group:   p.Group,
		Primary: config.Primary{URL: p.URL(), APIKey: config.NewSecret(key), ServerID: serverID},
		Timeout: 30 * time.Second, Concurrency: 4,
		Logger:    slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		supported: supported,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

// TestLab reads a fixture from each lab PowerDNS server, and checks the
// failures a real PowerDNS gives.
func TestLab(t *testing.T) {
	for _, p := range lab.PowerDNSes {
		t.Run(p.Name, func(t *testing.T) {
			t.Parallel()
			f := lab.NewPowerDNSFixture(t, p)

			t.Run("reader", func(t *testing.T) {
				var logs bytes.Buffer
				c := labClient(t, p, lab.PowerDNSAPIKey, "localhost", &logs, nil)
				s, err := c.Connect(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if err := c.Check(s); err != nil || !strings.HasPrefix(s.Version, p.Version+".") {
					t.Errorf("server %+v: %v", s, err)
				}
				zones, err := c.Zones(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				for _, name := range append([]string{f.Zone}, f.Others...) {
					if !slices.ContainsFunc(zones, func(z Zone) bool { return z.Name == name && z.Kind == "Native" && z.Serial != 0 }) {
						t.Errorf("no zone %s in %+v", name, zones)
					}
				}
				z, err := c.FindZone(t.Context(), strings.ToUpper(f.Zone))
				if err != nil || z.Name != f.Zone {
					t.Fatalf("FindZone: %+v, %v", z, err)
				}
				others := make([]Zone, len(f.Others))
				for i, name := range f.Others {
					if others[i], err = c.FindZone(t.Context(), name); err != nil {
						t.Fatal(err)
					}
				}
				read, probs, err := c.ReadZones(t.Context(), append([]Zone{z}, others...))
				if err != nil {
					t.Fatal(err)
				}
				checkFixtureZone(t, f, read[0], probs)
				for i, o := range read[1:] {
					if o.Name != f.Others[i] || len(o.RRsets) != 2 {
						t.Errorf("zone %d = %+v", i+1, o)
					}
				}
				if strings.Contains(logs.String(), lab.PowerDNSAPIKey) {
					t.Error("the API key is in the log")
				}
				if !strings.Contains(logs.String(), "crosses the network unencrypted") {
					t.Error("no warning about http://")
				}
			})

			t.Run("no such zone", func(t *testing.T) {
				c := labClient(t, p, lab.PowerDNSAPIKey, "localhost", io.Discard, nil)
				var zn *ZoneNotFoundError
				if _, err := c.FindZone(t.Context(), "missing-"+f.Zone); !errors.As(err, &zn) {
					t.Errorf("FindZone: %v, want a ZoneNotFoundError", err)
				}
				if _, err := c.ZoneData(t.Context(), Zone{ID: "missing-" + f.Zone, Name: "missing-" + f.Zone}); !errors.As(err, &zn) {
					t.Errorf("ZoneData: %v, want a ZoneNotFoundError", err)
				}
			})

			t.Run("a wrong key", func(t *testing.T) {
				c := labClient(t, p, "not-the-key", "localhost", io.Discard, nil)
				var ae *AuthError
				if _, err := c.Server(t.Context()); !errors.As(err, &ae) {
					t.Errorf("Server: %v, want an AuthError", err)
				}
				if _, err := c.Zones(t.Context()); !errors.As(err, &ae) {
					t.Errorf("Zones: %v, want an AuthError", err)
				}
			})

			t.Run("no such server", func(t *testing.T) {
				c := labClient(t, p, lab.PowerDNSAPIKey, "nosuchserver", io.Discard, nil)
				var sn *ServerNotFoundError
				if _, err := c.Connect(t.Context()); !errors.As(err, &sn) || sn.ServerID != "nosuchserver" {
					t.Errorf("Connect: %v, want a ServerNotFoundError", err)
				}
			})

			t.Run("unsupported release", func(t *testing.T) {
				var logs bytes.Buffer
				c := labClient(t, p, lab.PowerDNSAPIKey, "localhost", &logs, []string{"9.9"})
				if _, err := c.Connect(t.Context()); err != nil {
					t.Fatalf("an unsupported release only warns: %v", err)
				}
				if !strings.Contains(logs.String(), "isn't supported") || !strings.Contains(logs.String(), "nbpdns supports PowerDNS 9.9.x") {
					t.Errorf("no warning about the release:\n%s", logs.String())
				}
			})
		})
	}
}

// TestRecord records the lab's responses for a fixture with the fixed ID
// "rec" into testdata/<server>/, where the unit tests read them. Run it, on a
// lab with no other zones, after changing the fixture or the lab's
// releases:
//
//	go test -tags integration -run TestRecord ./internal/powerdns -record
func TestRecord(t *testing.T) {
	if !*record {
		t.Skip("run with -record to record the lab's responses")
	}
	for _, p := range lab.PowerDNSes {
		t.Run(p.Name, func(t *testing.T) {
			f := lab.CreatePowerDNSFixture(t, p, "rec")
			dir := filepath.Join("testdata", p.Name)
			if err := os.MkdirAll(dir, 0o750); err != nil {
				t.Fatal(err)
			}
			for name, path := range map[string]string{
				"server.json": "",
				"zones.json":  "zones?dnssec=false",
				"zone.json":   "zones/" + url.PathEscape(f.Zone),
			} {
				body := fetch(t, p, path)
				var out bytes.Buffer
				if err := json.Indent(&out, body, "", "  "); err != nil {
					t.Fatal(err)
				}
				out.WriteByte('\n')
				//nolint:gosec // Recorded responses are ordinary repository files.
				if err := os.WriteFile(filepath.Join(dir, name), out.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// fetch returns the body of a GET of path, under p's server.
func fetch(t *testing.T, p lab.PowerDNS, path string) []byte {
	t.Helper()
	u := p.URL() + "/api/v1/servers/localhost"
	if path != "" {
		u += "/" + path
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, u, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-API-Key", lab.PowerDNSAPIKey)
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
