package powerdns

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"sync"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
)

// A Zone is a zone in the API's list of zones, without its RRsets.
type Zone struct {
	// ID is the zone's ID in the API's paths, usually its name.
	ID   string `json:"id"`
	Name string `json:"name"`
	// Kind is Native, Master, Slave, Producer or Consumer.
	Kind   string `json:"kind"`
	Serial uint32 `json:"serial"`
	// Catalog is the catalog zone the zone is a member of, if any.
	Catalog string `json:"catalog"`
}

// A ZoneData is a zone with its RRsets.
type ZoneData struct {
	Zone
	RRsets []RRset `json:"rrsets"`
}

// An RRset is a zone's records with one owner name and type.
type RRset struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	TTL     uint32   `json:"ttl"`
	Records []Record `json:"records"`
}

// A Record is one value of an RRset.
type Record struct {
	Content string `json:"content"`
	// Disabled marks a record that PowerDNS keeps but doesn't serve.
	Disabled bool `json:"disabled"`
}

// Zones lists every zone on the server.
func (c *Client) Zones(ctx context.Context) ([]Zone, error) {
	var zones []Zone
	if err := c.get(ctx, c.endpoint("zones", url.Values{"dnssec": {"false"}}), &zones, nil); err != nil {
		return nil, err
	}
	return zones, nil
}

// FindZone returns the zone named name, an absolute name such as
// example.com., or a *ZoneNotFoundError.
func (c *Client) FindZone(ctx context.Context, name string) (Zone, error) {
	var zones []Zone
	name = dns.Name(name, ".")
	if err := c.get(ctx, c.endpoint("zones", url.Values{"zone": {name}, "dnssec": {"false"}}), &zones, nil); err != nil {
		return Zone{}, err
	}
	for _, z := range zones {
		if dns.Name(z.Name, ".") == name {
			return z, nil
		}
	}
	return Zone{}, &ZoneNotFoundError{Group: c.group, Zone: name}
}

// ZoneData reads z with its RRsets, disabled records included.
func (c *Client) ZoneData(ctx context.Context, z Zone) (*ZoneData, error) {
	var data ZoneData
	notFound := func() error { return &ZoneNotFoundError{Group: c.group, Zone: z.Name} }
	if err := c.get(ctx, c.url.JoinPath("zones", z.ID), &data, notFound); err != nil {
		return nil, err
	}
	return &data, nil
}

// ReadZones reads every RRset of each of zones, and returns the zones in the
// normalized model, in the order given, with the problems normalization
// found. It reads one zone per request in flight, with no more requests in
// flight than the client's concurrency. The first error stops it.
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
			data, err := c.ZoneData(ctx, z)
			if err != nil {
				once.Do(func() {
					firstErr = fmt.Errorf("reading zone %s in server group %s: %w", z.Name, c.group, err)
					cancel()
				})
				return
			}
			out[i], probs[i] = data.DNS()
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

// DNS returns z in the normalized model, without its RRsets: a PowerDNS zone
// has no view or default TTL, and is always active.
func (z Zone) DNS() dns.Zone {
	return dns.Zone{Name: dns.Name(z.Name, "."), Active: true, SOASerial: z.Serial, Nameservers: []string{}}
}

// DNS returns d in the normalized model, and the problems normalization
// found. Its name servers are the values of its apex NS RRset.
func (d *ZoneData) DNS() (dns.Zone, []dns.Problem) {
	z := d.Zone.DNS()
	var raw []dns.RawRecord
	for _, s := range d.RRsets {
		for _, r := range s.Records {
			status := "active"
			if r.Disabled {
				status = "disabled"
			}
			raw = append(raw, dns.RawRecord{
				Name: s.Name, Type: s.Type, Value: r.Content, TTL: &s.TTL, Status: status, Active: !r.Disabled,
			})
		}
	}
	probs := z.SetRecords(raw)
	for _, s := range z.RRsets {
		if s.Type == "NS" && s.Name == z.Name {
			for _, r := range s.Records {
				if r.Active {
					z.Nameservers = append(z.Nameservers, r.Value)
				}
			}
		}
	}
	slices.SortFunc(z.Nameservers, dns.CompareNames)
	return z, probs
}
