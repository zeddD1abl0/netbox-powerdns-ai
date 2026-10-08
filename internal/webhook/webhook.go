// Package webhook reads NetBox's webhooks (ADR-0036). It checks each one's
// signature, and decodes its event into what nbpdns should refresh: the
// zones that the event names, or everything.
package webhook

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
)

// SignatureHeader is the header that carries a webhook's signature.
const SignatureHeader = "X-Hook-Signature"

// Sign returns body's signature, keyed by secret, as NetBox makes it: the
// hex HMAC-SHA512 of the body.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether signature is body's, keyed by secret. It compares
// in constant time, so how long it takes says nothing about how much of a
// forged signature is right. With no secret, nothing verifies.
func Verify(secret string, body []byte, signature string) bool {
	got, err := hex.DecodeString(signature)
	if secret == "" || err != nil || len(got) != sha512.Size {
		return false
	}
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// The DNS plugin's object types that events name.
const (
	TypeView   = "netbox_dns.view"
	TypeZone   = "netbox_dns.zone"
	TypeRecord = "netbox_dns.record"
)

// An Event is the body of a NetBox 4.7 webhook that has no body template.
// Only what nbpdns reads is decoded.
type Event struct {
	// Event is what happened to the object: created, updated or deleted.
	Event string `json:"event"`
	// ObjectType is the object's type, such as netbox_dns.record.
	ObjectType string `json:"object_type"`
	// Request is NetBox's request that made the change, or nil if none did,
	// as for a change that a script makes.
	Request *Request `json:"request"`
	// Data is the object as NetBox's API serializes it: after the change, or
	// before it, for a deletion.
	Data json.RawMessage `json:"data"`
	// Snapshots are the object's fields before and after the change, with
	// related objects as IDs.
	Snapshots *Snapshots `json:"snapshots"`
}

// A Request is the NetBox request that made a change.
type Request struct {
	// ID is NetBox's ID for the request, which every event it causes shares.
	ID string `json:"id"`
	// User is the name of the user who made the request.
	User string `json:"user"`
}

// Snapshots are an object's fields around a change.
type Snapshots struct {
	// Prechange is the object's fields before the change. It's null for a
	// creation, and for some updates that the DNS plugin makes itself.
	Prechange json.RawMessage `json:"prechange"`
}

// RequestID returns NetBox's ID for the request that made the change, or
// "".
func (e Event) RequestID() string {
	if e.Request == nil {
		return ""
	}
	return e.Request.ID
}

// User returns the name of the user who made the change, or "".
func (e Event) User() string {
	if e.Request == nil {
		return ""
	}
	return e.Request.User
}

// A Zone names a zone that an event touched: its NetBox view, and its name,
// absolute and lowercase, as dns.Name gives it.
type Zone struct {
	View string `json:"view"`
	Name string `json:"name"`
}

// A Refresh is what an event asks nbpdns to refresh.
type Refresh struct {
	// Zones are the zones the event names, unless Full is set.
	Zones []Zone
	// Full asks for a full refresh: the event changed a view, or moved a
	// zone or a record from a place that it names only by its ID.
	Full bool
	// Views, for a view's event, are the view's name, and its old name if
	// it was renamed: the full refresh is needed only if a group serves
	// one of them.
	Views []string
	// Reason says why it's a full refresh, or why it asks for nothing.
	Reason string
}

// Ignored reports whether r asks for nothing.
func (r Refresh) Ignored() bool { return !r.Full && len(r.Zones) == 0 }

// view and zone are related objects, as an event's data nests them.
type view struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type zone struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	View *view  `json:"view"`
}

// target returns z as a Zone, or an error if it lacks its name or view.
func (z *zone) target() (Zone, error) {
	if z == nil || z.Name == "" || z.View == nil || z.View.Name == "" {
		return Zone{}, errors.New("no zone with a name and a view")
	}
	return Zone{View: z.View.Name, Name: dns.Name(z.Name, ".")}, nil
}

