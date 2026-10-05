package netbox

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
)

// pluginAPI is where the DNS plugin's REST API lives, under NetBox's base URL.
const pluginAPI = "api/plugins/netbox-dns/"

// endpoints maps each object type nbpdns reads to its endpoint, under
// pluginAPI.
var endpoints = map[string]string{
	"netbox_dns.view":       "views/",
	"netbox_dns.zone":       "zones/",
	"netbox_dns.nameserver": "nameservers/",
	"netbox_dns.record":     "records/",
}

// ObjectTypes are the DNS plugin's object types that nbpdns reads, which its
// token's user must be allowed to view.
var ObjectTypes = []string{"netbox_dns.view", "netbox_dns.zone", "netbox_dns.nameserver", "netbox_dns.record"}

// A View is a DNS view in NetBox. Every zone is in one.
type View struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	DefaultView bool   `json:"default_view"`
}

// A Nameserver is a name server that NetBox lists for zones.
type Nameserver struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// A Zone is a DNS zone, as the plugin's API returns it.
type Zone struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	View View   `json:"view"`
	// Status is the zone's status, such as active or parked.
	Status string `json:"status"`
	// Active reports whether the plugin counts the status as active.
	Active      bool         `json:"active"`
	DefaultTTL  uint32       `json:"default_ttl"`
	SOASerial   uint32       `json:"soa_serial"`
	Nameservers []Nameserver `json:"nameservers"`
}

// A Record is one DNS record, as the plugin's API returns it.
type Record struct {
	ID int `json:"id"`
	// Name is the owner name, relative to the zone, or @ for its apex.
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
	// TTL is the record's own TTL, or nil for the zone's default.
	TTL *uint32 `json:"ttl"`
	// Status is active or inactive.
	Status string `json:"status"`
	// Active reports whether the record and its zone are both active.
	Active bool `json:"active"`
	// Managed marks a record that the plugin generates, such as SOA and NS.
	Managed bool `json:"managed"`
}

// page is one page of a NetBox list.
type page[T any] struct {
	Count   int    `json:"count"`
	Next    string `json:"next"`
	Results []T    `json:"results"`
}

// list reads every page of the list at path, under pluginAPI, following
// NetBox's next links.
func list[T any](ctx context.Context, c *Client, path string, query url.Values) ([]T, error) {
	if query == nil {
		query = url.Values{}
	}
	if c.pageSize > 0 {
		query.Set("limit", strconv.Itoa(c.pageSize))
	}
	u := c.endpoint(pluginAPI+path, query)
	var out []T
	for {
		var p page[T]
		if err := c.get(ctx, u, &p); err != nil {
			return nil, err
		}
		if out == nil {
			out = make([]T, 0, p.Count)
		}
		out = append(out, p.Results...)
		if p.Next == "" {
			return out, nil
		}
		next, err := c.rebase(p.Next)
		if err != nil {
			return nil, err
		}
		if next.String() == u.String() {
			return nil, fmt.Errorf("NetBox's next-page link for GET %s points back to the same page", u.RequestURI())
		}
		u = next
	}
}

// Views lists every view.
func (c *Client) Views(ctx context.Context) ([]View, error) {
	return list[View](ctx, c, endpoints["netbox_dns.view"], nil)
}

// Nameservers lists every name server.
func (c *Client) Nameservers(ctx context.Context) ([]Nameserver, error) {
	return list[Nameserver](ctx, c, endpoints["netbox_dns.nameserver"], nil)
}

// A ZoneFilter selects zones. Each field that isn't empty must match.
type ZoneFilter struct {
	// Name is the zone's name, as ZoneName returns it.
	Name string
	// View is the view's name.
	View string
	// Status is the zone's status, such as active.
	Status string
}

// Zones lists the zones that f selects.
func (c *Client) Zones(ctx context.Context, f ZoneFilter) ([]Zone, error) {
	q := url.Values{}
	for k, v := range map[string]string{"name": f.Name, "view": f.View, "status": f.Status} {
		if v != "" {
			q.Set(k, v)
		}
	}
	return list[Zone](ctx, c, endpoints["netbox_dns.zone"], q)
}

// Records lists every record in the zone with the ID zoneID.
func (c *Client) Records(ctx context.Context, zoneID int) ([]Record, error) {
	return list[Record](ctx, c, endpoints["netbox_dns.record"], url.Values{"zone_id": {strconv.Itoa(zoneID)}})
}

