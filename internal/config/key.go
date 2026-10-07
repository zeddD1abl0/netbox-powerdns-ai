package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// EnvPrefix starts every environment variable that nbpdns reads (ADR-0005).
const EnvPrefix = "NBPDNS_"

// ConfigEnv names the config file, like the --config flag.
const ConfigEnv = EnvPrefix + "CONFIG"

// A Key is one configuration setting. It's declared once, in keys, and the
// declaration drives its default, its config file key, environment variable
// and flag, its validation, `nbpdns config show`, and the generated
// reference.
type Key struct {
	// Name is the key's dotted path in the config file, such as netbox.url.
	Name string
	// Summary is one sentence, used as the flag's help.
	Summary string
	// Details adds sentences to Summary in the reference.
	Details string
	// Warning, if set, is shown as a warning in the reference.
	Warning string
	// Default is the default value, written as in the config file.
	Default string
	// Secret marks a key whose value is redacted everywhere, and which can
	// also be read from a file.
	Secret bool

	typ      string              // the kind of value, for the reference
	flagType string              // the kind of value, for --help
	parse    func(raw any) error // validates a raw value and stores it
	show     func() string       // the stored value, redacted if secret
}

// Env returns the key's environment variable, such as NBPDNS_NETBOX_URL.
func (k Key) Env() string { return EnvPrefix + strings.ToUpper(strings.ReplaceAll(k.Name, ".", "_")) }

// Flag returns the key's flag, without its leading dashes, such as
// netbox-url.
func (k Key) Flag() string { return strings.NewReplacer(".", "-", "_", "-").Replace(k.Name) }

// FileName returns the config file key that reads a secret from a file,
// such as netbox.token_file.
func (k Key) FileName() string { return k.Name + "_file" }

// FileEnv returns the environment variable that reads a secret from a file,
// such as NBPDNS_NETBOX_TOKEN_FILE.
func (k Key) FileEnv() string { return k.Env() + "_FILE" }

// FileFlag returns the flag that reads a secret from a file, such as
// netbox-token-file.
func (k Key) FileFlag() string { return k.Flag() + "-file" }

// UnsetError returns the error for a key that a command needs but that has
// no value, saying where to set it.
func UnsetError(name string) error {
	for _, k := range Keys() {
		if k.Name != name {
			continue
		}
		if k.Secret {
			return fmt.Errorf("%s isn't set; set it, or %s for a file that holds it, in the config file; or set %s or %s; or pass --%s or --%s",
				k.Name, k.FileName(), k.Env(), k.FileEnv(), k.Flag(), k.FileFlag())
		}
		return fmt.Errorf("%s isn't set; set it in the config file, set %s, or pass --%s", k.Name, k.Env(), k.Flag())
	}
	return fmt.Errorf("%s isn't set", name)
}

// Type describes the key's values, in Markdown, for the reference.
func (k Key) Type() string { return k.typ }

// MarkdownUsage is the flag annotation that holds a flag's help in Markdown,
// for the generated reference. The help itself is plain text, since pflag
// would take a back-quoted word as the flag's value name.
const MarkdownUsage = "nbpdns/markdown-usage"

// usage is the flag's help: the summary, without Markdown.
func (k Key) usage() string { return strings.ReplaceAll(k.Summary, "`", "") }

// stringKey declares a string key stored in dst. typ describes its values
// for the reference, and flagType for --help. check, if not nil, validates
// non-empty values.
func stringKey(dst *string, typ, flagType string, k Key, check func(string) error) Key {
	k.typ, k.flagType = typ, flagType
	k.parse = func(raw any) error {
		s, err := toString(raw)
		if err != nil {
			return err
		}
		if s != "" && check != nil {
			if err := check(s); err != nil {
				return err
			}
		}
		*dst = s
		return nil
	}
	k.show = func() string { return *dst }
	return k
}

// enumKey declares a string key stored in dst that takes one of allowed.
func enumKey(dst *string, k Key, allowed ...string) Key {
	quoted := make([]string, len(allowed))
	for i, a := range allowed {
		quoted[i] = "`" + a + "`"
	}
	typ := strings.Join(quoted[:len(quoted)-1], ", ") + " or " + quoted[len(quoted)-1]
	return stringKey(dst, typ, "string", k, func(s string) error {
		if !slices.Contains(allowed, s) {
			return fmt.Errorf("%q isn't one of %s", s, strings.Join(allowed, ", "))
		}
		return nil
	})
}

// secretKey declares a secret string key stored in dst.
func secretKey(dst *Secret, k Key) Key {
	k.Secret = true
	k.typ, k.flagType = "string, secret", "string"
	k.parse = func(raw any) error {
		s, err := secretString(raw)
		if err != nil {
			return err
		}
		*dst = NewSecret(s)
		return nil
	}
	k.show = func() string { return dst.String() }
	return k
}

