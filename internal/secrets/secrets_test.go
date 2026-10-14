package secrets

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSealAndOpen(t *testing.T) {
	k := NewMasterKey()
	stored, err := Seal(k, "setting:netbox.token", "nbt_abc.s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "s3cret") || strings.Contains(stored, base64.StdEncoding.EncodeToString([]byte("nbt_abc.s3cret"))) {
		t.Fatalf("the sealed value shows the secret: %s", stored)
	}
	if id, err := KeyID(stored); err != nil || id != k.ID() || len(id) != 16 {
		t.Errorf("KeyID() = %q, %v, want %s", id, err, k.ID())
	}
	got, err := Open(k, "setting:netbox.token", stored)
	if err != nil || got != "nbt_abc.s3cret" {
		t.Errorf("Open() = %q, %v", got, err)
	}
	// Each seal has its own data key and nonces.
	again, _ := Seal(k, "setting:netbox.token", "nbt_abc.s3cret")
	if again == stored {
		t.Error("two seals of one value are the same")
	}
}

func TestOpenRefuses(t *testing.T) {
	k := NewMasterKey()
	stored, err := Seal(k, "setting:netbox.token", "nbt_abc.s3cret")
	if err != nil {
		t.Fatal(err)
	}
	var s sealed
	if err := json.Unmarshal([]byte(stored), &s); err != nil {
		t.Fatal(err)
	}
	tamperedValue := func() string {
		c := s
		b, _ := base64.StdEncoding.DecodeString(c.Value)
		b[len(b)-1] ^= 1
		c.Value = base64.StdEncoding.EncodeToString(b)
		out, _ := json.Marshal(c)
		return string(out)
	}()
	tamperedKey := func() string {
		c := s
		b, _ := base64.StdEncoding.DecodeString(c.DataKey)
		b[len(b)-1] ^= 1
		c.DataKey = base64.StdEncoding.EncodeToString(b)
		out, _ := json.Marshal(c)
		return string(out)
	}()
	tests := []struct {
		name   string
		k      MasterKey
		place  string
		stored string
		want   error
		text   string
	}{
		{"another master key", NewMasterKey(), "setting:netbox.token", stored, nil, "it's sealed with master key " + k.ID()},
		{"no master key", MasterKey{}, "setting:netbox.token", stored, nil, "database.master_key isn't set"},
		{"another place", k, "setting:netbox.webhook_secret", stored, ErrTampered, ""},
		{"a changed value", k, "setting:netbox.token", tamperedValue, ErrTampered, ""},
		{"a changed data key", k, "setting:netbox.token", tamperedKey, ErrTampered, ""},
		{"not a sealed value", k, "setting:netbox.token", "nbt_abc.s3cret", nil, "it isn't a sealed value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Open(tt.k, tt.place, tt.stored)
			switch {
			case err == nil:
				t.Fatalf("Open() = %q, want an error", got)
			case tt.want != nil && !errors.Is(err, tt.want):
				t.Errorf("Open() error = %v, want %v", err, tt.want)
			case tt.text != "" && !strings.Contains(err.Error(), tt.text):
				t.Errorf("Open() error = %v, want %q", err, tt.text)
			case strings.Contains(err.Error(), "s3cret"):
				t.Errorf("the error shows the secret: %v", err)
			}
		})
	}
	var wk *WrongKeyError
	if _, err := Open(NewMasterKey(), "setting:netbox.token", stored); !errors.As(err, &wk) {
		t.Errorf("Open() with another key = %v, want a WrongKeyError", err)
	}
}

func TestRewrap(t *testing.T) {
	old, next := NewMasterKey(), NewMasterKey()
	stored, err := Seal(old, "group:site-b", "pdns-key")
	if err != nil {
		t.Fatal(err)
	}
	rewrapped, err := Rewrap(old, next, "group:site-b", stored)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Open(next, "group:site-b", rewrapped); err != nil || got != "pdns-key" {
		t.Errorf("Open() with the new key = %q, %v", got, err)
	}
	if _, err := Open(old, "group:site-b", rewrapped); err == nil {
		t.Error("the old key still opens the rewrapped value")
	}
	var a, b sealed
	_ = json.Unmarshal([]byte(stored), &a)
	_ = json.Unmarshal([]byte(rewrapped), &b)
	if a.Value != b.Value || a.DataKey == b.DataKey || b.KeyID != next.ID() {
		t.Error("rewrapping changed the value, or kept the old wrapping")
	}
	if _, err := Rewrap(NewMasterKey(), next, "group:site-b", stored); err == nil {
		t.Error("Rewrap() with the wrong old key succeeded")
	}
}

func TestParseMasterKey(t *testing.T) {
	k := NewMasterKey()
	parsed, err := ParseMasterKey(k.Base64() + "\n")
	if err != nil || parsed.ID() != k.ID() {
		t.Errorf("ParseMasterKey() = %v, %v, want %s", parsed, err, k)
	}
	for _, bad := range []string{"not base64!", base64.StdEncoding.EncodeToString([]byte("too short"))} {
		if _, err := ParseMasterKey(bad); err == nil || strings.Contains(err.Error(), bad) {
			t.Errorf("ParseMasterKey(%q) = %v, want an error without the value", bad, err)
		}
	}
	if s := k.String(); strings.Contains(s, k.Base64()) || !strings.Contains(s, k.ID()) {
		t.Errorf("String() = %q, want the ID, and not the key", s)
	}
}
