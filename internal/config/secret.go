package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

// Redacted is what a set Secret shows instead of its value.
const Redacted = "[redacted]"

// Secret holds a sensitive string, such as an API token. It redacts itself
// wherever it could be printed, formatted, marshaled or logged: only Reveal
// returns the value. An unset Secret shows as the empty string, so it's
// visible that no value is configured.
type Secret struct {
	value string
}

// NewSecret returns a Secret holding value.
func NewSecret(value string) Secret { return Secret{value: value} }

// Reveal returns the secret's value. Call it only where the value is used,
// never to print or log it.
func (s Secret) Reveal() string { return s.value }

// IsSet reports whether the secret has a value.
func (s Secret) IsSet() bool { return s.value != "" }

// String returns Redacted, or "" if the secret is unset.
func (s Secret) String() string {
	if s.value == "" {
		return ""
	}
	return Redacted
}

// GoString redacts the secret for %#v.
func (s Secret) GoString() string { return fmt.Sprintf("config.Secret(%q)", s.String()) }

// Format redacts the secret for every fmt verb.
func (s Secret) Format(f fmt.State, verb rune) {
	switch verb {
	case 'v':
		if f.Flag('#') {
			fmt.Fprint(f, s.GoString())
			return
		}
		fmt.Fprint(f, s.String())
	case 'q':
		fmt.Fprintf(f, "%q", s.String())
	default:
		fmt.Fprint(f, s.String())
	}
}

// MarshalJSON redacts the secret in JSON.
func (s Secret) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

// MarshalText redacts the secret for text encoders, such as YAML.
func (s Secret) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// LogValue redacts the secret in log/slog output.
func (s Secret) LogValue() slog.Value { return slog.StringValue(s.String()) }