// Count returns how many objects of objectType, one of ObjectTypes, the
// token's user can view. It reads one object at most.
func (c *Client) Count(ctx context.Context, objectType string) (int, error) {
	path, ok := endpoints[objectType]
	if !ok {
		return 0, fmt.Errorf("nbpdns doesn't read %s objects", objectType)
	}
	var p page[struct{}]
	if err := c.get(ctx, c.endpoint(pluginAPI+path, url.Values{"limit": {"1"}}), &p); err != nil {
		return 0, err
	}
	return p.Count, nil
}

// ZoneName returns a zone name the way NetBox stores it: lowercase, without
// a final dot, and in its ASCII form, which for an internationalized name
// starts with xn--.
func ZoneName(name string) (string, error) {
	n := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
	for _, r := range n {
		if r > 0x7e || r < 0x21 {
			return "", fmt.Errorf("zone %q: give the name in its ASCII form, such as xn--bcher-kva.example for bücher.example", name)
		}
	}
	if n == "" {
		return "", errors.New("the zone name is empty")
	}
	return n, nil
}

// FindZone returns the zone named name, as ZoneName returns it, in the view
// named view. If view is empty, the zone may be in any view, but only one:
// otherwise FindZone returns an *AmbiguousZoneError.
func (c *Client) FindZone(ctx context.Context, name, view string) (Zone, error) {
	zones, err := c.Zones(ctx, ZoneFilter{Name: name, View: view})
	if err != nil {
		return Zone{}, err
	}
	switch len(zones) {
	case 0:
		if view != "" {
			return Zone{}, fmt.Errorf("NetBox has no zone %s in the view %s", name, view)
		}
		return Zone{}, fmt.Errorf("NetBox has no zone %s", name)
	case 1:
		return zones[0], nil
	}
	views := make([]string, len(zones))
	for i, z := range zones {
		views[i] = z.View.Name
	}
	slices.Sort(views)
	return Zone{}, &AmbiguousZoneError{Zone: name, Views: views}
}

// DNS returns z in the normalized model, without its records.
func (z Zone) DNS() dns.Zone {
	ns := make([]string, len(z.Nameservers))
	for i, n := range z.Nameservers {
		ns[i] = dns.Name(n.Name, ".")
	}
	slices.SortFunc(ns, dns.CompareNames)
	return dns.Zone{
		Name:        dns.Name(z.Name, "."),
		View:        z.View.Name,
		Status:      z.Status,
		Active:      z.Active,
		DefaultTTL:  z.DefaultTTL,
		SOASerial:   z.SOASerial,
		Nameservers: ns,
	}
}

// raw returns r as the normalized model's input.
func (r Record) raw() dns.RawRecord {
	return dns.RawRecord{
		Name: r.Name, Type: r.Type, Value: r.Value, TTL: r.TTL,
		Status: r.Status, Active: r.Active, Managed: r.Managed,
	}
}

// normalize returns z, with records, in the normalized model, and the
// problems normalization found in NetBox's data.
func normalize(z Zone, records []Record) (dns.Zone, []dns.Problem) {
	raw := make([]dns.RawRecord, len(records))
	for i, r := range records {
		raw[i] = r.raw()
	}
	out := z.DNS()
	return out, out.SetRecords(raw)
}

// ReadZones reads every record of each of zones, and returns the zones in
// the normalized model, in the order given, with the problems normalization
// found in NetBox's data. It reads one zone at a time per request in flight,
// with no more requests in flight than the client's concurrency. The first
// error stops it.
func (c *Client) ReadZones(ctx context.Context, zones []Zone) ([]dns.Zone, []dns.Problem, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := make([]dns.Zone, len(zones))
	probs := make([][]dns.Problem, len(zones))
	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
	)
	sem := make(chan struct{}, c.concurrency)
	for i, z := range zones {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
		wg.Go(func() {
			defer func() { <-sem }()
			records, err := c.Records(ctx, z.ID)
			if err != nil {
				once.Do(func() {
					firstErr = fmt.Errorf("reading zone %s in view %s: %w", z.Name, z.View.Name, err)
					cancel()
				})
				return
			}
			out[i], probs[i] = normalize(z, records)
		})
	}
	wg.Wait()
	if firstErr != nil {
		return nil, nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return out, slices.Concat(probs...), nil
}
