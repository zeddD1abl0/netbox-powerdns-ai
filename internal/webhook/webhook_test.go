package webhook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The test data are bodies that the lab's NetBox 4.7 sent, byte for byte,
// with the signatures it sent with them, keyed by this lab-only secret.
const (
	captures      = "testdata/netbox-47"
	captureSecret = "nbpdns-capture-secret" // gitleaks:allow
)

func capture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(captures, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func signatures(t *testing.T) map[string]string {
	t.Helper()
	var sigs map[string]string
	if err := json.Unmarshal(capture(t, "signatures.json"), &sigs); err != nil {
		t.Fatal(err)
	}
	return sigs
}

func TestSignAndVerifyNetBoxsSignatures(t *testing.T) {
	sigs := signatures(t)
	if len(sigs) == 0 {
		t.Fatal("no signatures")
	}
	for name, sig := range sigs {
		t.Run(name, func(t *testing.T) {
			body := capture(t, name)
			if got := Sign(captureSecret, body); got != sig {
				t.Errorf("Sign = %s, NetBox sent %s", got, sig)
			}
			if !Verify(captureSecret, body, sig) {
				t.Error("Verify refused NetBox's own signature")
			}
		})
	}
}

func TestVerify(t *testing.T) {
	body := capture(t, "record-created.json")
	sig := signatures(t)["record-created.json"]
	other := Sign(captureSecret, capture(t, "record-updated.json"))
	tests := []struct {
		name   string
		secret string
		body   []byte
		sig    string
		want   bool
	}{
		{"NetBox's", captureSecret, body, sig, true},
		{"in uppercase hex", captureSecret, body, strings.ToUpper(sig), true},
		{"missing", captureSecret, body, "", false},
		{"another secret's", "another-secret", body, sig, false},
		{"another body's", captureSecret, body, other, false},
		{"over a changed body", captureSecret, append([]byte(nil), append(body, ' ')...), sig, false},
		{"one hex digit wrong", captureSecret, body, flipLast(sig), false},
		{"cut short", captureSecret, body, sig[:64], false},
		{"not hex", captureSecret, body, "sha512=" + sig, false},
		{"with no secret", "", body, Sign("", body), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Verify(tt.secret, tt.body, tt.sig); got != tt.want {
				t.Errorf("Verify = %t, want %t", got, tt.want)
			}
		})
	}
}

// flipLast returns hex with its last digit changed.
func flipLast(hex string) string {
	last := hex[len(hex)-1]
	if last == '0' {
		return hex[:len(hex)-1] + "1"
	}
	return hex[:len(hex)-1] + "0"
}

func TestRefreshOfNetBoxsEvents(t *testing.T) {
	tests := []struct {
		file string
		want Refresh
	}{
		{"record-created.json", Refresh{Zones: []Zone{{"capture-a", "capture.example."}}}},
		{"record-updated.json", Refresh{Zones: []Zone{{"capture-a", "capture.example."}}}},
		{"record-updated-without-snapshot.json", Refresh{Zones: []Zone{{"capture-a", "renamed.example."}}}},
		{"record-deleted.json", Refresh{Zones: []Zone{{"capture-c", "other.example."}}}},
		{"record-moved.json", Refresh{Full: true, Reason: "a record moved to zone other.example."}},
		{"zone-created.json", Refresh{Zones: []Zone{{"capture-a", "capture.example."}}}},
		{"zone-updated.json", Refresh{Zones: []Zone{{"capture-a", "capture.example."}}}},
		{"zone-renamed.json", Refresh{Zones: []Zone{{"capture-a", "renamed.example."}, {"capture-a", "capture.example."}}}},
		{"zone-moved.json", Refresh{Full: true, Reason: "zone renamed.example. moved to another view"}},
		{"zone-deleted.json", Refresh{Zones: []Zone{{"capture-c", "renamed.example."}}}},
		{"view-created.json", Refresh{Full: true, Views: []string{"capture-a"}, Reason: "view capture-a was created"}},
		{"view-renamed.json", Refresh{Full: true, Views: []string{"capture-c", "capture-b"}, Reason: "view capture-c was updated"}},
		{"view-deleted.json", Refresh{Full: true, Views: []string{"capture-a"}, Reason: "view capture-a was deleted"}},
		{"tag-created.json", Refresh{Reason: "nbpdns doesn't read extras.tag objects"}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			var e Event
			if err := json.Unmarshal(capture(t, tt.file), &e); err != nil {
				t.Fatal(err)
			}
			got, err := e.Refresh()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Refresh = %+v, want %+v", got, tt.want)
			}
			if got.Ignored() != (tt.file == "tag-created.json") {
				t.Errorf("Ignored = %t", got.Ignored())
			}
			if e.User() != "admin" {
				t.Errorf("User = %q, want admin", e.User())
			}
			if len(e.RequestID()) != 36 {
				t.Errorf("RequestID = %q, want NetBox's UUID", e.RequestID())
			}
		})
	}
}

func TestRefreshOfOtherEvents(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    Refresh
		wantErr string
	}{
		{"no object_type", `{"event": "created", "data": {}}`, Refresh{}, "no object_type"},
		{"no event", `{"object_type": "netbox_dns.zone", "data": {}}`, Refresh{}, "no event"},
		{"a record without data", `{"event": "created", "object_type": "netbox_dns.record"}`, Refresh{}, "data: it's missing"},
		{"a record without its zone", `{"event": "created", "object_type": "netbox_dns.record", "data": {"name": "www"}}`, Refresh{}, "no zone"},
		{"a record whose zone has no view",
			`{"event": "created", "object_type": "netbox_dns.record", "data": {"zone": {"id": 1, "name": "example.com"}}}`, Refresh{}, "no zone with a name and a view"},
		{"a zone whose data isn't an object", `{"event": "created", "object_type": "netbox_dns.zone", "data": 5}`, Refresh{}, "cannot unmarshal"},
		{"a zone whose snapshot isn't an object",
			`{"event": "updated", "object_type": "netbox_dns.zone", "data": {"id": 1, "name": "example.com", "view": {"id": 1, "name": "v"}}, "snapshots": {"prechange": []}}`,
			Refresh{}, "prechange snapshot"},
		{"a zone with an uppercase name",
			`{"event": "created", "object_type": "netbox_dns.zone", "data": {"id": 1, "name": "Example.COM", "view": {"id": 1, "name": "v"}}, "snapshots": null}`,
			Refresh{Zones: []Zone{{"v", "example.com."}}}, ""},
		{"a zone renamed only in case",
			`{"event": "updated", "object_type": "netbox_dns.zone", "data": {"id": 1, "name": "example.com", "view": {"id": 1, "name": "v"}}, "snapshots": {"prechange": {"name": "EXAMPLE.com", "view": 1}}}`,
			Refresh{Zones: []Zone{{"v", "example.com."}}}, ""},
		{"no request", `{"event": "created", "object_type": "dcim.site", "data": {}}`, Refresh{Reason: "nbpdns doesn't read dcim.site objects"}, ""},
		{"a view without a name", `{"event": "created", "object_type": "netbox_dns.view", "data": {"id": 1}}`, Refresh{}, "no name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e Event
			if err := json.Unmarshal([]byte(tt.body), &e); err != nil {
				t.Fatal(err)
			}
			got, err := e.Refresh()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Refresh error = %v, want one with %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Refresh = %+v, want %+v", got, tt.want)
			}
			if e.RequestID() != "" || e.User() != "" {
				t.Errorf("RequestID, User = %q, %q, want none", e.RequestID(), e.User())
			}
		})
	}
}