// Refresh returns what e asks nbpdns to refresh, or an error if e isn't an
// event that NetBox sends:
//   - a record's event names its zone, in the zone's view;
//   - a zone's event names the zone, in its view, and its old name too, if
//     the change renamed it;
//   - a view's event asks for a full refresh, with the view's names, as
//     does a zone that moved to another view, or a record that moved to
//     another zone, since the event names where they were only by ID;
//   - any other type's event asks for nothing.
func (e Event) Refresh() (Refresh, error) {
	if e.Event == "" || e.ObjectType == "" {
		return Refresh{}, errors.New("the event has no event or no object_type")
	}
	switch e.ObjectType {
	case TypeView:
		var now, before struct {
			Name string `json:"name"`
		}
		if err := decode(e.Data, &now); err != nil {
			return Refresh{}, fmt.Errorf("a %s event's data: %w", e.ObjectType, err)
		}
		if now.Name == "" {
			return Refresh{}, fmt.Errorf("a %s event's data has no name", e.ObjectType)
		}
		if err := decodeSnapshot(e.Snapshots, &before); err != nil {
			return Refresh{}, fmt.Errorf("a %s event's prechange snapshot: %w", e.ObjectType, err)
		}
		views := []string{now.Name}
		if before.Name != "" && before.Name != now.Name {
			views = append(views, before.Name)
		}
		return Refresh{Full: true, Views: views, Reason: "view " + now.Name + " was " + e.Event}, nil
	case TypeZone:
		var now zone
		if err := decode(e.Data, &now); err != nil {
			return Refresh{}, fmt.Errorf("a %s event's data: %w", e.ObjectType, err)
		}
		z, err := now.target()
		if err != nil {
			return Refresh{}, fmt.Errorf("a %s event's data has %w", e.ObjectType, err)
		}
		var before struct {
			Name string `json:"name"`
			View *int   `json:"view"`
		}
		if err := decodeSnapshot(e.Snapshots, &before); err != nil {
			return Refresh{}, fmt.Errorf("a %s event's prechange snapshot: %w", e.ObjectType, err)
		}
		if before.View != nil && *before.View != now.View.ID {
			return Refresh{Full: true, Reason: "zone " + z.Name + " moved to another view"}, nil
		}
		zones := []Zone{z}
		if before.Name != "" {
			if old := dns.Name(before.Name, "."); old != z.Name {
				zones = append(zones, Zone{View: z.View, Name: old})
			}
		}
		return Refresh{Zones: zones}, nil
	case TypeRecord:
		var now struct {
			Zone *zone `json:"zone"`
		}
		if err := decode(e.Data, &now); err != nil {
			return Refresh{}, fmt.Errorf("a %s event's data: %w", e.ObjectType, err)
		}
		z, err := now.Zone.target()
		if err != nil {
			return Refresh{}, fmt.Errorf("a %s event's data has %w", e.ObjectType, err)
		}
		var before struct {
			Zone *int `json:"zone"`
		}
		if err := decodeSnapshot(e.Snapshots, &before); err != nil {
			return Refresh{}, fmt.Errorf("a %s event's prechange snapshot: %w", e.ObjectType, err)
		}
		if before.Zone != nil && *before.Zone != now.Zone.ID {
			return Refresh{Full: true, Reason: "a record moved to zone " + z.Name}, nil
		}
		return Refresh{Zones: []Zone{z}}, nil
	default:
		return Refresh{Reason: "nbpdns doesn't read " + e.ObjectType + " objects"}, nil
	}
}

// decode decodes an object that an event must have.
func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return errors.New("it's missing")
	}
	return json.Unmarshal(raw, v)
}

// decodeSnapshot decodes the prechange snapshot, if there's one.
func decodeSnapshot(s *Snapshots, v any) error {
	if s == nil || len(s.Prechange) == 0 || string(s.Prechange) == "null" {
		return nil
	}
	return json.Unmarshal(s.Prechange, v)
}
