package store

import (
	"cmp"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store/sqlite"
)

// A zoneKey names one of NetBox's zones: its view, and its name.
type zoneKey struct{ view, name string }

// recordNetBoxZones writes nz, the zones that refresh id read, to NetBox's
// zones (ADR-0048): a row for each zone whose digest differs from its row's,
// or that has none, and none for each zone that's gone. For a full refresh,
// a zone is gone if nz doesn't have it; for a zone refresh, if it's in one of
// the scope's views, with one of its names, and nz doesn't have it in that
// view. A zone refresh's zones outside its views change nothing. It returns
// how many rows it wrote or deleted.
func (t *Tx) recordNetBoxZones(ctx context.Context, id int64, nz NetBoxZones) (int, error) {
	index, err := t.q.NetBoxZoneIndex(ctx)
	if err != nil {
		return 0, err
	}
	digests := make(map[zoneKey]string, len(index))
	for _, r := range index {
		digests[zoneKey{r.ViewName, r.Name}] = r.Digest
	}
	inScope := func(dns.Zone) bool { return true }
	if sc := nz.Scope; sc != nil {
		inScope = func(z dns.Zone) bool { return slices.Contains(sc.Views, z.View) }
	}
	written := 0
	read := map[zoneKey]bool{}
	for _, z := range nz.Zones {
		if !inScope(z) {
			continue
		}
		k := zoneKey{z.View, z.Name}
		read[k] = true
		text, err := jsonText(z)
		if err != nil {
			return 0, err
		}
		sum := sha512.Sum384([]byte(text))
		digest := hex.EncodeToString(sum[:])
		if d, ok := digests[k]; ok && d == digest {
			continue
		}
		if err := t.q.UpsertNetBoxZone(ctx, sqlite.UpsertNetBoxZoneParams{
			ViewName: z.View, Name: z.Name, RefreshID: id, Digest: digest, ZoneJson: text,
		}); err != nil {
			return 0, err
		}
		written++
	}
	gone := func(zoneKey) bool { return true }
	if sc := nz.Scope; sc != nil {
		gone = func(k zoneKey) bool { return slices.Contains(sc.Views, k.view) && slices.Contains(sc.Names, k.name) }
	}
	// The deletes go in a fixed order, by view and name, byte by byte.
	keys := make([]zoneKey, 0, len(digests))
	for k := range digests {
		if !read[k] && gone(k) {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(a, b zoneKey) int { return cmp.Or(cmp.Compare(a.view, b.view), cmp.Compare(a.name, b.name)) })
	for _, k := range keys {
		if err := t.q.DeleteNetBoxZone(ctx, sqlite.DeleteNetBoxZoneParams{ViewName: k.view, Name: k.name}); err != nil {
			return 0, err
		}
		written++
	}
	return written, nil
}

// loadNetBox reads what the refreshes found of NetBox with q: its state,
// the index of its zones, and the zones written after the version since, or
// every zone for 0.
func loadNetBox(ctx context.Context, q sqlite.Querier, since int64) (NetBoxState, error) {
	var out NetBoxState
	st, err := q.GetNetBoxState(ctx)
	if err != nil {
		return out, fmt.Errorf("NetBox's state: %w", err)
	}
	out.Status, out.Error = st.Status, st.Error
	if st.ZonesAsOf != "" {
		if out.ZonesAsOf, err = time.Parse(TimeLayout, st.ZonesAsOf); err != nil {
			return out, fmt.Errorf("NetBox's state: %w", err)
		}
	}
	index, err := q.NetBoxZoneIndex(ctx)
	if err != nil {
		return out, fmt.Errorf("NetBox's zones: %w", err)
	}
	out.Index = map[string]map[string]int64{}
	for _, r := range index {
		if out.Index[r.ViewName] == nil {
			out.Index[r.ViewName] = map[string]int64{}
		}
		out.Index[r.ViewName][r.Name] = r.RefreshID
	}
	rows, err := q.NetBoxZonesAfter(ctx, max(since, 0))
	if err != nil {
		return out, fmt.Errorf("NetBox's zones: %w", err)
	}
	out.Zones = make([]dns.Zone, len(rows))
	for i, r := range rows {
		if err := json.Unmarshal([]byte(r.ZoneJson), &out.Zones[i]); err != nil {
			return out, fmt.Errorf("NetBox's zone %s in view %s: %w", r.Name, r.ViewName, err)
		}
	}
	return out, nil
}
