package config

import (
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/http/httpguts"
)

// ParseHeaders reads otlp.headers: name=value pairs, separated by commas,
// with each value percent-decoded, as OTEL_EXPORTER_OTLP_HEADERS is written.
// The value is a secret, so an error says which pair is wrong by its
// number, and never what it holds.
func ParseHeaders(s string) (map[string]string, error) {
	h := map[string]string{}
	if strings.TrimSpace(s) == "" {
		return h, nil
	}
	for i, pair := range strings.Split(s, ",") {
		name, value, ok := strings.Cut(pair, "=")
		name = strings.TrimSpace(name)
		if !ok || !httpguts.ValidHeaderFieldName(name) {
			return nil, fmt.Errorf("pair %d isn't a header name, =, and a value", i+1)
		}
		v, err := url.PathUnescape(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("pair %d's value isn't percent-encoded correctly", i+1)
		}
		h[name] = v
	}
	return h, nil
}
