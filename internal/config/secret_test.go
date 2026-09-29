package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

const secretValue = "nbt_hunter2.s3cr3t" // A fake token, to test redaction. gitleaks:allow

// holder nests a secret the way a config struct does.
type holder struct {
	Name  string
	Token Secret
}

func TestSecretRedacts(t *testing.T) {
	s := NewSecret(secretValue)
	h := holder{Name: "n", Token: s}
	var logJSON, logText bytes.Buffer
	slog.New(slog.NewJSONHandler(&logJSON, nil)).Info("m", "token", s, "holder", h)
	slog.New(slog.NewTextHandler(&logText, nil)).Info("m", slog.Any("token", s), slog.Any("ptr", &s))
	jsonOut, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	text, err := s.MarshalText()
	if err != nil {
		t.Fatal(err)
	}

	outputs := map[string]string{
		"String":            s.String(),
		"%v":                fmt.Sprintf("%v", s),
		"%s":                fmt.Sprintf("%s", s),
		"%q":                fmt.Sprintf("%q", s),
		"%x":                fmt.Sprintf("%x", s),
		"%d":                fmt.Sprintf("%d", s),
		"%10s":              fmt.Sprintf("%10s", s),
		"%#v":               fmt.Sprintf("%#v", s),
		"pointer %v":        fmt.Sprintf("%v", &s),
		"Sprint":            fmt.Sprint(s),
		"Sprintln":          fmt.Sprintln(s),
		"struct %v":         fmt.Sprintf("%v", h),
		"struct %+v":        fmt.Sprintf("%+v", h),
		"struct %#v":        fmt.Sprintf("%#v", h),
		"error wrap":        fmt.Errorf("token %v: %w", s, errors.New("bad")).Error(),
		"json":              string(jsonOut),
		"MarshalText":       string(text),
		"slog JSON handler": logJSON.String(),
		"slog text handler": logText.String(),
	}
	for name, out := range outputs {
		if strings.Contains(out, secretValue) || strings.Contains(out, "hunter2") {
			t.Errorf("%s leaks the secret: %s", name, out)
		}
		if !strings.Contains(out, Redacted) {
			t.Errorf("%s = %q, want it to show %s", name, out, Redacted)
		}
	}
	if got := s.Reveal(); got != secretValue {
		t.Errorf("Reveal() = %q, want the value", got)
	}
}

func TestSecretUnset(t *testing.T) {
	var s Secret
	if s.IsSet() || s.String() != "" || fmt.Sprintf("%v|%s|%q", s, s, s) != `||""` {
		t.Errorf("an unset secret should show as empty, got %q", fmt.Sprintf("%v|%s|%q", s, s, s))
	}
	if b, _ := json.Marshal(s); string(b) != `""` {
		t.Errorf("json = %s, want \"\"", b)
	}
	if !NewSecret("x").IsSet() {
		t.Error("IsSet() = false for a set secret")
	}
}

// TestSettingsRedactSecrets checks what `nbpdns config show` prints.
func TestSettingsRedactSecrets(t *testing.T) {
	_, settings, err := load(t, "", map[string]string{"NBPDNS_NETBOX_TOKEN": secretValue})
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "hunter2") || strings.Contains(fmt.Sprint(settings), "hunter2") {
		t.Errorf("settings leak the secret: %s", out)
	}
}
