package lab

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// A Fixture is DNS data created in a lab NetBox for one test, and the users
// that read it back. Every name in it contains its ID, so fixtures of
// concurrent tests don't collide.
type Fixture struct {
	ID string
	// View and OtherView each hold a zone named Zone.
	View, OtherView string
	// Zone is the name of both zones, such as t1a2b3c4d.nbpdns.example. The
	// one in View holds Records; the one in OtherView holds only the
	// records the plugin manages.
	Zone string
	// Nameserver is both zones' name server, outside the zones.
	Nameserver string
	// DefaultTTL is both zones' default TTL.
	DefaultTTL uint32
	// Records are the records in Zone in View, besides the SOA and NS
	// records that the plugin manages.
	Records []FixtureRecord
	// ReaderToken is the v2 token of a user who can only view the DNS
	// plugin's objects: nbpdns's least privilege.
	ReaderToken string
	// NoAccessToken is the v2 token of a user with no permissions.
	NoAccessToken string
}

// A FixtureRecord is a record as the fixture creates it in NetBox.
type FixtureRecord struct {
	Name, Type, Value string
	// TTL is the record's own TTL, or 0 for the zone's default.
	TTL uint32
	// Status is active, unless it's inactive.
	Status string
}

// DescribeFixture returns the fixture with the ID id, without its tokens,
// and without creating anything.
func DescribeFixture(id string) *Fixture {
	f := &Fixture{
		ID:         id,
		View:       "nbpdns-" + id,
		OtherView:  "nbpdns-" + id + "-other",
		Zone:       id + ".nbpdns.example",
		Nameserver: "ns1-" + id + ".nbpdns.example",
		DefaultTTL: 3600,
		Records: []FixtureRecord{
			// The plugin keeps an RRset's active records on one TTL, so the
			// second takes the first's. The inactive record keeps its own,
			// which it can only have if it's created first.
			{Name: "www", Type: "A", Value: "192.0.2.14", TTL: 60, Status: "inactive"},
			{Name: "www", Type: "A", Value: "192.0.2.10", TTL: 300},
			{Name: "www", Type: "A", Value: "192.0.2.11"},
			{Name: "www", Type: "AAAA", Value: "2001:DB8::10"},
			{Name: "Mixed", Type: "A", Value: "192.0.2.12", Status: "inactive"},
			{Name: "alias", Type: "CNAME", Value: "www"},
			{Name: "@", Type: "MX", Value: "10 mail"},
			{Name: "_sip._tcp", Type: "SRV", Value: "10 5 5060 sip"},
			{Name: "@", Type: "TXT", Value: "v=spf1 -all"},
			{Name: "quoted", Type: "TXT", Value: `"quoted" "two"`},
			{Name: "long", Type: "TXT", Value: strings.Repeat("a", 300)},
			{Name: "bücher", Type: "A", Value: "192.0.2.13"},
			{Name: "caa", Type: "CAA", Value: `0 issue "letsencrypt.org"`},
		},
	}
	// Enough records for several pages at a small page size.
	for i := range 24 {
		f.Records = append(f.Records, FixtureRecord{
			Name: fmt.Sprintf("host-%02d", i), Type: "A", Value: fmt.Sprintf("198.51.100.%d", i+1),
		})
	}
	return f
}

// NewFixture creates a fixture with a random ID in nb, and removes it when
// the test ends.
func NewFixture(t testing.TB, nb NetBox) *Fixture {
	t.Helper()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return CreateFixture(t, nb, "t"+hex.EncodeToString(b))
}

// CreateFixture creates the fixture with the ID id in nb, and removes it
// when the test ends.
func CreateFixture(t testing.TB, nb NetBox, id string) *Fixture {
	t.Helper()
	f := DescribeFixture(id)
	a := &adminClient{t: t, nb: nb}

	ns := a.create("plugins/netbox-dns/nameservers/", map[string]any{"name": f.Nameserver})
	view := a.create("plugins/netbox-dns/views/", map[string]any{"name": f.View})
	other := a.create("plugins/netbox-dns/views/", map[string]any{"name": f.OtherView})
	zoneFields := func(viewID int) map[string]any {
		return map[string]any{
			"name": f.Zone, "view": viewID, "status": "active", "default_ttl": f.DefaultTTL,
			"nameservers": []int{ns}, "soa_mname": ns, "soa_rname": "hostmaster." + f.Zone,
		}
	}
	zone := a.create("plugins/netbox-dns/zones/", zoneFields(view))
	a.create("plugins/netbox-dns/zones/", zoneFields(other))

	a.records(zone, f.Records)

	f.ReaderToken = a.user("nbpdns-reader-"+id, true)
	f.NoAccessToken = a.user("nbpdns-noaccess-"+id, false)
	return f
}

