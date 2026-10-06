package dns

import (
	"cmp"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	mdns "codeberg.org/miekg/dns"
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
// and origin its zone, for relative names. Every type that miekg/dns knows
// is parsed as its RDATA and printed as the library's canonical text, with
// domain names in lowercase and hex in uppercase (ADR-0025). TXT and SPF follow M01's
// own rule, so that NetBox's unquoted values stay one string. If the data
// doesn't parse as its type, Value returns it with its whitespace collapsed,
// and an error. A type the library doesn't know is returned that way too,
// with no error.
func Value(typ, value, origin string) (string, error) {
	asIs := strings.Join(strings.Fields(value), " ")
	if typ == "TXT" || typ == "SPF" {
		s, err := canonicalTXT(value)
		if err != nil {
			return asIs, err
		}
		return s, nil
	}
	t, ok := mdns.StringToType[typ]
	if !ok {
		return asIs, nil
	}
	rd, err := mdns.NewData(t, strings.TrimSpace(value), Name("", origin))
	if err != nil {
		return asIs, fmt.Errorf("%q isn't %s data: %w", asIs, typ, err)
	}
	out := canonical(rd)
	if n, changed := changedNumber(asIs, out); changed {
		// miekg/dns keeps a number too big for its field modulo the field's
		// size, so that 70000 in a 16-bit field would become 4464.
		return asIs, fmt.Errorf("%q isn't %s data: %s is out of range", asIs, typ, n)
	}
	return out, nil
}

// canonical prints rd with its domain names made lowercase and its hex
// uppercase, since DNS compares both without regard to case and miekg/dns
// prints some types' hex in uppercase whatever case it was given. Other
// fields, such as base64 keys and text, keep their case.
func canonical(rd mdns.RDATA) string {
	if reflect.TypeOf(rd).Kind() != reflect.Struct {
		return rd.String()
	}
	v := reflect.New(reflect.TypeOf(rd)).Elem()
	v.Set(reflect.ValueOf(rd))
	for i := range v.NumField() {
		f, fold := v.Field(i), folding(v.Type().Field(i).Tag.Get("dns"))
		if !f.CanSet() || fold == nil {
			continue
		}
		switch {
		case f.Kind() == reflect.String:
			f.SetString(fold(f.String()))
		case f.Kind() == reflect.Slice && f.Type().Elem().Kind() == reflect.String:
			for j := range f.Len() {
				f.Index(j).SetString(fold(f.Index(j).String()))
			}
		}
	}
	return v.Interface().(mdns.RDATA).String()
}

// folding returns how to fold the case of a field with a miekg/dns tag: a
// domain name to lowercase, hex to uppercase, or nil to keep it as it is.
func folding(tag string) func(string) string {
	switch {
	case tag == "name" || tag == "cname" || tag == "mname":
		return strings.ToLower
	case tag == "hex" || strings.HasPrefix(tag, "size-hex"):
		return strings.ToUpper
	}
	return nil
}

// changedNumber returns the first number in the input in that differs from
// the number in the same place in the printed data out, if there is one.
// Only places where both hold a plain decimal number are compared, so 010
// and 10 are the same number.
func changedNumber(in, out string) (string, bool) {
	a, b := fields(in), fields(out)
	for i := range min(len(a), len(b)) {
		if isDigits(a[i]) && isDigits(b[i]) && trimZeros(a[i]) != trimZeros(b[i]) {
			return a[i], true
		}
	}
	return "", false
}

// fields splits zone-file text at whitespace outside double quotes.
func fields(s string) []string {
	var (
		out              []string
		b                strings.Builder
		quoted, escaping bool
	)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case escaping:
			escaping = false
		case c == '\\':
			escaping = true
		case c == '"':
			quoted = !quoted
		case !quoted && (c == ' ' || c == '\t'):
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
			continue
		}
		b.WriteByte(c)
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

func trimZeros(s string) string {
	if t := strings.TrimLeft(s, "0"); t != "" {
		return t
	}
	return "0"
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