// intKey declares an integer key stored in dst, from 1 to hi inclusive.
func intKey(dst *int, k Key, hi int) Key {
	const lo = 1
	k.typ = fmt.Sprintf("integer, %d to %d", lo, hi)
	k.flagType = "integer"
	k.parse = func(raw any) error {
		var n int
		switch v := raw.(type) {
		case int:
			n = v
		case string:
			var err error
			if n, err = strconv.Atoi(strings.TrimSpace(v)); err != nil {
				return fmt.Errorf("%q isn't an integer", v)
			}
		default:
			return fmt.Errorf("want an integer, not %s", describe(raw))
		}
		if n < lo || n > hi {
			return fmt.Errorf("%d is outside %d to %d", n, lo, hi)
		}
		*dst = n
		return nil
	}
	k.show = func() string { return strconv.Itoa(*dst) }
	return k
}

// durationKey declares a positive duration key stored in dst, written the
// way time.ParseDuration reads it, such as 30s.
func durationKey(dst *time.Duration, k Key) Key {
	k.typ, k.flagType = "duration", "duration"
	k.parse = func(raw any) error {
		s, ok := raw.(string)
		if !ok {
			return fmt.Errorf("want a duration with a unit, such as 30s, not %s", describe(raw))
		}
		d, err := time.ParseDuration(strings.TrimSpace(s))
		if err != nil {
			return fmt.Errorf("%q isn't a duration such as 30s", s)
		}
		if d <= 0 {
			return fmt.Errorf("%q isn't positive", s)
		}
		*dst = d
		return nil
	}
	k.show = func() string { return dst.String() }
	return k
}

// durationKeyAtLeast is durationKey, for a duration of at least least.
func durationKeyAtLeast(dst *time.Duration, k Key, least time.Duration) Key {
	k = durationKey(dst, k)
	k.typ = "duration, at least `" + least.String() + "`"
	parse := k.parse
	k.parse = func(raw any) error {
		old := *dst
		if err := parse(raw); err != nil {
			return err
		}
		if d := *dst; d < least {
			*dst = old
			return fmt.Errorf("%s is shorter than %s", d, least)
		}
		return nil
	}
	return k
}

// checkListen accepts a TCP address to listen on, such as :8080 or
// 127.0.0.1:8080.
func checkListen(s string) error {
	_, port, err := net.SplitHostPort(s)
	if err != nil {
		return fmt.Errorf("%q isn't an address such as :8080 or 127.0.0.1:8080", s)
	}
	if n, err := strconv.Atoi(port); err != nil || n < 0 || n > 65535 {
		return fmt.Errorf("%q has no port number from 0 to 65535", s)
	}
	return nil
}

// checkURL accepts an absolute http or https URL with a host, and no
// credentials, query or fragment.
func checkURL(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("%q isn't a URL", s)
	}
	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return fmt.Errorf("%q needs an http:// or https:// scheme", s)
	case u.Host == "":
		return fmt.Errorf("%q has no host", s)
	case u.User != nil:
		return errors.New("the URL mustn't contain credentials; set the token or API key in its own key")
	case u.RawQuery != "" || u.Fragment != "":
		return fmt.Errorf("%q mustn't have a query or fragment", s)
	}
	return nil
}

// secretString accepts only a string. A secret that YAML reads as a number
// or a boolean would come back in Go's form, such as 1e+10 for 1e10, and
// silently stop matching. The error names only the kind of value it got,
// never the value, which is the secret.
func secretString(raw any) (string, error) {
	switch v := raw.(type) {
	case string:
		return v, nil
	case int, int64, uint64, float64:
		return "", errors.New("want a string, not a number; put it in quotes")
	case bool:
		return "", errors.New("want a string, not true or false; put it in quotes")
	case map[string]any, []any, nil:
		return "", fmt.Errorf("want a string, not %s", describe(raw))
	default:
		return "", errors.New("want a string; put it in quotes")
	}
}

// toString accepts a string, or a YAML scalar that reads as one.
func toString(raw any) (string, error) {
	switch v := raw.(type) {
	case string:
		return v, nil
	case int, int64, float64, bool:
		return fmt.Sprint(v), nil
	default:
		return "", fmt.Errorf("want a string, not %s", describe(raw))
	}
}

// describe names a raw value's kind for an error message.
func describe(raw any) string {
	switch raw.(type) {
	case map[string]any:
		return "a mapping"
	case []any:
		return "a list"
	case nil:
		return "an empty value"
	default:
		return fmt.Sprintf("%v", raw)
	}
}