// AdminDo sends a request to path, under nb's API, as its admin, and decodes
// the answer into out, if it isn't nil. It fails t on any error.
func AdminDo(t testing.TB, nb NetBox, method, path string, body, out any) {
	t.Helper()
	(&adminClient{t: t, nb: nb}).do(method, path, body, out)
}

// AdminCreate creates an object at path, under nb's API, as its admin,
// returns its ID, and deletes it when the test ends.
func AdminCreate(t testing.TB, nb NetBox, path string, fields any) int {
	t.Helper()
	return (&adminClient{t: t, nb: nb}).create(path, fields)
}

// adminClient calls a lab NetBox's API as its admin, failing the test on any
// error.
type adminClient struct {
	t  testing.TB
	nb NetBox
}

// create creates an object at path, returns its ID, and deletes it when the
// test ends.
func (a *adminClient) create(path string, fields any) int {
	a.t.Helper()
	var obj struct {
		ID int `json:"id"`
	}
	a.do(http.MethodPost, path, fields, &obj)
	a.t.Cleanup(func() { a.do(http.MethodDelete, fmt.Sprintf("%s%d/", path, obj.ID), nil, nil) })
	return obj.ID
}

// records creates records in the zone with the ID zone, in one bulk create.
// Deleting the zone deletes them.
func (a *adminClient) records(zone int, records []FixtureRecord) {
	a.t.Helper()
	if len(records) == 0 {
		return
	}
	fields := make([]map[string]any, len(records))
	for i, r := range records {
		rec := map[string]any{"zone": zone, "name": r.Name, "type": r.Type, "value": r.Value, "status": "active"}
		if r.TTL != 0 {
			rec["ttl"] = r.TTL
		}
		if r.Status != "" {
			rec["status"] = r.Status
		}
		fields[i] = rec
	}
	a.do(http.MethodPost, "plugins/netbox-dns/records/", fields, nil)
}

// user creates a user, and gives it the view permission on the DNS
// plugin's objects if canView. It returns a new v2 token for the user.
func (a *adminClient) user(name string, canView bool) string {
	a.t.Helper()
	password := make([]byte, 16)
	if _, err := rand.Read(password); err != nil {
		a.t.Fatal(err)
	}
	// NetBox's password rules want an uppercase letter and a digit.
	user := a.create("users/users/", map[string]any{"username": name, "password": "Nbpdns1-" + hex.EncodeToString(password)})
	if canView {
		a.create("users/permissions/", map[string]any{
			"name":         name,
			"object_types": []string{"netbox_dns.view", "netbox_dns.zone", "netbox_dns.nameserver", "netbox_dns.record"},
			"actions":      []string{"view"},
			"users":        []int{user},
		})
	}
	// NetBox shows a v2 token's secret once, on creation, without its
	// nbt_<key>. prefix. Deleting the user deletes the token.
	var token struct {
		Key   string `json:"key"`
		Token string `json:"token"`
	}
	a.do(http.MethodPost, "users/tokens/", map[string]any{"user": user, "description": "nbpdns test", "write_enabled": false}, &token)
	return "nbt_" + token.Key + "." + token.Token
}

// do sends a request to path, under the API, and decodes the answer into
// out, if it isn't nil.
func (a *adminClient) do(method, path string, body, out any) {
	a.t.Helper()
	// A cleanup runs after the test's context is canceled.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(a.t.Context()), time.Minute)
	defer cancel()
	var r io.Reader = http.NoBody
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			a.t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.nb.URL()+"/api/"+path, r)
	if err != nil {
		a.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+AdminToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatalf("%s %s: %v (is the lab up? make lab-up)", method, path, err)
	}
	defer resp.Body.Close()
	answer, err := io.ReadAll(resp.Body)
	if err != nil {
		a.t.Fatal(err)
	}
	if resp.StatusCode/100 != 2 {
		a.t.Fatalf("%s %s on %s: %s: %s", method, path, a.nb.Name, resp.Status, answer)
	}
	if out != nil {
		if err := json.Unmarshal(answer, out); err != nil {
			a.t.Fatalf("%s %s: %v", method, path, err)
		}
	}
}
