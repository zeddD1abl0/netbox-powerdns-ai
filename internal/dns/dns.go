// Package dns is nbpdns's normalized model of DNS data: zones and their
// RRsets. NetBox's data is read into it (M01), and PowerDNS's (M02), so that
// M03 can compare the two RRset by RRset. The rules come from ADR-0023 and
// ADR-0025:
//
//   - Names are lowercase and absolute, ending with a dot.
//   - Values are parsed as their type with miekg/dns and printed in its
//     canonical text, with every name inside them made absolute against the
//     zone and lowercase, and hex uppercase. A number too big for its field
//     is an error, not a smaller number.
//   - TXT and SPF values are canonical: each string in double quotes, with "
//     and \ escaped, strings separated by one space, none longer than 255
//     bytes.
//   - A record's effective TTL is its own, or else its zone's default.
//   - An RRset is every record with one owner name and type. Its TTL is the
//     lowest effective TTL of its active records, or of all its records if
//     none is active.
//   - Each value keeps its record's status and managed flag.
package dns

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// A Zone is a DNS zone and its RRsets.
type Zone struct {
	// Name is the zone's absolute name, such as example.com.
	Name string `json:"name"`
	// View is the NetBox DNS view the zone is in. PowerDNS zones have none.
	View string `json:"view,omitempty"`
	// Status is the zone's status in NetBox, such as active. PowerDNS zones
	// have none.
	Status string `json:"status,omitempty"`
	// Active reports whether the zone's status is one that's published.
	Active bool `json:"active"`
	// DefaultTTL is the TTL of NetBox records that set none. PowerDNS zones
	// have none.
	DefaultTTL uint32 `json:"default_ttl,omitempty"`
	// SOASerial is the zone's SOA serial number.
	SOASerial uint32 `json:"soa_serial"`
	// Nameservers are the zone's name servers, as absolute names.
	Nameservers []string `json:"nameservers"`
	// RRsets are the zone's RRsets, in canonical order.
	RRsets []RRset `json:"rrsets,omitempty"`
}

// An RRset is every record in a zone with one owner name and type.
type RRset struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	TTL     uint32   `json:"ttl"`
	Records []Record `json:"records"`
}

// A Record is one value in an RRset.
type Record struct {
	// Value is the record's data, normalized.
	Value string `json:"value"`
	// TTL is the record's effective TTL.
	TTL uint32 `json:"ttl"`
	// Status is the record's status in its source, such as active.
	Status string `json:"status"`
	// Active reports whether the record is published: its status is active
	// and so is its zone's.
	Active bool `json:"active"`
	// Managed marks a record that the source generates itself, such as
	// NetBox's SOA and NS records.
	Managed bool `json:"managed"`
}

// A RawRecord is a record as its source holds it, before normalization.
type RawRecord struct {
	// Name is the owner: absolute, relative to the zone, or @ for the apex.
	Name  string
	Type  string
	Value string
	// TTL is the record's own TTL, or nil for its zone's default.
	TTL     *uint32
	Status  string
	Active  bool
	Managed bool
}

// A Problem is something wrong with the source data that normalization
// worked around. Fix it at the source.
type Problem struct {
	Zone   string `json:"zone"`
	Name   string `json:"name,omitempty"`
	Type   string `json:"type,omitempty"`
	Detail string `json:"detail"`
}

func (p Problem) String() string {
	return strings.TrimSpace(fmt.Sprintf("%s %s %s: %s", p.Zone, p.Name, p.Type, p.Detail))
}

// SetRecords normalizes raw into z's RRsets, replacing any it had, and
// returns the problems it found. z.Name and z.DefaultTTL must be set.
func (z *Zone) SetRecords(raw []RawRecord) []Problem {
	type key struct{ name, typ string }
	sets := map[key]*RRset{}
	var probs []Problem
	for _, r := range raw {
		name := Name(r.Name, z.Name)
		typ := strings.ToUpper(strings.TrimSpace(r.Type))
		value, err := Value(typ, r.Value, z.Name)
		if err != nil {
			probs = append(probs, Problem{z.Name, name, typ, err.Error() + "; kept as given"})
		}
		ttl := z.DefaultTTL
		if r.TTL != nil {
			ttl = *r.TTL
		}
		k := key{name, typ}
		s, ok := sets[k]
		if !ok {
			s = &RRset{Name: name, Type: typ}
			sets[k] = s
		}
		s.Records = append(s.Records, Record{Value: value, TTL: ttl, Status: r.Status, Active: r.Active, Managed: r.Managed})
	}

	z.RRsets = make([]RRset, 0, len(sets))
	for _, s := range sets {
		s.TTL, probs = rrsetTTL(z.Name, s, probs)
		slices.SortFunc(s.Records, func(a, b Record) int {
			return cmp.Or(strings.Compare(a.Value, b.Value), strings.Compare(a.Status, b.Status), cmp.Compare(a.TTL, b.TTL))
		})
		z.RRsets = append(z.RRsets, *s)
	}
	slices.SortFunc(z.RRsets, func(a, b RRset) int { return CompareRRsets(a.Name, a.Type, b.Name, b.Type) })
	slices.SortFunc(probs, func(a, b Problem) int { return strings.Compare(a.String(), b.String()) })
	return probs
}

// rrsetTTL returns the lowest effective TTL of s's active records, or of all
// of them if none is active. Active records that disagree are a problem.
func rrsetTTL(zone string, s *RRset, probs []Problem) (uint32, []Problem) {
	var active, all []uint32
	for _, r := range s.Records {
		all = append(all, r.TTL)
		if r.Active {
			active = append(active, r.TTL)
		}
	}
	if len(active) == 0 {
		return slices.Min(all), probs
	}
	lo, hi := slices.Min(active), slices.Max(active)
	if lo != hi {
		probs = append(probs, Problem{zone, s.Name, s.Type,
			fmt.Sprintf("its active records have different TTLs, from %d to %d; the RRset uses %d", lo, hi, lo)})
	}
	return lo, probs
}

// CompareRRsets orders RRsets, by their owner names and types, canonically:
// by name, as CompareNames does, then the SOA, then NS, then the other types
// in alphabetical order.
func CompareRRsets(aName, aType, bName, bType string) int {
	return cmp.Or(CompareNames(aName, bName), cmp.Compare(typeRank(aType), typeRank(bType)), strings.Compare(aType, bType))
}

// typeRank puts SOA, then NS, before other types at the same name.
func typeRank(typ string) int {
	switch typ {
	case "SOA":
		return 0
	case "NS":
		return 1
	}
	return 2
}
