package dns

import (
	"cmp"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// maxStringLen is the longest character string a TXT record can hold.
const maxStringLen = 255

// Name returns name as an absolute, lowercase domain name ending with a dot.
// An empty name or @ is origin itself, and a name without a final dot is
// relative to origin.
func Name(name, origin string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	o := strings.ToLower(strings.TrimSpace(origin))
	if !strings.HasSuffix(o, ".") {
		o += "."
	}
	switch {
	case n == "" || n == "@":
		return o
	case strings.HasSuffix(n, "."):
		return n
	case o == ".":
		return n + "."
	default:
		return n + "." + o
	}
}

// CompareNames orders absolute names canonically (RFC 4034, section 6.1): by
// their labels from the right, so a zone's apex comes before its other names.
func CompareNames(a, b string) int {
	la := strings.Split(strings.TrimSuffix(a, "."), ".")
	lb := strings.Split(strings.TrimSuffix(b, "."), ".")
	for i := 1; i <= min(len(la), len(lb)); i++ {
		if c := strings.Compare(la[len(la)-i], lb[len(lb)-i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(la), len(lb))
}

// Value returns a record's data in normalized form: typ is the record's type,
// and origin its zone, for relative names. If the data doesn't parse as its
// type, Value returns it with its whitespace collapsed, and an error.
func Value(typ, value, origin string) (string, error) {
	f := strings.Fields(value)
	asIs := strings.Join(f, " ")
	var err error
	switch typ {
	case "A", "AAAA":
		a, perr := netip.ParseAddr(strings.TrimSpace(value))
		if perr == nil && (typ == "A") == a.Is4() {
			return a.String(), nil
		}
		err = fmt.Errorf("%q isn't an %s address", strings.TrimSpace(value), typ)
	case "CNAME", "DNAME", "NS", "PTR":
		if len(f) == 1 {
			return Name(f[0], origin), nil
		}
		err = fmt.Errorf("want one name, got %q", value)
	case "MX":
		if len(f) == 2 {
			if pref, perr := strconv.ParseUint(f[0], 10, 16); perr == nil {
				return fmt.Sprintf("%d %s", pref, Name(f[1], origin)), nil
			}
		}
		err = fmt.Errorf("want a preference and a name, got %q", value)
	case "SRV":
		if len(f) == 4 {
			nums, nerr := uint16s(f[:3])
			if nerr == nil {
				return fmt.Sprintf("%d %d %d %s", nums[0], nums[1], nums[2], Name(f[3], origin)), nil
			}
		}
		err = fmt.Errorf("want a priority, weight, port and target, got %q", value)
	case "SOA":
		if len(f) == 7 {
			return strings.Join(append([]string{Name(f[0], origin), Name(f[1], origin)}, f[2:]...), " "), nil
		}
		err = fmt.Errorf("want seven SOA fields, got %q", value)
	case "TXT", "SPF":
		var s string
		if s, err = canonicalTXT(value); err == nil {
			return s, nil
		}
	default:
		return asIs, nil
	}
	return asIs, err
}

func uint16s(fields []string) ([]uint64, error) {
	out := make([]uint64, len(fields))
	for i, s := range fields {
		n, err := strconv.ParseUint(s, 10, 16)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}

// canonicalTXT returns a TXT value in canonical form. A value that starts
// with a double quote is read as a zone file would read it: quoted strings,
// with \" \\ and \DDD escapes, and bare words between them. Any other value
// is one string, as the NetBox DNS plugin takes it.
func canonicalTXT(value string) (string, error) {
	strs := []string{value}
	if strings.HasPrefix(strings.TrimSpace(value), `"`) {
		var err error
		if strs, err = parseStrings(value); err != nil {
			return "", err
		}
	}
	var out []string
	for _, s := range strs {
		for len(s) > maxStringLen {
			out = append(out, quote(s[:maxStringLen]))
			s = s[maxStringLen:]
		}
		out = append(out, quote(s))
	}
	return strings.Join(out, " "), nil
}

// parseStrings splits zone-file text into its character strings.
func parseStrings(text string) ([]string, error) {
	var out []string
	for i := 0; i < len(text); {
		switch text[i] {
		case ' ', '\t':
			i++
		case '"':
			s, n, err := unescape(text[i+1:], true)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
			i += 1 + n
		default:
			s, n, err := unescape(text[i:], false)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
			i += n
		}
	}
	return out, nil
}

// unescape reads one character string from the start of text, up to a
// closing quote if quoted, or else up to whitespace. It returns the string
// and how many bytes of text it used, including the closing quote.
func unescape(text string, quoted bool) (string, int, error) {
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case quoted && c == '"':
			return b.String(), i + 1, nil
		case !quoted && (c == ' ' || c == '\t'):
			return b.String(), i, nil
		case c == '\\':
			e, n, err := escape(text[i+1:])
			if err != nil {
				return "", 0, err
			}
			b.WriteByte(e)
			i += n
		default:
			b.WriteByte(c)
		}
	}
	if quoted {
		return "", 0, errors.New("a quoted string isn't closed")
	}
	return b.String(), len(text), nil
}

// escape reads the escape at the start of text, which follows a backslash:
// \DDD, a byte in decimal, or else one character standing for itself. It
// returns the byte and how many bytes of text it used.
func escape(text string) (byte, int, error) {
	switch {
	case len(text) >= 3 && isDigits(text[:3]):
		n, err := strconv.ParseUint(text[:3], 10, 8)
		if err != nil {
			return 0, 0, fmt.Errorf("escape \\%s is out of range", text[:3])
		}
		return byte(n), 3, nil
	case text != "":
		return text[0], 1, nil
	}
	return 0, 0, errors.New("the text ends with a lone backslash")
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

// quote writes s as a quoted character string, escaping " and \ with a
// backslash, and bytes outside printable ASCII as \DDD.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c < 0x20 || c > 0x7e:
			fmt.Fprintf(&b, "\\%03d", c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}
